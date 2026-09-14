-- +goose Up
-- Local mirror of the hosted PBX's call records. Verimor's CDR API cannot
-- search by number or extension (its filters return unrelated rows), so the
-- poller copies every record here and the panel searches this table. Raw
-- fields are kept verbatim so a row maps back to the API's shape; the derived
-- columns exist for indexing and matching.
CREATE TABLE pbx_cdrs (
    call_uuid          varchar(80) PRIMARY KEY,
    start_at           timestamptz NOT NULL,
    start_stamp        varchar(40) NOT NULL,
    direction          varchar(16) NOT NULL,           -- normalized: inbound / outbound / internal
    raw_direction      varchar(32) NOT NULL DEFAULT '',
    caller_id_number   varchar(80) NOT NULL DEFAULT '',
    destination_number varchar(80) NOT NULL DEFAULT '',
    caller_num         varchar(32) NOT NULL DEFAULT '', -- digits of the caller's number (external or DID)
    dest_num           varchar(32) NOT NULL DEFAULT '', -- digits of the callee's number
    caller_ext         varchar(16) NOT NULL DEFAULT '', -- extension on the caller side, if any
    dest_ext           varchar(16) NOT NULL DEFAULT '', -- extension on the callee side, if any
    duration           varchar(16) NOT NULL DEFAULT '',
    talk_duration      varchar(16) NOT NULL DEFAULT '',
    answer_stamp       varchar(40) NOT NULL DEFAULT '',
    result             varchar(64) NOT NULL DEFAULT '',
    missed             boolean NOT NULL DEFAULT false,
    recording_present  boolean NOT NULL DEFAULT false,
    fetched_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_pbx_cdrs_start ON pbx_cdrs (start_at DESC);
CREATE INDEX idx_pbx_cdrs_caller_ext ON pbx_cdrs (caller_ext, start_at DESC) WHERE caller_ext <> '';
CREATE INDEX idx_pbx_cdrs_dest_ext ON pbx_cdrs (dest_ext, start_at DESC) WHERE dest_ext <> '';
CREATE INDEX idx_pbx_cdrs_caller_num ON pbx_cdrs (caller_num);
CREATE INDEX idx_pbx_cdrs_dest_num ON pbx_cdrs (dest_num);

-- Backfill progress: days on or after this date are fully mirrored.
INSERT INTO system_settings (key, value) VALUES ('cdr_mirror_cursor', '') ON CONFLICT (key) DO NOTHING;

-- +goose Down
DELETE FROM system_settings WHERE key = 'cdr_mirror_cursor';
DROP TABLE pbx_cdrs;
