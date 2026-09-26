-- +goose Up
-- What fills each blank of a template when someone sends it: the
-- customer's name, the sender's first name or full name, or nothing (typed
-- by hand). Kept by the panel; Meta knows nothing about it, so refreshing
-- templates from Meta leaves it alone.
ALTER TABLE wa_templates ADD COLUMN fill jsonb NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE wa_templates DROP COLUMN fill;
