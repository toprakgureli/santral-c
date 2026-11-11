-- +goose Up
INSERT INTO permissions (key, module, description)
VALUES ('whatsapp.missed_call_recovery', 'whatsapp', 'Kaçan çağrılara WhatsApp''tan otomatik bilgilendirme şablonunu ayarlar')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT rp.role_id, (SELECT id FROM permissions WHERE key = 'whatsapp.missed_call_recovery')
FROM role_permissions rp JOIN permissions p ON p.id = rp.permission_id
WHERE p.key = 'whatsapp.call_survey_manage'
ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS wa_missed_call_recoveries (
    id          bigserial PRIMARY KEY,
    call_uuid   varchar(80) NOT NULL,
    peer_number varchar(40) NOT NULL,
    peer_key    varchar(20) NOT NULL,
    channel_id  bigint REFERENCES wa_channels (id) ON DELETE SET NULL,
    template    varchar(120) NOT NULL DEFAULT '',
    status      varchar(20) NOT NULL DEFAULT 'sent',
    note        text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_wa_missed_call_recoveries_call ON wa_missed_call_recoveries (call_uuid);
CREATE INDEX IF NOT EXISTS idx_wa_missed_call_recoveries_peer_time ON wa_missed_call_recoveries (peer_key, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_wa_missed_call_recoveries_status_time ON wa_missed_call_recoveries (status, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS wa_missed_call_recoveries;
DELETE FROM permissions WHERE key = 'whatsapp.missed_call_recovery';
