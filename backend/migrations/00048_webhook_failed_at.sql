-- +goose Up
-- When a Meta notice was given up on, so the alerts and the system warnings
-- count only the notices that failed lately; one old notice nobody retried
-- no longer keeps them on for ever. Notices that failed before this column
-- existed take the time of their last try.
ALTER TABLE wa_webhook_events ADD COLUMN failed_at timestamptz;
UPDATE wa_webhook_events SET failed_at = LEAST(next_try_at, now()) WHERE status = 'failed';
CREATE INDEX wa_webhook_events_failed_idx ON wa_webhook_events (failed_at) WHERE status = 'failed';

-- +goose Down
DROP INDEX IF EXISTS wa_webhook_events_failed_idx;
ALTER TABLE wa_webhook_events DROP COLUMN failed_at;
