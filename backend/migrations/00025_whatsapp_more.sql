-- +goose Up
-- WhatsApp, second part: files uploaded for chatbots and templates, the ad
-- a customer came from, module-wide settings (the reply assistant, the
-- survey after a phone call) and the surveys sent after calls.

-- A file uploaded from the panel. The bytes live in the connected storage;
-- the row only says where and what it is.
CREATE TABLE wa_files (
    id          bigserial PRIMARY KEY,
    storage_id  varchar(200) NOT NULL,
    name        varchar(255) NOT NULL,
    mime        varchar(120) NOT NULL,
    size        bigint       NOT NULL DEFAULT 0,
    created_by  bigint REFERENCES users(id) ON DELETE SET NULL,
    created_at  timestamptz  NOT NULL DEFAULT now()
);

-- The ad a message came from (click-to-chat ads), as Meta described it.
ALTER TABLE wa_messages ADD COLUMN referral jsonb;

-- Settings that belong to the whole module rather than to one number.
-- Secrets inside the value are stored encrypted.
CREATE TABLE wa_global_settings (
    key         varchar(64) PRIMARY KEY,
    value       jsonb       NOT NULL DEFAULT '{}',
    updated_by  bigint REFERENCES users(id) ON DELETE SET NULL,
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- A survey sent over WhatsApp after a phone call.
CREATE TABLE wa_call_surveys (
    id              bigserial PRIMARY KEY,
    call_id         varchar(80)  NOT NULL UNIQUE,
    user_id         bigint REFERENCES users(id) ON DELETE SET NULL,
    peer_key        varchar(20)  NOT NULL,
    wa_id           varchar(32)  NOT NULL,
    channel_id      bigint REFERENCES wa_channels(id) ON DELETE SET NULL,
    conversation_id bigint REFERENCES wa_conversations(id) ON DELETE SET NULL,
    message_id      bigint REFERENCES wa_messages(id) ON DELETE SET NULL,
    direction       varchar(10)  NOT NULL DEFAULT '',
    talk_seconds    int          NOT NULL DEFAULT 0,
    status          varchar(16)  NOT NULL DEFAULT 'queued', -- queued, sent, answered, failed, skipped
    note            varchar(255) NOT NULL DEFAULT '',
    score           smallint,
    comment         text         NOT NULL DEFAULT '',
    send_at         timestamptz  NOT NULL DEFAULT now(),
    sent_at         timestamptz,
    answered_at     timestamptz,
    created_at      timestamptz  NOT NULL DEFAULT now()
);
CREATE INDEX wa_call_surveys_due ON wa_call_surveys (send_at) WHERE status = 'queued';
CREATE INDEX wa_call_surveys_peer ON wa_call_surveys (peer_key, created_at);
CREATE INDEX wa_call_surveys_user ON wa_call_surveys (user_id, created_at);

-- There will be no bulk sends; the permission for them goes away.
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE key = 'whatsapp.broadcast');
DELETE FROM permissions WHERE key = 'whatsapp.broadcast';

-- +goose Down
DROP TABLE wa_call_surveys;
DROP TABLE wa_global_settings;
ALTER TABLE wa_messages DROP COLUMN referral;
DROP TABLE wa_files;
