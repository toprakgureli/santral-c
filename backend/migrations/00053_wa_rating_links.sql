-- +goose Up
-- Links that open the ratings without signing in, for someone outside the
-- panel. The link carries a signature over the row's id, nonce and end;
-- the row says who made it, until when it works and whether it was
-- cancelled, and counts how often it was opened.
CREATE TABLE wa_rating_links (
    id             bigserial PRIMARY KEY,
    nonce          varchar(64) NOT NULL,
    label          varchar(120) NOT NULL DEFAULT '',
    created_by     bigint NOT NULL REFERENCES users(id),
    created_at     timestamptz NOT NULL DEFAULT now(),
    expires_at     timestamptz NOT NULL,
    revoked_at     timestamptz,
    revoked_by     bigint REFERENCES users(id),
    open_count     integer NOT NULL DEFAULT 0,
    last_opened_at timestamptz
);
CREATE INDEX wa_rating_links_expires_idx ON wa_rating_links (expires_at);

-- +goose Down
DROP TABLE wa_rating_links;
