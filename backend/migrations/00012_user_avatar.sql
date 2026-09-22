-- +goose Up
-- Profile photo, stored as a webp data URI the panel produced (256x256, a
-- few tens of KB). Kept in the row like Devtrack does, so backups carry it
-- and no file store is needed. Served by GET /users/:id/avatar.
ALTER TABLE users ADD COLUMN avatar text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE users DROP COLUMN avatar;
