-- +goose Up
-- Each room gets its own Drive folder (SantralC / Gruplar / <ad> or
-- SantralC / Özel Mesajlar / <a> - <b>), created on first upload.
ALTER TABLE chat_groups
    ADD COLUMN drive_folder varchar(128) NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE chat_groups DROP COLUMN drive_folder;
