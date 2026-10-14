-- name: GetSettings :one
SELECT owner_timezone, telegram_chat_id, digest_enabled, digest_time, alerts_enabled, updated_at FROM settings WHERE singleton;

-- name: UpdateSettings :one
UPDATE settings
SET owner_timezone   = coalesce(sqlc.narg(owner_timezone), owner_timezone),
    telegram_chat_id = CASE WHEN sqlc.arg(clear_telegram_chat_id)::boolean THEN NULL
                            ELSE coalesce(sqlc.narg(telegram_chat_id), telegram_chat_id) END,
    digest_enabled   = coalesce(sqlc.narg(digest_enabled), digest_enabled),
    digest_time      = coalesce(sqlc.narg(digest_time), digest_time),
    alerts_enabled   = coalesce(sqlc.narg(alerts_enabled), alerts_enabled),
    updated_at       = sqlc.arg(now)
WHERE singleton
RETURNING owner_timezone, telegram_chat_id, digest_enabled, digest_time, alerts_enabled, updated_at;

-- name: StartJobRun :one
INSERT INTO job_runs (job, started_at) VALUES (sqlc.arg(job), sqlc.arg(now)) RETURNING id;

-- name: FinishJobRun :exec
-- details дополняет записанное задачей по ходу цикла (AppendDeletedAppToJobRun), а не заменяет.
UPDATE job_runs SET finished_at = sqlc.arg(now), status = sqlc.arg(status), details = details || sqlc.arg(details)::jsonb
WHERE id = sqlc.arg(id);

-- name: AppendDeletedAppToJobRun :exec
-- Копия deletion-запроса subject_type app в details.deleted_apps цикла — до удаления строки apps (каскад снимет запрос).
UPDATE job_runs
SET details = jsonb_set(details, '{deleted_apps}', coalesce(details -> 'deleted_apps', '[]'::jsonb) || jsonb_build_array(sqlc.arg(request)::jsonb))
WHERE id = sqlc.arg(id);

-- name: ListLatestJobRuns :many
SELECT DISTINCT ON (job) id, job, started_at, finished_at, status, details
FROM job_runs ORDER BY job, started_at DESC;

-- name: GetLastSuccessfulJobRun :one
SELECT id, job, started_at, finished_at, status, details
FROM job_runs WHERE job = sqlc.arg(job) AND status = 'ok' ORDER BY started_at DESC LIMIT 1;

-- name: GetDigestDayState :one
-- Запуски digest с начала суток владельца: отправлен ли дайджест (ok с details.sent_on) и неудачные попытки;
-- без неудач last_failed_at = since.
SELECT coalesce(bool_or(status = 'ok' AND details ->> 'sent_on' IS NOT NULL), false)::boolean AS sent,
       count(*) FILTER (WHERE status = 'failed') AS failed_attempts,
       coalesce(max(started_at) FILTER (WHERE status = 'failed'), sqlc.arg(since)::timestamptz)::timestamptz AS last_failed_at
FROM job_runs WHERE job = 'digest' AND started_at >= sqlc.arg(since)::timestamptz;

-- name: DeleteOldJobRuns :execrows
DELETE FROM job_runs WHERE started_at < sqlc.arg(before)::timestamptz;

-- name: ListAlertStates :many
SELECT key, firing, first_fired_at, last_notified_at, last_value FROM alert_states ORDER BY key;

-- name: GetAlertState :one
SELECT key, firing, first_fired_at, last_notified_at, last_value FROM alert_states WHERE key = sqlc.arg(key);

-- name: UpsertAlertState :one
INSERT INTO alert_states (key, firing, first_fired_at, last_notified_at, last_value)
VALUES (sqlc.arg(key), sqlc.arg(firing), sqlc.narg(first_fired_at), sqlc.narg(last_notified_at), sqlc.arg(last_value))
ON CONFLICT (key) DO UPDATE
SET firing = EXCLUDED.firing, first_fired_at = EXCLUDED.first_fired_at,
    last_notified_at = EXCLUDED.last_notified_at, last_value = EXCLUDED.last_value
RETURNING key, firing, first_fired_at, last_notified_at, last_value;

-- name: IsDayClosed :one
SELECT EXISTS (SELECT 1 FROM closed_days WHERE app_id = sqlc.arg(app_id) AND day = sqlc.arg(day)) AS closed;

-- name: MarkDayClosed :exec
INSERT INTO closed_days (app_id, day, closed_at) VALUES (sqlc.arg(app_id), sqlc.arg(day), sqlc.arg(now))
ON CONFLICT (app_id, day) DO NOTHING;

-- name: GetLastClosedDay :one
SELECT coalesce(max(day), DATE '1970-01-01')::date AS day FROM closed_days WHERE app_id = sqlc.arg(app_id);

-- name: ReopenClosedDays :execrows
-- Шаг reclose: close_day закроет эти дни заново с данными источника.
DELETE FROM closed_days WHERE app_id = sqlc.arg(app_id) AND day >= sqlc.arg(from_day);

-- name: DeleteExpiredSessions :execrows
DELETE FROM auth_sessions
WHERE expires_at < sqlc.arg(now)::timestamptz
   OR revoked_at IS NOT NULL
   OR last_seen_at + idle_timeout < sqlc.arg(now)::timestamptz;

-- name: DeleteExpiredCeremonies :execrows
DELETE FROM webauthn_ceremonies WHERE expires_at < sqlc.arg(now)::timestamptz;

-- name: DeleteSpentSetupTokens :execrows
DELETE FROM passkey_setup_tokens WHERE expires_at < sqlc.arg(now)::timestamptz OR used_at IS NOT NULL;
