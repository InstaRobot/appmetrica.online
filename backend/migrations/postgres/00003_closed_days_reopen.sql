-- +goose Up
-- Заявки на переоткрытие закрытых дней приложения: смена часового пояса и удаление данных человека меняют уже
-- закрытые дни, и job close_day закрывает их заново. Заявка одна на приложение, from_day — самый ранний день.
CREATE TABLE closed_days_reopen (
    app_id       uuid        PRIMARY KEY REFERENCES apps (id) ON DELETE CASCADE,
    from_day     date        NOT NULL,
    requested_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE closed_days_reopen;
