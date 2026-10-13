-- +goose NO TRANSACTION
-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS events
(
    app_id        UUID,
    event_date    Date,
    event_time    DateTime64(3, 'UTC'),
    client_time   DateTime64(3, 'UTC'),
    sent_at       DateTime64(3, 'UTC'),
    received_at   DateTime64(3, 'UTC'),
    insert_id     UUID,
    name          LowCardinality(String),
    install_id    UUID,
    user_id       String,
    session_id    UUID,
    duration_ms   UInt32,
    environment   LowCardinality(String),
    platform      LowCardinality(String),
    os_name       LowCardinality(String),
    os_version    LowCardinality(String),
    app_version   LowCardinality(String),
    app_build     LowCardinality(String),
    device_model  LowCardinality(String),
    device_type   LowCardinality(String),
    locale        LowCardinality(String),
    country       LowCardinality(String),
    region        String,
    city          String,
    write_key_id  UUID,
    INDEX idx_user_id user_id TYPE bloom_filter(0.01) GRANULARITY 4,
    INDEX idx_session_id session_id TYPE bloom_filter(0.01) GRANULARITY 4,
    INDEX idx_install_id install_id TYPE bloom_filter(0.01) GRANULARITY 4
)
ENGINE = ReplacingMergeTree(received_at)
PARTITION BY toYYYYMM(event_date)
ORDER BY (app_id, event_date, install_id, insert_id)
TTL event_date + INTERVAL 25 MONTH DELETE
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS events;
-- +goose StatementEnd
