-- +goose Up
-- A call whose end never reached the panel used to be closed as a two-hour
-- conversation. Its length now comes from the phone system's record when one
-- matches; otherwise it is marked unknown and left out of talk time and of
-- the short/long counts. Calls already closed that way (ended exactly two
-- hours after they started, with two hours of talk) are marked too.
ALTER TABLE call_logs ADD COLUMN duration_unknown boolean NOT NULL DEFAULT false;

UPDATE call_logs SET duration_unknown = true, duration_seconds = 0
WHERE answered_at IS NOT NULL
  AND ended_at = started_at + interval '2 hours'
  AND duration_seconds >= 7200 - 1;

-- +goose Down
UPDATE call_logs
SET duration_seconds = GREATEST(0, EXTRACT(EPOCH FROM (ended_at - answered_at))::int)
WHERE duration_unknown AND answered_at IS NOT NULL AND ended_at IS NOT NULL;
ALTER TABLE call_logs DROP COLUMN duration_unknown;
