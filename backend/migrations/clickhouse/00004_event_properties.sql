-- +goose NO TRANSACTION
-- +goose Up
-- +goose StatementBegin
ALTER TABLE events ADD COLUMN IF NOT EXISTS properties Map(LowCardinality(String), String);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE events DROP COLUMN IF EXISTS properties;
-- +goose StatementEnd
