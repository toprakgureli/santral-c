-- +goose Up
-- A game card is a line of its own kind; the original check only knew
-- text and system, so the card could not be written and nobody saw the
-- lobby.
ALTER TABLE chat_messages DROP CONSTRAINT IF EXISTS chat_messages_kind_check;
ALTER TABLE chat_messages ADD CONSTRAINT chat_messages_kind_check CHECK (kind IN ('text', 'system', 'game'));

-- +goose Down
ALTER TABLE chat_messages DROP CONSTRAINT IF EXISTS chat_messages_kind_check;
ALTER TABLE chat_messages ADD CONSTRAINT chat_messages_kind_check CHECK (kind IN ('text', 'system'));
