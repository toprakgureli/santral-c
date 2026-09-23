-- +goose Up
-- When each person received and read each line, for the message info
-- window. Written as the seat pointers move; the sender's own lines are
-- never logged.
CREATE TABLE chat_receipts (
    message_id   bigint NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
    user_id      bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    delivered_at timestamptz,
    read_at      timestamptz,
    PRIMARY KEY (message_id, user_id)
);

-- +goose Down
DROP TABLE chat_receipts;
