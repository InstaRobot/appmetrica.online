-- name: ListPasskeys :many
SELECT id, name, aaguid, transports, backup_eligible, backup_state, created_at, last_used_at
FROM passkeys ORDER BY created_at;

-- name: ListPasskeyCredentials :many
SELECT id, credential_id, public_key, attestation_type, aaguid, sign_count, transports, backup_eligible, backup_state
FROM passkeys WHERE owner_id = sqlc.arg(owner_id);

-- name: GetPasskeyByCredentialID :one
SELECT id, owner_id, credential_id, public_key, attestation_type, aaguid, sign_count, transports,
       backup_eligible, backup_state, name, created_at, last_used_at
FROM passkeys WHERE credential_id = sqlc.arg(credential_id);

-- name: CountPasskeys :one
SELECT count(*) FROM passkeys;

-- name: CreatePasskey :one
INSERT INTO passkeys (owner_id, credential_id, public_key, attestation_type, aaguid, sign_count, transports,
                      backup_eligible, backup_state, name)
VALUES (sqlc.arg(owner_id), sqlc.arg(credential_id), sqlc.arg(public_key), sqlc.arg(attestation_type), sqlc.arg(aaguid),
        sqlc.arg(sign_count), sqlc.arg(transports), sqlc.arg(backup_eligible), sqlc.arg(backup_state), sqlc.arg(name))
RETURNING id, name, created_at;

-- name: UpdatePasskeyAfterLogin :exec
UPDATE passkeys
SET sign_count = sqlc.arg(sign_count), backup_state = sqlc.arg(backup_state), last_used_at = sqlc.arg(now)
WHERE credential_id = sqlc.arg(credential_id);

-- name: RenamePasskey :one
UPDATE passkeys SET name = sqlc.arg(name) WHERE id = sqlc.arg(id) RETURNING id, name;

-- name: DeletePasskey :execrows
DELETE FROM passkeys WHERE id = sqlc.arg(id);
