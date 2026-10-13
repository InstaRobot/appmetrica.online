-- name: ListWriteKeys :many
SELECT id, app_id, key, label, created_at, revoked_at, last_used_at
FROM write_keys WHERE app_id = sqlc.arg(app_id) ORDER BY created_at DESC;

-- name: CreateWriteKey :one
INSERT INTO write_keys (app_id, key, label) VALUES (sqlc.arg(app_id), sqlc.arg(key), sqlc.arg(label))
RETURNING id, app_id, key, label, created_at, revoked_at, last_used_at;

-- name: RevokeWriteKey :one
UPDATE write_keys SET revoked_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND app_id = sqlc.arg(app_id) AND revoked_at IS NULL
RETURNING id, app_id, key, label, created_at, revoked_at, last_used_at;

-- name: GetWriteKey :one
SELECT id, app_id, key, label, created_at, revoked_at, last_used_at
FROM write_keys WHERE id = sqlc.arg(id) AND app_id = sqlc.arg(app_id);

-- name: LoadWriteKeyCache :many
SELECT k.id AS write_key_id, k.key, k.app_id, a.status AS app_status, a.platform AS app_platform
FROM write_keys AS k
JOIN apps AS a ON a.id = k.app_id
WHERE k.revoked_at IS NULL;

-- name: TouchWriteKeysLastUsed :exec
UPDATE write_keys AS k
SET last_used_at = u.last_used_at
-- Два unnest в списке выборки равносильны unnest(a, b): sqlc 1.31.1 не выводит типы многоаргументного unnest.
FROM (SELECT unnest(sqlc.arg(ids)::uuid[]) AS id, unnest(sqlc.arg(last_used)::timestamptz[]) AS last_used_at) AS u
WHERE k.id = u.id AND (k.last_used_at IS NULL OR k.last_used_at < u.last_used_at);

-- name: ListApiKeys :many
SELECT id, key_prefix, label, scopes, app_id, created_at, revoked_at, last_used_at
FROM api_keys ORDER BY created_at DESC;

-- name: CreateApiKey :one
INSERT INTO api_keys (key_prefix, key_hash, label, scopes, app_id)
VALUES (sqlc.arg(key_prefix), sqlc.arg(key_hash), sqlc.arg(label), sqlc.arg(scopes), sqlc.narg(app_id))
RETURNING id, key_prefix, label, scopes, app_id, created_at, revoked_at, last_used_at;

-- name: RevokeApiKey :one
UPDATE api_keys SET revoked_at = sqlc.arg(now) WHERE id = sqlc.arg(id) AND revoked_at IS NULL
RETURNING id, key_prefix, label, scopes, app_id, created_at, revoked_at, last_used_at;

-- name: GetApiKey :one
SELECT id, key_prefix, label, scopes, app_id, created_at, revoked_at, last_used_at
FROM api_keys WHERE id = sqlc.arg(id);

-- name: GetActiveApiKeyByHash :one
SELECT id, scopes, app_id, last_used_at FROM api_keys WHERE key_hash = sqlc.arg(key_hash) AND revoked_at IS NULL;

-- name: TouchApiKeyLastUsed :exec
UPDATE api_keys SET last_used_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND (last_used_at IS NULL OR last_used_at < sqlc.arg(now)::timestamptz - interval '1 minute');
