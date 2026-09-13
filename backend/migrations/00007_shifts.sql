-- +goose Up
-- Work shifts. An agent starts a shift before the dialer opens and ends it when
-- leaving; a shift left open past the evening cutoff is closed by the server
-- (ended_by = 'auto').
CREATE TABLE shifts (
    id         bigserial PRIMARY KEY,
    user_id    bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    started_at timestamptz NOT NULL DEFAULT now(),
    ended_at   timestamptz,
    ended_by   varchar(8) CHECK (ended_by IN ('user', 'auto')),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_shifts_user_started ON shifts (user_id, started_at DESC);
CREATE UNIQUE INDEX idx_shifts_open ON shifts (user_id) WHERE ended_at IS NULL;

-- Off-shift agents show as such in the live agent list instead of as paused.
ALTER TABLE agent_presence DROP CONSTRAINT agent_presence_state_check;
ALTER TABLE agent_presence ADD CONSTRAINT agent_presence_state_check
    CHECK (state IN ('available', 'break', 'backoffice', 'dnd', 'off'));

-- +goose Down
ALTER TABLE agent_presence DROP CONSTRAINT agent_presence_state_check;
UPDATE agent_presence SET state = 'available' WHERE state = 'off';
ALTER TABLE agent_presence ADD CONSTRAINT agent_presence_state_check
    CHECK (state IN ('available', 'break', 'backoffice', 'dnd'));
DROP TABLE shifts;
