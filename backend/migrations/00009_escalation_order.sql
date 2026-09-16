-- +goose Up
-- Managers order the escalation catalog by hand; the panel lists categories
-- and reasons in that order instead of alphabetically. Existing rows keep
-- their alphabetical order as the starting point; rows that never received an
-- order (0) sort after the ordered ones, by name.
ALTER TABLE escalation_categories ADD COLUMN sort_order integer NOT NULL DEFAULT 0;
ALTER TABLE escalation_reasons ADD COLUMN sort_order integer NOT NULL DEFAULT 0;

WITH o AS (
    SELECT id, row_number() OVER (ORDER BY lower(name)) AS rn
    FROM escalation_categories
    WHERE deleted_at IS NULL
)
UPDATE escalation_categories c SET sort_order = o.rn FROM o WHERE c.id = o.id;

WITH o AS (
    SELECT id, row_number() OVER (PARTITION BY category_id ORDER BY lower(name)) AS rn
    FROM escalation_reasons
    WHERE deleted_at IS NULL
)
UPDATE escalation_reasons r SET sort_order = o.rn FROM o WHERE r.id = o.id;

-- +goose Down
ALTER TABLE escalation_reasons DROP COLUMN sort_order;
ALTER TABLE escalation_categories DROP COLUMN sort_order;
