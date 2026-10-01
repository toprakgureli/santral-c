-- +goose Up
-- The browser and address that handed in a refresh token when it was
-- replaced. If the same browser comes back from the same address with
-- that old token, the answer carrying its new token was lost on the way (a
-- dropped connection, a page reloaded at that moment): it gets a fresh
-- token instead of having its session ended as if the token had been
-- copied. Rows from before have neither and keep the strict rule.
ALTER TABLE session_spent_tokens ADD COLUMN device varchar(64) NOT NULL DEFAULT '';
ALTER TABLE session_spent_tokens ADD COLUMN ip varchar(64) NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE session_spent_tokens DROP COLUMN ip;
ALTER TABLE session_spent_tokens DROP COLUMN device;
