-- +goose Up
-- The earliest line the person who sent an invite could read themselves.
-- Accepting never shows the newcomer anything before it, whatever the
-- invite's history choice says: nobody hands out more of a room's past
-- than they can see. 0 means the sender could read the whole history.
ALTER TABLE chat_invites ADD COLUMN history_floor bigint NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE chat_invites DROP COLUMN history_floor;
