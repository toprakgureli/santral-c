-- +goose Up
-- Each person's own WhatsApp preferences: sound and desktop notices, a
-- mute for everything, and per-conversation mutes and pins. They change
-- nothing for anybody else.
CREATE TABLE wa_user_prefs (
    user_id     bigint PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    sound       boolean     NOT NULL DEFAULT true,
    desktop     boolean     NOT NULL DEFAULT true,
    muted_until timestamptz,
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE wa_user_conversations (
    user_id         bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    conversation_id bigint NOT NULL REFERENCES wa_conversations(id) ON DELETE CASCADE,
    muted_until     timestamptz,
    pinned_at       timestamptz,
    PRIMARY KEY (user_id, conversation_id)
);

-- +goose Down
DROP TABLE wa_user_conversations;
DROP TABLE wa_user_prefs;
