-- +goose Up
CREATE TYPE app_platform AS ENUM ('ios', 'android', 'web', 'macos', 'windows', 'linux', 'bot', 'server', 'other');
CREATE TYPE app_status AS ENUM ('active', 'archived');
CREATE TYPE deletion_subject AS ENUM ('install', 'user', 'app');
CREATE TYPE deletion_status AS ENUM ('pending', 'running', 'done', 'failed');
CREATE TYPE deletion_source AS ENUM ('api', 'dashboard');
CREATE TYPE job_name AS ENUM ('close_day', 'digest', 'system_check', 'deletion', 'session_cleanup');
CREATE TYPE job_status AS ENUM ('running', 'ok', 'failed');
CREATE TYPE ceremony_kind AS ENUM ('registration', 'login');

CREATE TABLE owner (
    id                   uuid        PRIMARY KEY DEFAULT uuidv7(),
    singleton            boolean     NOT NULL DEFAULT true,
    webauthn_user_handle bytea       NOT NULL,
    display_name         text        NOT NULL DEFAULT 'owner',
    created_at           timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT owner_singleton_uq UNIQUE (singleton),
    CONSTRAINT owner_singleton_chk CHECK (singleton),
    CONSTRAINT owner_webauthn_user_handle_uq UNIQUE (webauthn_user_handle),
    CONSTRAINT owner_webauthn_user_handle_len_chk CHECK (octet_length(webauthn_user_handle) = 64)
);

CREATE TABLE passkeys (
    id               uuid        PRIMARY KEY DEFAULT uuidv7(),
    owner_id         uuid        NOT NULL REFERENCES owner (id) ON DELETE CASCADE,
    credential_id    bytea       NOT NULL,
    public_key       bytea       NOT NULL,
    attestation_type text        NOT NULL,
    aaguid           uuid        NOT NULL,
    sign_count       bigint      NOT NULL DEFAULT 0,
    transports       text[]      NOT NULL DEFAULT '{}',
    backup_eligible  boolean     NOT NULL,
    backup_state     boolean     NOT NULL,
    name             text        NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    last_used_at     timestamptz,
    CONSTRAINT passkeys_credential_id_uq UNIQUE (credential_id),
    CONSTRAINT passkeys_name_len_chk CHECK (char_length(name) BETWEEN 1 AND 64),
    CONSTRAINT passkeys_sign_count_chk CHECK (sign_count >= 0)
);
CREATE INDEX passkeys_owner_id_idx ON passkeys (owner_id);

CREATE TABLE passkey_setup_tokens (
    token_hash bytea       PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    CONSTRAINT passkey_setup_tokens_hash_len_chk CHECK (octet_length(token_hash) = 32),
    CONSTRAINT passkey_setup_tokens_expiry_chk CHECK (expires_at > created_at)
);
CREATE INDEX passkey_setup_tokens_expires_at_idx ON passkey_setup_tokens (expires_at);

CREATE TABLE webauthn_ceremonies (
    id               uuid          PRIMARY KEY DEFAULT uuidv7(),
    kind             ceremony_kind NOT NULL,
    session_data     jsonb         NOT NULL,
    setup_token_hash bytea         REFERENCES passkey_setup_tokens (token_hash) ON DELETE CASCADE,
    created_at       timestamptz   NOT NULL DEFAULT now(),
    expires_at       timestamptz   NOT NULL,
    CONSTRAINT webauthn_ceremonies_setup_kind_chk CHECK (setup_token_hash IS NULL OR kind = 'registration')
);
CREATE INDEX webauthn_ceremonies_expires_at_idx ON webauthn_ceremonies (expires_at);
CREATE INDEX webauthn_ceremonies_setup_token_hash_idx ON webauthn_ceremonies (setup_token_hash)
    WHERE setup_token_hash IS NOT NULL;

CREATE TABLE auth_sessions (
    token_hash   bytea       PRIMARY KEY,
    owner_id     uuid        NOT NULL REFERENCES owner (id) ON DELETE CASCADE,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL,
    idle_timeout interval    NOT NULL DEFAULT '7 days',
    user_agent   text        NOT NULL DEFAULT '',
    revoked_at   timestamptz,
    CONSTRAINT auth_sessions_hash_len_chk CHECK (octet_length(token_hash) = 32),
    CONSTRAINT auth_sessions_user_agent_len_chk CHECK (char_length(user_agent) <= 512)
);
CREATE INDEX auth_sessions_owner_id_idx ON auth_sessions (owner_id);
CREATE INDEX auth_sessions_expires_at_idx ON auth_sessions (expires_at);

CREATE TABLE apps (
    id          uuid         PRIMARY KEY DEFAULT uuidv7(),
    name        text         NOT NULL,
    platform    app_platform NOT NULL,
    timezone    text         NOT NULL,
    status      app_status   NOT NULL DEFAULT 'active',
    created_at  timestamptz  NOT NULL DEFAULT now(),
    updated_at  timestamptz  NOT NULL DEFAULT now(),
    archived_at timestamptz,
    CONSTRAINT apps_name_len_chk CHECK (char_length(name) BETWEEN 1 AND 64),
    CONSTRAINT apps_timezone_len_chk CHECK (char_length(timezone) BETWEEN 1 AND 64),
    CONSTRAINT apps_archived_consistency_chk CHECK ((status = 'archived') = (archived_at IS NOT NULL))
);
CREATE UNIQUE INDEX apps_name_lower_uq ON apps (lower(name));

CREATE TABLE write_keys (
    id           uuid        PRIMARY KEY DEFAULT uuidv7(),
    app_id       uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    key          text        NOT NULL,
    label        text        NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    revoked_at   timestamptz,
    last_used_at timestamptz,
    CONSTRAINT write_keys_key_uq UNIQUE (key),
    CONSTRAINT write_keys_key_format_chk CHECK (key ~ '^pk_[A-Za-z0-9]{32}$'),
    CONSTRAINT write_keys_label_len_chk CHECK (char_length(label) <= 64)
);
CREATE INDEX write_keys_app_id_idx ON write_keys (app_id);

CREATE TABLE api_keys (
    id           uuid        PRIMARY KEY DEFAULT uuidv7(),
    key_prefix   text        NOT NULL,
    key_hash     bytea       NOT NULL,
    label        text        NOT NULL,
    scopes       text[]      NOT NULL,
    app_id       uuid        REFERENCES apps (id) ON DELETE CASCADE,
    created_at   timestamptz NOT NULL DEFAULT now(),
    revoked_at   timestamptz,
    last_used_at timestamptz,
    CONSTRAINT api_keys_key_hash_uq UNIQUE (key_hash),
    CONSTRAINT api_keys_key_hash_len_chk CHECK (octet_length(key_hash) = 32),
    CONSTRAINT api_keys_key_prefix_chk CHECK (key_prefix ~ '^sk_[A-Za-z0-9]{8}$'),
    CONSTRAINT api_keys_label_len_chk CHECK (char_length(label) BETWEEN 1 AND 64),
    CONSTRAINT api_keys_scopes_chk CHECK (scopes <@ ARRAY['read', 'delete']::text[] AND cardinality(scopes) > 0)
);
CREATE INDEX api_keys_app_id_idx ON api_keys (app_id) WHERE app_id IS NOT NULL;

CREATE TABLE deletion_requests (
    id            uuid             PRIMARY KEY DEFAULT uuidv7(),
    app_id        uuid             NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    subject_type  deletion_subject NOT NULL,
    subject_value text             NOT NULL,
    status        deletion_status  NOT NULL DEFAULT 'pending',
    source        deletion_source  NOT NULL,
    api_key_id    uuid             REFERENCES api_keys (id) ON DELETE SET NULL,
    requested_at  timestamptz      NOT NULL DEFAULT now(),
    started_at    timestamptz,
    completed_at  timestamptz,
    attempts      integer          NOT NULL DEFAULT 0,
    error         text,
    CONSTRAINT deletion_requests_subject_value_len_chk CHECK (char_length(subject_value) BETWEEN 1 AND 128),
    CONSTRAINT deletion_requests_attempts_chk CHECK (attempts BETWEEN 0 AND 5)
);
CREATE INDEX deletion_requests_status_requested_at_idx ON deletion_requests (status, requested_at);
CREATE INDEX deletion_requests_app_id_idx ON deletion_requests (app_id);
CREATE INDEX deletion_requests_api_key_id_idx ON deletion_requests (api_key_id) WHERE api_key_id IS NOT NULL;
CREATE UNIQUE INDEX deletion_requests_active_subject_uq ON deletion_requests (app_id, subject_type, subject_value)
    WHERE status IN ('pending', 'running');

CREATE TABLE settings (
    singleton        boolean     PRIMARY KEY DEFAULT true,
    owner_timezone   text        NOT NULL DEFAULT 'Europe/Moscow',
    telegram_chat_id text,
    digest_enabled   boolean     NOT NULL DEFAULT false,
    digest_time      time        NOT NULL DEFAULT '09:00',
    alerts_enabled   boolean     NOT NULL DEFAULT true,
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT settings_singleton_chk CHECK (singleton),
    CONSTRAINT settings_telegram_chat_id_chk CHECK (
        telegram_chat_id IS NULL OR telegram_chat_id ~ '^(-?[0-9]{1,20}|@[A-Za-z][A-Za-z0-9_]{4,31})$'),
    CONSTRAINT settings_digest_requires_chat_chk CHECK (NOT digest_enabled OR telegram_chat_id IS NOT NULL)
);
INSERT INTO settings (singleton) VALUES (true);

CREATE TABLE job_runs (
    id          uuid        PRIMARY KEY DEFAULT uuidv7(),
    job         job_name    NOT NULL,
    started_at  timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    status      job_status  NOT NULL DEFAULT 'running',
    details     jsonb       NOT NULL DEFAULT '{}'
);
CREATE INDEX job_runs_job_started_at_idx ON job_runs (job, started_at DESC);

CREATE TABLE alert_states (
    key              text        PRIMARY KEY,
    firing           boolean     NOT NULL DEFAULT false,
    first_fired_at   timestamptz,
    last_notified_at timestamptz,
    last_value       text        NOT NULL DEFAULT '',
    CONSTRAINT alert_states_key_chk CHECK (key IN ('disk_free', 'insert_errors', 'backup_stale', 'ingest_down', 'api_down', 'clickhouse_down'))
);

CREATE TABLE closed_days (
    app_id    uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    day       date        NOT NULL,
    closed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (app_id, day)
);

-- +goose Down
DROP TABLE closed_days;
DROP TABLE alert_states;
DROP TABLE job_runs;
DROP TABLE settings;
DROP TABLE deletion_requests;
DROP TABLE api_keys;
DROP TABLE write_keys;
DROP TABLE apps;
DROP TABLE auth_sessions;
DROP TABLE webauthn_ceremonies;
DROP TABLE passkey_setup_tokens;
DROP TABLE passkeys;
DROP TABLE owner;
DROP TYPE ceremony_kind;
DROP TYPE job_status;
DROP TYPE job_name;
DROP TYPE deletion_source;
DROP TYPE deletion_status;
DROP TYPE deletion_subject;
DROP TYPE app_status;
DROP TYPE app_platform;
