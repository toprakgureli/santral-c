-- +goose Up
-- Delivery pointer per seat (read pointer already exists) and the last time
-- a person had the chat open, for "son görülme".
ALTER TABLE chat_members
    ADD COLUMN last_delivered_id bigint NOT NULL DEFAULT 0;

CREATE TABLE chat_presence (
    user_id      bigint PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    last_seen_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE chat_presence;
ALTER TABLE chat_members DROP COLUMN last_delivered_id;
