-- +goose Up
-- Notices for one person, shown on any page of the panel until read.
CREATE TABLE user_notices (
    id         bigserial PRIMARY KEY,
    user_id    bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind       varchar(32) NOT NULL,
    text       varchar(500) NOT NULL,
    link       varchar(200) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    read_at    timestamptz
);
CREATE INDEX user_notices_user_idx ON user_notices (user_id, id DESC);

-- A number someone called and could not reach. One open row per person and
-- number; it closes by itself once anyone has a real conversation with the
-- number, in either direction, or by hand when no call back is needed.
CREATE TABLE call_unreached (
    id                bigserial PRIMARY KEY,
    peer_key          varchar(20) NOT NULL,
    peer_number       varchar(40) NOT NULL,
    user_id           bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    attempts          integer NOT NULL DEFAULT 1,
    first_at          timestamptz NOT NULL,
    last_at           timestamptz NOT NULL,
    last_call_id      varchar(80) NOT NULL DEFAULT '',
    last_reason       varchar(16) NOT NULL DEFAULT '',
    status            varchar(10) NOT NULL DEFAULT 'open',
    reached_at        timestamptz,
    reached_by        bigint REFERENCES users(id) ON DELETE SET NULL,
    reached_direction varchar(10),
    reached_seconds   integer,
    reached_call_id   varchar(80),
    claimed_by        bigint REFERENCES users(id) ON DELETE SET NULL,
    claimed_until     timestamptz,
    dropped_by        bigint REFERENCES users(id) ON DELETE SET NULL,
    dropped_at        timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX call_unreached_open_idx ON call_unreached (peer_key, user_id) WHERE status = 'open';
CREATE INDEX call_unreached_peer_idx ON call_unreached (peer_key, status);
CREATE INDEX call_unreached_last_idx ON call_unreached (last_at DESC);

-- +goose Down
DROP TABLE call_unreached;
DROP TABLE user_notices;
