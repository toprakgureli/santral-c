-- +goose Up
-- Two ways to silence a room: "mentions" (only @ tags get through) and
-- "all" (nothing does). The old boolean stays in step for the badge count.
ALTER TABLE chat_members
    ADD COLUMN mute varchar(10) NOT NULL DEFAULT 'none';
UPDATE chat_members SET mute = 'mentions' WHERE muted;

-- +goose Down
ALTER TABLE chat_members DROP COLUMN mute;
