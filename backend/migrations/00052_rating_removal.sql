-- +goose Up
-- A score taken out of the ratings keeps its original values here, with who
-- removed it and when; the score itself is emptied so every report and
-- average leaves it out without knowing about removals.
ALTER TABLE wa_tickets ADD COLUMN rating_removed jsonb;
ALTER TABLE wa_call_surveys ADD COLUMN removed jsonb;

-- +goose Down
ALTER TABLE wa_call_surveys DROP COLUMN removed;
ALTER TABLE wa_tickets DROP COLUMN rating_removed;
