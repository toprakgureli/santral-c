-- +goose Up
-- A call back someone planned for a time. It reminds its owner on every page
-- once due, and closes by itself when anyone has a real conversation with
-- the number.
CREATE TABLE call_reminders (
    id          bigserial PRIMARY KEY,
    user_id     bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    peer_number varchar(40) NOT NULL,
    peer_key    varchar(20) NOT NULL,
    note        varchar(300) NOT NULL DEFAULT '',
    due_at      timestamptz NOT NULL,
    snoozes     integer NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now(),
    done_at     timestamptz,
    done_reason varchar(10),
    done_by     bigint REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX call_reminders_open_idx ON call_reminders (user_id, due_at) WHERE done_at IS NULL;
CREATE INDEX call_reminders_peer_idx ON call_reminders (peer_key) WHERE done_at IS NULL;

-- +goose Down
DROP TABLE call_reminders;
