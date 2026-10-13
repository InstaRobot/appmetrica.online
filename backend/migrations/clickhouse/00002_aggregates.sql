-- +goose NO TRANSACTION
-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS install_first_seen
(
    app_id      UUID,
    environment LowCardinality(String),
    install_id  UUID,
    first_seen  SimpleAggregateFunction(min, DateTime64(3, 'UTC')),
    last_seen   SimpleAggregateFunction(max, DateTime64(3, 'UTC'))
)
ENGINE = AggregatingMergeTree
ORDER BY (app_id, environment, install_id)
TTL toDate(last_seen) + INTERVAL 25 MONTH DELETE;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE MATERIALIZED VIEW IF NOT EXISTS install_first_seen_mv TO install_first_seen AS
SELECT app_id, environment, install_id, min(event_time) AS first_seen, max(event_time) AS last_seen
FROM events
GROUP BY app_id, environment, install_id;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS user_first_seen
(
    app_id      UUID,
    environment LowCardinality(String),
    user_id     String,
    first_seen  SimpleAggregateFunction(min, DateTime64(3, 'UTC')),
    last_seen   SimpleAggregateFunction(max, DateTime64(3, 'UTC'))
)
ENGINE = AggregatingMergeTree
ORDER BY (app_id, environment, user_id)
TTL toDate(last_seen) + INTERVAL 25 MONTH DELETE;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE MATERIALIZED VIEW IF NOT EXISTS user_first_seen_mv TO user_first_seen AS
SELECT app_id, environment, user_id, min(event_time) AS first_seen, max(event_time) AS last_seen
FROM events
WHERE user_id != ''
GROUP BY app_id, environment, user_id;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS identity_links
(
    app_id          UUID,
    install_id      UUID,
    first_user      AggregateFunction(argMin, String, DateTime64(3, 'UTC')),
    first_linked_at SimpleAggregateFunction(min, DateTime64(3, 'UTC')),
    last_seen_at    SimpleAggregateFunction(max, DateTime64(3, 'UTC'))
)
ENGINE = AggregatingMergeTree
ORDER BY (app_id, install_id)
TTL toDate(last_seen_at) + INTERVAL 25 MONTH DELETE;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE MATERIALIZED VIEW IF NOT EXISTS identity_links_mv TO identity_links AS
SELECT app_id, install_id,
       argMinState(user_id, event_time) AS first_user,
       min(event_time) AS first_linked_at,
       max(event_time) AS last_seen_at
FROM events
WHERE user_id != ''
GROUP BY app_id, install_id;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP VIEW IF EXISTS identity_links_mv;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS identity_links;
-- +goose StatementEnd
-- +goose StatementBegin
DROP VIEW IF EXISTS user_first_seen_mv;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS user_first_seen;
-- +goose StatementEnd
-- +goose StatementBegin
DROP VIEW IF EXISTS install_first_seen_mv;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS install_first_seen;
-- +goose StatementEnd
