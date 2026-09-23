-- +goose Up
-- Chat attachments live in Google Drive; only the card (name, size, kind,
-- thumbnail) is kept here. A row is created when an upload starts and
-- bound to a line when the message is sent; rows never bound are swept.
CREATE TABLE chat_attachments (
    id          bigserial PRIMARY KEY,
    group_id    bigint NOT NULL REFERENCES chat_groups(id) ON DELETE CASCADE,
    message_id  bigint REFERENCES chat_messages(id) ON DELETE SET NULL,
    uploader_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind        varchar(8)   NOT NULL DEFAULT 'file',    -- image | video | file
    name        varchar(255) NOT NULL,
    mime        varchar(120) NOT NULL DEFAULT '',
    size        bigint       NOT NULL DEFAULT 0,
    drive_id    varchar(128) NOT NULL DEFAULT '',
    status      varchar(10)  NOT NULL DEFAULT 'pending', -- pending | ready
    width       integer      NOT NULL DEFAULT 0,
    height      integer      NOT NULL DEFAULT 0,
    duration_ms integer      NOT NULL DEFAULT 0,
    thumb       text         NOT NULL DEFAULT '',
    created_at  timestamptz  NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);
CREATE INDEX chat_attachments_message_idx ON chat_attachments (message_id);
CREATE INDEX chat_attachments_orphans_idx ON chat_attachments (created_at) WHERE message_id IS NULL AND deleted_at IS NULL;

-- The encrypted Drive token does not fit the old 200-character column.
ALTER TABLE system_settings ALTER COLUMN value TYPE text;

INSERT INTO system_settings (key, value) VALUES
    ('drive_token', ''),
    ('drive_account', ''),
    ('drive_folder', '')
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DELETE FROM system_settings WHERE key IN ('drive_token', 'drive_account', 'drive_folder');
DROP TABLE chat_attachments;
ALTER TABLE system_settings ALTER COLUMN value TYPE varchar(200);
