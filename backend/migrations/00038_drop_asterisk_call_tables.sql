-- +goose Up
-- The call tables from the time the panel talked to Asterisk directly have
-- not been used since calls moved to the hosted PBX (calls now live in
-- call_logs and pbx_cdrs). They are dropped; should a server still hold
-- rows in them, the tables are kept under a legacy_ name instead, so no
-- data is lost.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM calls) OR EXISTS (SELECT 1 FROM call_events) OR EXISTS (SELECT 1 FROM call_quality) THEN
        ALTER TABLE call_quality RENAME TO legacy_call_quality;
        ALTER TABLE call_events RENAME TO legacy_call_events;
        ALTER TABLE calls RENAME TO legacy_calls;
    ELSE
        DROP TABLE call_quality;
        DROP TABLE call_events;
        DROP TABLE calls;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF to_regclass('legacy_calls') IS NOT NULL THEN
        ALTER TABLE legacy_calls RENAME TO calls;
        ALTER TABLE legacy_call_events RENAME TO call_events;
        ALTER TABLE legacy_call_quality RENAME TO call_quality;
    END IF;
END $$;
-- +goose StatementEnd
CREATE TABLE IF NOT EXISTS calls (
    id                bigserial PRIMARY KEY,
    linkedid          varchar(80) NOT NULL,            -- Asterisk Linkedid
    direction         varchar(10) NOT NULL
                      CHECK (direction IN ('inbound', 'outbound', 'internal')),
    disposition       varchar(16) NOT NULL DEFAULT 'in_progress'
                      CHECK (disposition IN ('in_progress', 'answered', 'no_answer',
                                             'busy', 'failed', 'canceled', 'voicemail')),
    from_number       varchar(32) NOT NULL,
    to_number         varchar(32) NOT NULL,
    from_user_id      bigint REFERENCES users (id) ON DELETE SET NULL,     -- outbound/internal caller
    to_user_id        bigint REFERENCES users (id) ON DELETE SET NULL,     -- answering agent
    contact_id        bigint REFERENCES contacts (id) ON DELETE SET NULL,  -- matched external party
    trunk             varchar(60),                     -- e.g. verimor
    started_at        timestamptz NOT NULL DEFAULT now(),
    answered_at       timestamptz,
    ended_at          timestamptz,
    ring_seconds      integer NOT NULL DEFAULT 0,
    talk_seconds      integer NOT NULL DEFAULT 0,
    hangup_cause_code integer,                          -- Q.850 cause code
    hangup_cause_text varchar(80),
    hangup_by         varchar(10)
                      CHECK (hangup_by IN ('caller', 'callee', 'system')),
    recording_path    varchar(255),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_calls_linkedid ON calls (linkedid);
CREATE INDEX IF NOT EXISTS idx_calls_direction ON calls (direction);
CREATE INDEX IF NOT EXISTS idx_calls_disposition ON calls (disposition);
CREATE INDEX IF NOT EXISTS idx_calls_started_at ON calls (started_at DESC);
CREATE INDEX IF NOT EXISTS idx_calls_from_user_id ON calls (from_user_id);
CREATE INDEX IF NOT EXISTS idx_calls_to_user_id ON calls (to_user_id);
CREATE INDEX IF NOT EXISTS idx_calls_contact_id ON calls (contact_id);
CREATE INDEX IF NOT EXISTS idx_calls_from_number ON calls (from_number);
CREATE INDEX IF NOT EXISTS idx_calls_to_number ON calls (to_number);

-- Ordered event timeline for one call. Hangups carry the Q.850 cause and the
-- releasing party in detail; rtp_timeout and quality_alert capture media faults.
CREATE TABLE IF NOT EXISTS call_events (
    id         bigserial PRIMARY KEY,
    call_id    bigint NOT NULL REFERENCES calls (id) ON DELETE CASCADE,
    seq        integer NOT NULL DEFAULT 0,             -- order within the call
    type       varchar(40) NOT NULL,                  -- ringing, answered, hold, transfer, dtmf, hangup, rtp_timeout, quality_alert
    channel    varchar(80),                           -- Asterisk channel this event belongs to
    at         timestamptz NOT NULL DEFAULT now(),
    detail     jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_call_events_call_id_at ON call_events (call_id, at);
CREATE INDEX IF NOT EXISTS idx_call_events_call_id_seq ON call_events (call_id, seq);
CREATE INDEX IF NOT EXISTS idx_call_events_type ON call_events (type);

-- Periodic RTCP snapshots per media leg, plus an estimated MOS, so audio
-- problems are measured and logged rather than merely felt.
CREATE TABLE IF NOT EXISTS call_quality (
    id                bigserial PRIMARY KEY,
    call_id           bigint NOT NULL REFERENCES calls (id) ON DELETE CASCADE,
    leg               varchar(10) NOT NULL
                      CHECK (leg IN ('agent', 'trunk')),
    channel           varchar(80),
    codec             varchar(20),
    at                timestamptz NOT NULL DEFAULT now(),
    jitter_ms         numeric(8,2),
    rtt_ms            numeric(8,2),
    loss_pct          numeric(5,2),
    mos               numeric(3,2),                    -- estimated MOS, 1.00-5.00
    packets_sent      bigint NOT NULL DEFAULT 0,
    packets_received  bigint NOT NULL DEFAULT 0,
    packets_lost      bigint NOT NULL DEFAULT 0,
    created_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_call_quality_call_id_at ON call_quality (call_id, at);
