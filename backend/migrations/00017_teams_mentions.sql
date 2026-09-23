-- +goose Up
-- Who a line tags with @: one row per tagged person, plus an "@herkes" flag.
ALTER TABLE chat_messages
    ADD COLUMN mentions_all boolean NOT NULL DEFAULT false;

CREATE TABLE chat_mentions (
    message_id bigint NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
    user_id    bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (message_id, user_id)
);
CREATE INDEX chat_mentions_user_idx ON chat_mentions (user_id, message_id DESC);

-- +goose Down
DROP TABLE chat_mentions;
ALTER TABLE chat_messages DROP COLUMN mentions_all;
