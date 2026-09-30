-- +goose Up
-- A call survey the sender has taken is marked 'sending' with the time it
-- was taken, so it is sent once even when its outcome cannot be stored.
ALTER TABLE wa_call_surveys ADD COLUMN claimed_at timestamptz;

-- +goose Down
UPDATE wa_call_surveys SET status = 'queued' WHERE status = 'sending';
ALTER TABLE wa_call_surveys DROP COLUMN claimed_at;
