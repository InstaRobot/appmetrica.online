-- name: CreateSetupToken :exec
INSERT INTO passkey_setup_tokens (token_hash, created_at, expires_at)
VALUES (sqlc.arg(token_hash), sqlc.arg(now), sqlc.arg(expires_at));

-- name: GetUsableSetupToken :one
SELECT token_hash, created_at, expires_at FROM passkey_setup_tokens
WHERE token_hash = sqlc.arg(token_hash) AND used_at IS NULL AND expires_at > sqlc.arg(now)::timestamptz
FOR UPDATE;

-- name: MarkSetupTokenUsed :execrows
UPDATE passkey_setup_tokens SET used_at = sqlc.arg(now)
WHERE token_hash = sqlc.arg(token_hash) AND used_at IS NULL;

-- name: CreateCeremony :one
INSERT INTO webauthn_ceremonies (kind, session_data, setup_token_hash, created_at, expires_at)
VALUES (sqlc.arg(kind), sqlc.arg(session_data), sqlc.narg(setup_token_hash), sqlc.arg(now), sqlc.arg(expires_at))
RETURNING id;

-- name: TakeCeremony :one
DELETE FROM webauthn_ceremonies
WHERE id = sqlc.arg(id) AND kind = sqlc.arg(kind) AND expires_at > sqlc.arg(now)::timestamptz
RETURNING session_data, setup_token_hash;

-- name: CreateSession :exec
INSERT INTO auth_sessions (token_hash, owner_id, created_at, last_seen_at, expires_at, idle_timeout, user_agent)
VALUES (sqlc.arg(token_hash), sqlc.arg(owner_id), sqlc.arg(now), sqlc.arg(now), sqlc.arg(expires_at),
        make_interval(secs => sqlc.arg(idle_timeout_seconds)::float8), sqlc.arg(user_agent));

-- name: GetActiveSession :one
SELECT token_hash, owner_id, created_at, last_seen_at, expires_at
FROM auth_sessions
WHERE token_hash = sqlc.arg(token_hash)
  AND revoked_at IS NULL
  AND expires_at > sqlc.arg(now)::timestamptz
  AND last_seen_at + idle_timeout > sqlc.arg(now)::timestamptz;

-- name: TouchSession :exec
UPDATE auth_sessions SET last_seen_at = sqlc.arg(now)
WHERE token_hash = sqlc.arg(token_hash) AND last_seen_at < sqlc.arg(now)::timestamptz - interval '5 minutes';

-- name: RevokeSession :exec
UPDATE auth_sessions SET revoked_at = sqlc.arg(now) WHERE token_hash = sqlc.arg(token_hash) AND revoked_at IS NULL;

-- name: LockSetupToken :one
SELECT token_hash, expires_at, used_at FROM passkey_setup_tokens
WHERE token_hash = sqlc.arg(token_hash)
FOR UPDATE;

-- name: DeleteSetupTokens :exec
-- Только для стека E2E (auth.ResetForE2E): каскадно удаляет церемонии setup-ссылок.
DELETE FROM passkey_setup_tokens;

-- name: DeleteCeremonies :exec
-- Только для стека E2E (auth.ResetForE2E).
DELETE FROM webauthn_ceremonies;
