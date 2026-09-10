-- +goose Up
-- Panel-owned operational data: our own call log (so history no longer depends
-- on the rate-limited hosted CDR API) and persisted agent presence (so a paused
-- agent shows as paused everywhere, including in the live agent list).

-- One row per call the panel's softphone handles, created when the call starts
-- and finalized when it ends. call_id is a client-generated correlation id.
CREATE TABLE call_logs (
    id               bigserial PRIMARY KEY,
    call_id          varchar(80) NOT NULL,
    user_id          bigint REFERENCES users (id) ON DELETE SET NULL,
    direction        varchar(10) NOT NULL
                     CHECK (direction IN ('inbound', 'outbound', 'internal')),
    peer_number      varchar(40) NOT NULL,
    peer_key         varchar(20) NOT NULL,              -- last ten digits, for matching
    disposition      varchar(16) NOT NULL DEFAULT 'in_progress'
                     CHECK (disposition IN ('in_progress', 'answered', 'no_answer',
                                            'missed', 'busy', 'failed', 'canceled')),
    started_at       timestamptz NOT NULL DEFAULT now(),
    answered_at      timestamptz,
    ended_at         timestamptz,
    duration_seconds integer NOT NULL DEFAULT 0,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_call_logs_call_id ON call_logs (call_id);
CREATE INDEX idx_call_logs_user_started ON call_logs (user_id, started_at DESC);
CREATE INDEX idx_call_logs_started ON call_logs (started_at DESC);
CREATE INDEX idx_call_logs_peer_key ON call_logs (peer_key);

-- Persisted per-agent presence. DND is still pushed to the hosted PBX; this row
-- is the source of truth the panel overlays onto the live agent list.
CREATE TABLE agent_presence (
    user_id    bigint PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    state      varchar(16) NOT NULL DEFAULT 'available'
               CHECK (state IN ('available', 'break', 'backoffice', 'dnd')),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE agent_presence;
DROP TABLE call_logs;
