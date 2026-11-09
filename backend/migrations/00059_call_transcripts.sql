-- +goose Up
-- Spoken lines captured during a softphone call, separated by speaker
-- (agent or customer) and stored when the call ends so the wrap-up card
-- and the call history can show what was said.
CREATE TABLE call_transcripts (
    id          bigserial PRIMARY KEY,
    call_id     varchar(80) NOT NULL,
    user_id     bigint REFERENCES users (id) ON DELETE SET NULL,
    direction   varchar(10) NOT NULL DEFAULT 'inbound',
    peer_number varchar(40) NOT NULL DEFAULT '',
    peer_key    varchar(20) NOT NULL DEFAULT '',
    started_at  timestamptz NOT NULL DEFAULT now(),
    ended_at    timestamptz NOT NULL DEFAULT now(),
    entries     jsonb NOT NULL DEFAULT '[]',
    full_text   text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_call_transcripts_call_id ON call_transcripts (call_id);
CREATE INDEX idx_call_transcripts_peer_key_started ON call_transcripts (peer_key, started_at DESC);
CREATE INDEX idx_call_transcripts_user_started ON call_transcripts (user_id, started_at DESC);

-- +goose Down
DROP TABLE call_transcripts;
