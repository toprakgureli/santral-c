-- +goose Up
-- A message the send queue has taken is marked 'sending' with the time it
-- was taken, so a second worker cannot take it too and a message left in
-- the middle of a send (the server stopped) can be found and flagged
-- instead of being sent twice.
ALTER TABLE wa_messages ADD COLUMN sending_at timestamptz;
CREATE INDEX wa_messages_sending_idx ON wa_messages (sending_at) WHERE status = 'sending';

-- +goose Down
UPDATE wa_messages SET status = 'queued' WHERE status = 'sending';
DROP INDEX wa_messages_sending_idx;
ALTER TABLE wa_messages DROP COLUMN sending_at;
