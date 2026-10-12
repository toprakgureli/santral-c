-- +goose Up
-- Support BSUID (Business-Scoped User ID) which can be up to 128 characters long
-- for WhatsApp users with usernames and hidden phone numbers.
ALTER TABLE wa_contacts ALTER COLUMN wa_id TYPE varchar(160);
ALTER TABLE wa_call_surveys ALTER COLUMN wa_id TYPE varchar(160);

-- +goose Down
ALTER TABLE wa_call_surveys ALTER COLUMN wa_id TYPE varchar(32);
ALTER TABLE wa_contacts ALTER COLUMN wa_id TYPE varchar(32);
