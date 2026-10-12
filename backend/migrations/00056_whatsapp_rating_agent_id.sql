-- +goose Up
-- The agent who had the conversation when the score was given, kept here
-- so reopening or other agents later cannot move the score to anyone else.
ALTER TABLE wa_tickets ADD COLUMN IF NOT EXISTS rating_agent_id integer;

-- +goose Down
ALTER TABLE wa_tickets DROP COLUMN IF EXISTS rating_agent_id;
