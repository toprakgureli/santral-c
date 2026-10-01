-- +goose Up
-- A refresh token stays usable for a short while after it was replaced, so
-- two browser tabs that renew at the same moment do not sign each other
-- out. The previous hash and the time of the change are kept for that.
ALTER TABLE sessions ADD COLUMN previous_hash varchar(64);
ALTER TABLE sessions ADD COLUMN rotated_at timestamptz;
CREATE INDEX sessions_previous_hash_idx ON sessions (previous_hash) WHERE previous_hash IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS sessions_previous_hash_idx;
ALTER TABLE sessions DROP COLUMN rotated_at;
ALTER TABLE sessions DROP COLUMN previous_hash;
