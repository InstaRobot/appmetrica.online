-- name: CreateDeletionRequest :one
INSERT INTO deletion_requests (app_id, subject_type, subject_value, source, api_key_id, requested_at)
VALUES (sqlc.arg(app_id), sqlc.arg(subject_type), sqlc.arg(subject_value), sqlc.arg(source), sqlc.narg(api_key_id), sqlc.arg(now))
ON CONFLICT (app_id, subject_type, subject_value) WHERE status IN ('pending', 'running')
DO UPDATE SET requested_at = deletion_requests.requested_at
RETURNING id, app_id, subject_type, subject_value, status, source, api_key_id, requested_at, started_at, completed_at, attempts, error;

-- name: GetDeletionRequest :one
SELECT id, app_id, subject_type, subject_value, status, source, api_key_id, requested_at, started_at, completed_at, attempts, error
FROM deletion_requests WHERE id = sqlc.arg(id) AND app_id = sqlc.arg(app_id);

-- name: ListDeletionRequests :many
SELECT id, app_id, subject_type, subject_value, status, source, api_key_id, requested_at, started_at, completed_at, attempts, error
FROM deletion_requests
WHERE sqlc.narg(app_id)::uuid IS NULL OR app_id = sqlc.narg(app_id)::uuid
ORDER BY requested_at DESC
LIMIT 200;

-- name: ClaimNextDeletionRequest :one
UPDATE deletion_requests
SET status = 'running', started_at = sqlc.arg(now), attempts = attempts + 1
WHERE id = (
    SELECT id FROM deletion_requests
    WHERE status = 'pending'
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
UPDATE deletion_requests SET status = 'pending'
WHERE status = 'running' AND started_at < sqlc.arg(stale_before)::timestamptz;
