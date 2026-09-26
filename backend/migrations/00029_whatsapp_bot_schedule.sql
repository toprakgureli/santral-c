-- +goose Up
-- When a chatbot answers: always, in the device's working hours, outside
-- them, or in hours picked for that chatbot.
ALTER TABLE wa_bots ADD COLUMN schedule jsonb NOT NULL DEFAULT '{"mode":"always","spans":[]}';

-- +goose Down
ALTER TABLE wa_bots DROP COLUMN schedule;
