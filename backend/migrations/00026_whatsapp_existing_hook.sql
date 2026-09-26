-- +goose Up
-- A number can use a webhook that is already registered in Meta (for
-- example one an earlier system set up), instead of the address this
-- panel gives. The address and its verification word are entered in the
-- panel. When the app's secret is not at hand, unsigned notices can be
-- accepted for that number, knowingly.
ALTER TABLE wa_channels
    ADD COLUMN existing_hook_url    varchar(500) NOT NULL DEFAULT '',
    ADD COLUMN existing_hook_path   varchar(300) NOT NULL DEFAULT '',
    ADD COLUMN existing_verify_token varchar(200) NOT NULL DEFAULT '',
    ADD COLUMN accept_unsigned      boolean      NOT NULL DEFAULT false;
CREATE INDEX wa_channels_existing_hook ON wa_channels (existing_hook_path) WHERE existing_hook_path <> '';

-- +goose Down
DROP INDEX wa_channels_existing_hook;
ALTER TABLE wa_channels
    DROP COLUMN existing_hook_url,
    DROP COLUMN existing_hook_path,
    DROP COLUMN existing_verify_token,
    DROP COLUMN accept_unsigned;
