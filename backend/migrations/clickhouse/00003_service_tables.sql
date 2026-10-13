-- +goose NO TRANSACTION
-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS rejected_events
(
    received_at    DateTime64(3, 'UTC'),
    app_id         UUID,
    key_prefix     String,
    code           LowCardinality(String),
    field          String,
    message        String,
    event_name     LowCardinality(String),
    app_version    LowCardinality(String),
    platform       LowCardinality(String),
    payload_sample String
)
ENGINE = MergeTree
PARTITION BY toDate(received_at)
ORDER BY (app_id, received_at)
TTL toDate(received_at) + INTERVAL 7 DAY DELETE
SETTINGS ttl_only_drop_parts = 1;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS daily_stats
(
    app_id                 UUID,
    day                    Date,
    environment            LowCardinality(String),
    unit                   LowCardinality(String),
    active                 UInt64,
    new                    UInt64,
    sessions               UInt64,
    sessions_with_duration UInt64,
    duration_ms_sum        UInt64,
    closed_at              DateTime64(3, 'UTC')
)
ENGINE = ReplacingMergeTree(closed_at)
ORDER BY (app_id, environment, unit, day);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS daily_stats;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS rejected_events;
-- +goose StatementEnd
