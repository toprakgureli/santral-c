-- +goose Up
-- A second per-agent WhatsApp text, used while a call is in progress ("we are
-- on the phone right now, you can send files here"). Empty means the default.
ALTER TABLE users ADD COLUMN whatsapp_template_live text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE users DROP COLUMN whatsapp_template_live;
