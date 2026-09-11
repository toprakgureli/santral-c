-- +goose Up
-- Presence transition log, so we can report how long an agent spent in each
-- state today (in addition to the live "since" of the current state).
CREATE TABLE presence_events (
    id         bigserial PRIMARY KEY,
    user_id    bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    state      varchar(16) NOT NULL
               CHECK (state IN ('available', 'break', 'backoffice', 'dnd')),
    started_at timestamptz NOT NULL DEFAULT now(),
    ended_at   timestamptz
);
CREATE INDEX idx_presence_events_user ON presence_events (user_id, started_at DESC);
CREATE UNIQUE INDEX idx_presence_events_open ON presence_events (user_id) WHERE ended_at IS NULL;

-- +goose Down
DROP TABLE presence_events;
