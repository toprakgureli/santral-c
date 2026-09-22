-- +goose Up
-- Teams: in-house chat. Groups and direct messages share one table; a
-- direct message is a group of kind 'dm' with exactly two members and a
-- dm_key that makes the pair unique. Messages are text for now; the
-- attachments column is reserved so images, video and GIFs can follow
-- without a schema change. Deleting a message is a soft delete: the row
-- stays and the chat shows "Bu mesaj silindi."

CREATE TABLE chat_groups (
    id          bigserial PRIMARY KEY,
    kind        varchar(12) NOT NULL DEFAULT 'group' CHECK (kind IN ('group', 'dm')),
    name        varchar(120) NOT NULL DEFAULT '',
    description text NOT NULL DEFAULT '',
    avatar      text NOT NULL DEFAULT '',
    -- 'everyone': every member with can_post may write; 'admins': only
    -- owners and admins (an announcement group).
    post_policy varchar(12) NOT NULL DEFAULT 'everyone' CHECK (post_policy IN ('everyone', 'admins')),
    dm_key      varchar(41),
    created_by  bigint REFERENCES users (id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);
CREATE UNIQUE INDEX idx_chat_groups_dm_key ON chat_groups (dm_key) WHERE dm_key IS NOT NULL;
CREATE INDEX idx_chat_groups_deleted_at ON chat_groups (deleted_at);

CREATE TABLE chat_members (
    group_id     bigint NOT NULL REFERENCES chat_groups (id) ON DELETE CASCADE,
    user_id      bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role         varchar(8) NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'admin', 'member')),
    can_post     boolean NOT NULL DEFAULT true,
    muted        boolean NOT NULL DEFAULT false,
    last_read_id bigint NOT NULL DEFAULT 0,
    invited_by   bigint REFERENCES users (id) ON DELETE SET NULL,
    joined_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (group_id, user_id)
);
CREATE INDEX idx_chat_members_user ON chat_members (user_id);

CREATE TABLE chat_invites (
    id         bigserial PRIMARY KEY,
    group_id   bigint NOT NULL REFERENCES chat_groups (id) ON DELETE CASCADE,
    user_id    bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    invited_by bigint REFERENCES users (id) ON DELETE SET NULL,
    status     varchar(10) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'declined')),
    created_at timestamptz NOT NULL DEFAULT now(),
    decided_at timestamptz
);
CREATE UNIQUE INDEX idx_chat_invites_pending ON chat_invites (group_id, user_id) WHERE status = 'pending';
CREATE INDEX idx_chat_invites_user ON chat_invites (user_id, status);

CREATE TABLE chat_messages (
    id          bigserial PRIMARY KEY,
    group_id    bigint NOT NULL REFERENCES chat_groups (id) ON DELETE CASCADE,
    sender_id   bigint REFERENCES users (id) ON DELETE SET NULL,
    kind        varchar(8) NOT NULL DEFAULT 'text' CHECK (kind IN ('text', 'system')),
    body        text NOT NULL DEFAULT '',
    reply_to_id bigint REFERENCES chat_messages (id) ON DELETE SET NULL,
    attachments jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    edited_at   timestamptz,
    deleted_at  timestamptz,
    deleted_by  bigint REFERENCES users (id) ON DELETE SET NULL
);
CREATE INDEX idx_chat_messages_group_id ON chat_messages (group_id, id DESC);

CREATE TABLE chat_reactions (
    message_id bigint NOT NULL REFERENCES chat_messages (id) ON DELETE CASCADE,
    user_id    bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    emoji      varchar(16) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (message_id, user_id, emoji)
);

-- +goose Down
DROP TABLE chat_reactions;
DROP TABLE chat_messages;
DROP TABLE chat_invites;
DROP TABLE chat_members;
DROP TABLE chat_groups;
