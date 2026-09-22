-- +goose Up
-- Profile text, as in Devtrack: a one-line headline and a free biography.
ALTER TABLE users
    ADD COLUMN headline varchar(120) NOT NULL DEFAULT '',
    ADD COLUMN bio text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE users
    DROP COLUMN bio,
    DROP COLUMN headline;
