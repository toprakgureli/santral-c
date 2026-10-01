-- +goose Up
-- Database backups to a Google Shared Drive, set up in the panel. One row of
-- settings: the folder, the service account key (sealed with the data key)
-- and whether it runs; and a row for each run, shown on the settings page.
CREATE TABLE backup_settings (
    id              smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    enabled         boolean      NOT NULL DEFAULT false,
    folder_id       varchar(200) NOT NULL DEFAULT '',
    credentials_enc text         NOT NULL DEFAULT '',
    account         varchar(255) NOT NULL DEFAULT '',
    updated_at      timestamptz  NOT NULL DEFAULT now()
);

CREATE TABLE backup_runs (
    id          bigserial PRIMARY KEY,
    started_at  timestamptz  NOT NULL DEFAULT now(),
    finished_at timestamptz,
    ok          boolean      NOT NULL DEFAULT false,
    file        varchar(255) NOT NULL DEFAULT '',
    size        bigint       NOT NULL DEFAULT 0,
    error       text         NOT NULL DEFAULT '',
    started_by  bigint REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX backup_runs_started_idx ON backup_runs (started_at DESC);

-- +goose Down
DROP TABLE backup_runs;
DROP TABLE backup_settings;
