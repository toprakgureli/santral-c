-- +goose Up
-- Telephony core: CRM contacts, the call record (CDR) for inbound and
-- outbound calls, a per-call event timeline (drop diagnostics, transfers,
-- DTMF, quality alerts) and periodic RTCP audio-quality samples.

-- CRM foundation. A contact is an external party; calls link back to it.
CREATE TABLE contacts (
    id         bigserial PRIMARY KEY,
    name       varchar(160) NOT NULL,
    company    varchar(160),
    email      varchar(255),
    notes      text,
    created_by bigint REFERENCES users (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE INDEX idx_contacts_name ON contacts (lower(name));
CREATE INDEX idx_contacts_created_by ON contacts (created_by);
CREATE INDEX idx_contacts_deleted_at ON contacts (deleted_at);

-- One contact can hold several numbers; a number maps to one contact so
-- inbound calls resolve by caller id.
CREATE TABLE contact_phones (
    id          bigserial PRIMARY KEY,
    contact_id  bigint NOT NULL REFERENCES contacts (id) ON DELETE CASCADE,
    label       varchar(20) NOT NULL DEFAULT 'other'
                CHECK (label IN ('mobile', 'work', 'home', 'other')),
    number_e164 varchar(20) NOT NULL,             -- normalized E.164, e.g. +905321112233
    is_primary  boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_contact_phones_number ON contact_phones (number_e164);
CREATE INDEX idx_contact_phones_contact_id ON contact_phones (contact_id);

-- One row per call, correlated to Asterisk by linkedid (which ties the
-- channels of a single call together).
CREATE TABLE calls (
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
CREATE UNIQUE INDEX idx_calls_linkedid ON calls (linkedid);
CREATE INDEX idx_calls_direction ON calls (direction);
CREATE INDEX idx_calls_disposition ON calls (disposition);
CREATE INDEX idx_calls_started_at ON calls (started_at DESC);
CREATE INDEX idx_calls_from_user_id ON calls (from_user_id);
CREATE INDEX idx_calls_to_user_id ON calls (to_user_id);
CREATE INDEX idx_calls_contact_id ON calls (contact_id);
CREATE INDEX idx_calls_from_number ON calls (from_number);
CREATE INDEX idx_calls_to_number ON calls (to_number);

-- Ordered event timeline for one call. Hangups carry the Q.850 cause and the
-- releasing party in detail; rtp_timeout and quality_alert capture media faults.
CREATE TABLE call_events (
    id         bigserial PRIMARY KEY,
    call_id    bigint NOT NULL REFERENCES calls (id) ON DELETE CASCADE,
    seq        integer NOT NULL DEFAULT 0,             -- order within the call
    type       varchar(40) NOT NULL,                  -- ringing, answered, hold, transfer, dtmf, hangup, rtp_timeout, quality_alert
    channel    varchar(80),                           -- Asterisk channel this event belongs to
    at         timestamptz NOT NULL DEFAULT now(),
    detail     jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_call_events_call_id_at ON call_events (call_id, at);
CREATE INDEX idx_call_events_call_id_seq ON call_events (call_id, seq);
CREATE INDEX idx_call_events_type ON call_events (type);

-- Periodic RTCP snapshots per media leg, plus an estimated MOS, so audio
-- problems are measured and logged rather than merely felt.
CREATE TABLE call_quality (
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
CREATE INDEX idx_call_quality_call_id_at ON call_quality (call_id, at);

-- +goose Down
DROP TABLE call_quality;
DROP TABLE call_events;
DROP TABLE calls;
DROP TABLE contact_phones;
DROP TABLE contacts;
