-- name: CreateAppMerge :one
INSERT INTO app_merges (target_app_id, source_app_id, source_name, source_platforms, source_timezone, total_events)
VALUES (sqlc.arg(target_app_id), sqlc.arg(source_app_id), sqlc.arg(source_name),
        sqlc.arg(source_platforms)::app_platform[], sqlc.arg(source_timezone), sqlc.arg(total_events))
RETURNING *;

-- name: GetAppMerge :one
SELECT * FROM app_merges WHERE id = sqlc.arg(id) AND target_app_id = sqlc.arg(target_app_id);

-- name: ListAppMerges :many
SELECT * FROM app_merges WHERE target_app_id = sqlc.arg(target_app_id)
ORDER BY created_at DESC LIMIT 20;

-- name: ActiveMergeForApp :one
-- Слияние, которое блокирует операции с приложением (как цель или как источник).
SELECT * FROM app_merges
WHERE (target_app_id = sqlc.arg(app_id) OR source_app_id = sqlc.arg(app_id))
  AND status IN ('pending', 'running', 'failed')
ORDER BY created_at DESC LIMIT 1;

-- name: ClaimNextAppMerge :one
-- Старейшее слияние в очереди. Запросы идут вне транзакции, поэтому строка не блокируется: запуски job
-- app_merge разделяет advisory lock worker-а (worker.MergeJob), а не блокировка строки.
SELECT * FROM app_merges
WHERE status IN ('pending', 'running')
ORDER BY created_at
LIMIT 1;

-- name: SetAppMergeTotalEvents :exec
-- Шаг copy пересчитывает объём источника: за wait_keys источник принимал события по старому снимку ключей.
UPDATE app_merges SET total_events = sqlc.arg(total_events)::bigint WHERE id = sqlc.arg(id);

-- name: SetAppMergeStep :exec
UPDATE app_merges
SET status = 'running', step = sqlc.arg(step),
    started_at = coalesce(started_at, sqlc.arg(now)::timestamptz),
    keys_moved_at = coalesce(sqlc.narg(keys_moved_at), keys_moved_at),
    first_source_day = coalesce(sqlc.narg(first_source_day), first_source_day)
WHERE id = sqlc.arg(id);

-- name: AddCopiedPartition :exec
UPDATE app_merges
SET copied_partitions = array_append(copied_partitions, sqlc.arg(partition)::integer),
    copied_events = copied_events + sqlc.arg(rows)::bigint
WHERE id = sqlc.arg(id) AND NOT (sqlc.arg(partition)::integer = ANY (copied_partitions));

-- name: FailAppMerge :exec
UPDATE app_merges SET status = 'failed', error = sqlc.arg(error)::text WHERE id = sqlc.arg(id);

-- name: RetryAppMerge :one
UPDATE app_merges SET status = 'pending', error = NULL, attempts = attempts + 1
WHERE id = sqlc.arg(id) AND target_app_id = sqlc.arg(target_app_id) AND status = 'failed'
RETURNING *;

-- name: FinishAppMerge :exec
UPDATE app_merges SET status = 'done', step = 'done', finished_at = sqlc.arg(now)::timestamptz, error = NULL
WHERE id = sqlc.arg(id);
