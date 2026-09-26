-- +goose Up
-- Each question of a survey form with its own score, so the Puanlamalar
-- page can show an average per question (speed, care, solution...).
ALTER TABLE wa_tickets ADD COLUMN rating_answers jsonb NOT NULL DEFAULT '[]';
ALTER TABLE wa_call_surveys ADD COLUMN answers jsonb NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE wa_call_surveys DROP COLUMN answers;
ALTER TABLE wa_tickets DROP COLUMN rating_answers;
