-- +goose Up
-- Набор платформ приложения. Expand-фаза: apps.platform остаётся (старые бинарники ingest читают её
-- в LoadWriteKeyCache до перезапуска) и всегда равна platforms[1]; удаление — миграцией следующего релиза.
ALTER TABLE apps ADD COLUMN platforms app_platform[];
UPDATE apps SET platforms = ARRAY[platform];
ALTER TABLE apps ALTER COLUMN platforms SET NOT NULL;
ALTER TABLE apps ADD CONSTRAINT apps_platforms_len_chk CHECK (cardinality(platforms) BETWEEN 1 AND 9);
ALTER TABLE apps ADD CONSTRAINT apps_platform_first_chk CHECK (platform = platforms[1]);

-- Платформа ключа записи; NULL — «любая платформа». Существующие ключи получают платформу приложения,
-- поэтому уже выпущенные клиенты без context.platform пишут события как раньше.
ALTER TABLE write_keys ADD COLUMN platform app_platform;
UPDATE write_keys AS k SET platform = a.platform FROM apps AS a WHERE a.id = k.app_id;

-- Job слияния. Значение не используется в этой же транзакции (ограничение ALTER TYPE … ADD VALUE).
ALTER TYPE job_name ADD VALUE 'app_merge';

CREATE TYPE app_merge_status AS ENUM ('pending', 'running', 'done', 'failed');
CREATE TYPE app_merge_step AS ENUM ('move_keys', 'wait_keys', 'copy', 'purge_source', 'reclose', 'done');

-- Слияние «источник → цель». source_app_id обнуляется, когда шаг purge_source удаляет источник;
-- имя, платформы и пояс источника сохранены для журнала.
CREATE TABLE app_merges (
    id                uuid             PRIMARY KEY DEFAULT uuidv7(),
    target_app_id     uuid             NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    source_app_id     uuid             REFERENCES apps (id) ON DELETE SET NULL,
    source_name       text             NOT NULL,
    source_platforms  app_platform[]   NOT NULL,
    source_timezone   text             NOT NULL,
    status            app_merge_status NOT NULL DEFAULT 'pending',
    step              app_merge_step   NOT NULL DEFAULT 'move_keys',
    keys_moved_at     timestamptz,
    first_source_day  date,
    copied_partitions integer[]        NOT NULL DEFAULT '{}',
    copied_events     bigint           NOT NULL DEFAULT 0,
    total_events      bigint           NOT NULL DEFAULT 0,
    attempts          integer          NOT NULL DEFAULT 0,
    error             text,
    created_at        timestamptz      NOT NULL DEFAULT now(),
    started_at        timestamptz,
    finished_at       timestamptz,
    CONSTRAINT app_merges_distinct_apps_chk CHECK (source_app_id IS NULL OR source_app_id <> target_app_id),
    CONSTRAINT app_merges_source_name_len_chk CHECK (char_length(source_name) BETWEEN 1 AND 64),
    CONSTRAINT app_merges_counters_chk CHECK (copied_events >= 0 AND total_events >= 0 AND attempts >= 0),
    CONSTRAINT app_merges_finished_consistency_chk CHECK ((status = 'done') = (finished_at IS NOT NULL))
);
CREATE INDEX app_merges_target_created_idx ON app_merges (target_app_id, created_at DESC);
CREATE INDEX app_merges_status_created_idx ON app_merges (status, created_at);
-- Одно незавершённое слияние на приложение в каждой роли; failed тоже блокирует до повтора.
CREATE UNIQUE INDEX app_merges_active_target_uq ON app_merges (target_app_id)
    WHERE status IN ('pending', 'running', 'failed');
CREATE UNIQUE INDEX app_merges_active_source_uq ON app_merges (source_app_id)
    WHERE status IN ('pending', 'running', 'failed');

-- +goose Down
DROP TABLE app_merges;
DROP TYPE app_merge_step;
DROP TYPE app_merge_status;

-- Удалить значение из enum нельзя: пересоздаём job_name без app_merge.
DELETE FROM job_runs WHERE job = 'app_merge';
ALTER TYPE job_name RENAME TO job_name_old;
CREATE TYPE job_name AS ENUM ('close_day', 'digest', 'system_check', 'deletion', 'session_cleanup');
ALTER TABLE job_runs ALTER COLUMN job TYPE job_name USING job::text::job_name;
DROP TYPE job_name_old;

ALTER TABLE write_keys DROP COLUMN platform;

-- platform уже равна platforms[1]; остальные платформы теряются.
ALTER TABLE apps DROP CONSTRAINT apps_platform_first_chk;
ALTER TABLE apps DROP CONSTRAINT apps_platforms_len_chk;
ALTER TABLE apps DROP COLUMN platforms;
