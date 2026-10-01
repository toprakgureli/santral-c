-- +goose Up
-- Every refresh token a session has replaced. A replaced token that comes
-- back after the short grace for a second tab was copied by someone: the
-- session it belonged to is ended, so whoever holds it must sign in again.
-- Rows older than the longest session are removed when someone signs in.
CREATE TABLE session_spent_tokens (
    token_hash varchar(64) PRIMARY KEY,
    session_id bigint      NOT NULL REFERENCES sessions (id) ON DELETE CASCADE,
    spent_at   timestamptz NOT NULL
);
CREATE INDEX session_spent_tokens_session_idx ON session_spent_tokens (session_id);
CREATE INDEX session_spent_tokens_spent_idx ON session_spent_tokens (spent_at);

-- +goose Down
DROP TABLE session_spent_tokens;
