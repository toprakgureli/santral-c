-- +goose Up
-- "Okunmadı olarak işaretle": a manual unread flag on the seat, cleared the
-- next time the room is read. Read receipts are untouched by it.
ALTER TABLE chat_members
    ADD COLUMN marked_unread boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE chat_members DROP COLUMN marked_unread;
