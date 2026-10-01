-- +goose Up
-- Wrong passwords are counted per browser as well as per account, because
-- a whole office signs in from one public address. The browser's id is kept
-- with each attempt so the security page can show it.
ALTER TABLE login_attempts ADD COLUMN device varchar(64) NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE login_attempts DROP COLUMN device;
