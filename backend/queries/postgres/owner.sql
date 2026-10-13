-- name: GetOwner :one
SELECT id, webauthn_user_handle, display_name, created_at FROM owner WHERE singleton;

-- name: GetOrCreateOwner :one
INSERT INTO owner (webauthn_user_handle) VALUES (sqlc.arg(webauthn_user_handle))
ON CONFLICT (singleton) DO UPDATE SET singleton = EXCLUDED.singleton
RETURNING id, webauthn_user_handle, display_name, created_at;

-- name: LockOwner :one
SELECT id FROM owner WHERE singleton FOR UPDATE;
