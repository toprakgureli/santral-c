-- +goose Up
-- Deleted contacts kept their phone numbers, so the numbers could not be
-- saved on another contact and still resolved to the deleted one. Deleting
-- a contact now frees them; this frees those of contacts deleted before.
DELETE FROM contact_phones
WHERE contact_id IN (SELECT id FROM contacts WHERE deleted_at IS NOT NULL);

-- +goose Down
-- The freed numbers are not restored.
SELECT 1;
