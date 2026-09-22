-- +goose Up
-- Each agent's own WhatsApp follow-up message (sent after an unreached call
-- from the call manager). Empty means the panel's default text.
ALTER TABLE users ADD COLUMN whatsapp_template text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE users DROP COLUMN whatsapp_template;
