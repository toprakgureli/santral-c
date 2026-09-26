-- +goose Up
-- "Puanlamalar": the page with every score customers gave. Roles that see
-- the WhatsApp reports see it too; it can be taken away separately.
INSERT INTO permissions (key, module, description)
VALUES ('whatsapp.ratings', 'whatsapp', 'Müşterilerin verdiği bütün puanları ve yorumları görür (Puanlamalar)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT rp.role_id, (SELECT id FROM permissions WHERE key = 'whatsapp.ratings')
FROM role_permissions rp JOIN permissions p ON p.id = rp.permission_id
WHERE p.key = 'whatsapp.reports'
ON CONFLICT DO NOTHING;

CREATE INDEX IF NOT EXISTS wa_tickets_rated ON wa_tickets (rated_at) WHERE rating IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS wa_tickets_rated;
DELETE FROM permissions WHERE key = 'whatsapp.ratings';
