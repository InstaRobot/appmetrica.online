-- name: CreateDeletionRequest :one
INSERT INTO deletion_requests (app_id, subject_type, subject_value, source, api_key_id, requested_at)
VALUES (sqlc.arg(app_id), sqlc.arg(subject_type), sqlc.arg(subject_value), sqlc.arg(source), sqlc.narg(api_key_id), sqlc.arg(now))
ON CONFLICT (app_id, subject_type, subject_value) WHERE status IN ('pending', 'running')
DO UPDATE SET requested_at = deletion_requests.requested_at
RETURNING id, app_id, subject_type, subject_value, status, source, api_key_id, requested_at, started_at, completed_at, attempts, error;

-- name: GetDeletionRequest :one
SELECT id, app_id, subject_type, subject_value, status, source, api_key_id, requested_at, started_at, completed_at, attempts, error
FROM deletion_requests WHERE id = sqlc.arg(id) AND app_id = sqlc.arg(app_id);

-- name: AppDeletionPending :one
-- Есть ли невыполненная заявка на удаление самого приложения (createWriteKey и unarchiveApp отвечают 409).
SELECT EXISTS (
    SELECT 1 FROM deletion_requests
    WHERE app_id = sqlc.arg(app_id) AND subject_type = 'app' AND status IN ('pending', 'running')
)::boolean AS pending;

-- name: MoveDeletionRequestsForMerge :execrows
-- Шаг purge_source: заявки на удаление человека или установки из источника переходят в цель — его события
-- уже скопированы туда, и удаление выполнится в цели. Заявка pending/running, у которой в цели уже есть
-- такая же активная, остаётся на источнике и уходит каскадом: её субъект удалит заявка цели.
UPDATE deletion_requests AS d
SET app_id = sqlc.arg(target_id)
WHERE d.app_id = sqlc.arg(source_id) AND d.subject_type <> 'app'
  AND NOT (d.status IN ('pending', 'running') AND EXISTS (
      SELECT 1 FROM deletion_requests AS t
      WHERE t.app_id = sqlc.arg(target_id) AND t.subject_type = d.subject_type AND t.subject_value = d.subject_value
        AND t.status IN ('pending', 'running')));

-- name: ListDeletionRequests :many
SELECT id, app_id, subject_type, subject_value, status, source, api_key_id, requested_at, started_at, completed_at, attempts, error
FROM deletion_requests
WHERE sqlc.narg(app_id)::uuid IS NULL OR app_id = sqlc.narg(app_id)::uuid
ORDER BY requested_at DESC
LIMIT 200;

-- name: ClaimNextDeletionRequest :one
-- Запрос app берётся не раньше app_requested_before (deletion.AppPurgeDelay после requested_at): ingest со
-- снимком ключей, загруженным до заявки, ещё принимает события приложения — снимок живёт до KeyCacheMaxAge.
UPDATE deletion_requests
SET status = 'running', started_at = sqlc.arg(now), attempts = attempts + 1
WHERE id = (
    SELECT id FROM deletion_requests
    WHERE status = 'pending'
      AND (subject_type <> 'app' OR requested_at <= sqlc.arg(app_requested_before)::timestamptz)
    ORDER BY requested_at
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
RETURNING id, app_id, subject_type, subject_value, status, source, api_key_id, requested_at, started_at, completed_at, attempts, error;

-- name: CompleteDeletionRequest :exec
UPDATE deletion_requests SET status = 'done', completed_at = sqlc.arg(now), error = NULL WHERE id = sqlc.arg(id);

-- name: FailDeletionRequest :exec
UPDATE deletion_requests
SET status = CASE WHEN attempts >= 5 THEN 'failed'::deletion_status ELSE 'pending'::deletion_status END,
    error = sqlc.arg(error)
WHERE id = sqlc.arg(id);

-- name: RequeueStuckDeletionRequests :execrows
-- Зависший running (worker упал посреди удаления) возвращается в очередь; с исчерпанными попытками — failed:
-- следующий claim нарушил бы CHECK attempts ≤ 5.
UPDATE deletion_requests
SET status = CASE WHEN attempts >= 5 THEN 'failed'::deletion_status ELSE 'pending'::deletion_status END,
    error  = coalesce(error, 'worker interrupted')
WHERE status = 'running' AND started_at < sqlc.arg(stale_before)::timestamptz;
