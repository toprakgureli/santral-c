-- +goose Up
-- The written answers of a survey form, each under its question, so they
-- read as a form and not as one run-on comment.
ALTER TABLE wa_tickets ADD COLUMN rating_texts jsonb NOT NULL DEFAULT '[]';
ALTER TABLE wa_call_surveys ADD COLUMN texts jsonb NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE wa_call_surveys DROP COLUMN texts;
ALTER TABLE wa_tickets DROP COLUMN rating_texts;
