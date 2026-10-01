-- +goose Up
-- A call waiting for the phone system's record is looked for again later
-- each time it is missing, so old calls the records never show do not
-- hold up newer ones.
ALTER TABLE call_logs ADD COLUMN hooks_next_at timestamptz;
ALTER TABLE call_logs ADD COLUMN hooks_tries integer NOT NULL DEFAULT 0;

-- Nothing follows a missed incoming ring or a call between colleagues:
-- those stop waiting. The rest are looked for at the next pass.
UPDATE call_logs SET hooks_done = true
 WHERE hooks_done = false AND (direction = 'internal' OR (direction <> 'outbound' AND disposition <> 'answered'));
UPDATE call_logs SET hooks_next_at = ended_at WHERE hooks_done = false;

DROP INDEX IF EXISTS idx_call_logs_hooks_pending;
CREATE INDEX idx_call_logs_hooks_pending ON call_logs (hooks_tries, hooks_next_at) WHERE hooks_done = false;

-- +goose Down
DROP INDEX IF EXISTS idx_call_logs_hooks_pending;
CREATE INDEX idx_call_logs_hooks_pending ON call_logs (ended_at) WHERE hooks_done = false;
ALTER TABLE call_logs DROP COLUMN hooks_tries;
ALTER TABLE call_logs DROP COLUMN hooks_next_at;
