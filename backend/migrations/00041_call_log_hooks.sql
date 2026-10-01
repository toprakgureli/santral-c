-- +goose Up
-- What follows a call (the automatic escalation entry, the WhatsApp survey)
-- waits until the phone system's own record confirms the call, so a call
-- the panel was only told about cannot trigger a message to any number.
-- Calls that ended before this change count as done.
ALTER TABLE call_logs ADD COLUMN hooks_done boolean NOT NULL DEFAULT true;
CREATE INDEX idx_call_logs_hooks_pending ON call_logs (ended_at) WHERE hooks_done = false;

-- +goose Down
DROP INDEX IF EXISTS idx_call_logs_hooks_pending;
ALTER TABLE call_logs DROP COLUMN hooks_done;
