-- name: ListApps :many
SELECT id, name, platform, timezone, status, created_at, updated_at, archived_at
FROM apps
WHERE sqlc.arg(include_archived)::boolean OR status = 'active'
ORDER BY lower(name);

-- name: GetApp :one
SELECT id, name, platform, timezone, status, created_at, updated_at, archived_at FROM apps WHERE id = sqlc.arg(id);

-- name: CreateApp :one
INSERT INTO apps (name, platform, timezone) VALUES (sqlc.arg(name), sqlc.arg(platform), sqlc.arg(timezone))
RETURNING id, name, platform, timezone, status, created_at, updated_at, archived_at;

-- name: UpdateApp :one
UPDATE apps
SET name = coalesce(sqlc.narg(name), name),
    timezone = coalesce(sqlc.narg(timezone), timezone),
    updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(id)
RETURNING id, name, platform, timezone, status, created_at, updated_at, archived_at;

-- name: ArchiveApp :one
UPDATE apps SET status = 'archived', archived_at = sqlc.arg(now), updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND status = 'active'
RETURNING id, name, platform, timezone, status, created_at, updated_at, archived_at;

-- name: UnarchiveApp :one
UPDATE apps SET status = 'active', archived_at = NULL, updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND status = 'archived'
RETURNING id, name, platform, timezone, status, created_at, updated_at, archived_at;

-- name: DeleteApp :execrows
DELETE FROM apps WHERE id = sqlc.arg(id);

-- name: ListAppsForJobs :many
SELECT id, name, timezone, status, created_at FROM apps ORDER BY id;
