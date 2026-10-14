-- name: ListApps :many
SELECT id, name, platform, timezone, status, created_at, updated_at, archived_at, platforms
FROM apps
WHERE sqlc.arg(include_archived)::boolean OR status = 'active'
ORDER BY lower(name);

-- name: GetApp :one
SELECT id, name, platform, timezone, status, created_at, updated_at, archived_at, platforms FROM apps WHERE id = sqlc.arg(id);

-- name: GetAppForUpdate :one
-- Блокировка строки на транзакцию: deleteApp, createWriteKey и unarchiveApp сериализуются, иначе ключ,
-- выпущенный параллельно с заявкой на удаление, остался бы активным.
SELECT id, name, platform, timezone, status, created_at, updated_at, archived_at, platforms FROM apps WHERE id = sqlc.arg(id) FOR UPDATE;

-- name: CreateApp :one
-- platforms нормализован сервисом (без дублей, порядок enum); platform = platforms[1] (expand-фаза).
INSERT INTO apps (name, platform, platforms, timezone)
VALUES (sqlc.arg(name), (sqlc.arg(platforms)::app_platform[])[1], sqlc.arg(platforms)::app_platform[], sqlc.arg(timezone))
RETURNING id, name, platform, timezone, status, created_at, updated_at, archived_at, platforms;

-- name: UpdateApp :one
UPDATE apps
SET name = coalesce(sqlc.narg(name), name),
    timezone = coalesce(sqlc.narg(timezone), timezone),
    platforms = coalesce(sqlc.narg(platforms)::app_platform[], platforms),
    platform = coalesce((sqlc.narg(platforms)::app_platform[])[1], platform),
    updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(id)
RETURNING id, name, platform, timezone, status, created_at, updated_at, archived_at, platforms;

-- name: ArchiveApp :one
UPDATE apps SET status = 'archived', archived_at = sqlc.arg(now), updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND status = 'active'
RETURNING id, name, platform, timezone, status, created_at, updated_at, archived_at, platforms;

-- name: UnarchiveApp :one
UPDATE apps SET status = 'active', archived_at = NULL, updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND status = 'archived'
RETURNING id, name, platform, timezone, status, created_at, updated_at, archived_at, platforms;

-- name: DeleteApp :execrows
DELETE FROM apps WHERE id = sqlc.arg(id);

-- name: AbsorbAppForMerge :one
-- Шаг move_keys: цель получает объединение платформ и самый ранний created_at.
-- ORDER BY p для enum сортирует по порядку объявления ('ios','android','web',…), поэтому platforms[1]
-- у цели может смениться; CHECK apps_platform_first_chk требует platform = platforms[1] — ставим обе.
WITH merged AS (
    SELECT ARRAY(
        SELECT p
        FROM unnest(t.platforms || sqlc.arg(source_platforms)::app_platform[]) AS p
        GROUP BY p
        ORDER BY p
    ) AS platforms
    FROM apps AS t
    WHERE t.id = sqlc.arg(target_id)
)
UPDATE apps AS t
SET platforms = merged.platforms,
    platform = merged.platforms[1],
    created_at = least(t.created_at, sqlc.arg(source_created_at)),
    updated_at = sqlc.arg(now)
FROM merged
WHERE t.id = sqlc.arg(target_id)
RETURNING t.id, t.name, t.platform, t.timezone, t.status, t.created_at, t.updated_at, t.archived_at, t.platforms;

-- name: ListAppsForJobs :many
SELECT id, name, timezone, status, created_at FROM apps ORDER BY id;
