-- +goose Up
-- WhatsApp Business: devices (numbers), customers, conversations, tickets,
-- messages, and everything that is configured per device. Nothing about an
-- account is in the code: numbers, ids and tokens are entered from the
-- panel, secrets are stored encrypted.

-- One WhatsApp number is one device. Settings that belong to a device live
-- in its own row, so a new device starts from nothing.
CREATE TABLE wa_channels (
    id                bigserial PRIMARY KEY,
    name              varchar(120) NOT NULL,
    display_phone     varchar(40)  NOT NULL DEFAULT '',
    phone_number_id   varchar(64)  NOT NULL,
    waba_id           varchar(64)  NOT NULL,
    app_id            varchar(64)  NOT NULL DEFAULT '',
    graph_version     varchar(16)  NOT NULL DEFAULT '',
    access_token_enc  text         NOT NULL DEFAULT '',
    app_secret_enc    text         NOT NULL DEFAULT '',
    verify_token      varchar(64)  NOT NULL,
    hook_key          varchar(64)  NOT NULL UNIQUE,
    active            boolean      NOT NULL DEFAULT true,
    settings          jsonb        NOT NULL DEFAULT '{}',
    verified_name     varchar(200) NOT NULL DEFAULT '',
    quality_rating    varchar(20)  NOT NULL DEFAULT '',
    messaging_limit   varchar(40)  NOT NULL DEFAULT '',
    last_webhook_at   timestamptz,
    last_error        text         NOT NULL DEFAULT '',
    last_error_at     timestamptz,
    created_by        bigint REFERENCES users(id) ON DELETE SET NULL,
    created_at        timestamptz  NOT NULL DEFAULT now(),
    updated_at        timestamptz  NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX wa_channels_phone_number_idx ON wa_channels (phone_number_id);

-- Agent teams, for routing ("Satış", "Destek").
CREATE TABLE wa_teams (
    id         bigserial PRIMARY KEY,
    name       varchar(80) NOT NULL,
    color      varchar(20) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE wa_team_members (
    team_id bigint NOT NULL REFERENCES wa_teams(id) ON DELETE CASCADE,
    user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (team_id, user_id)
);

-- Who works on which device. last_assigned_at drives the round in the
-- automatic distribution.
CREATE TABLE wa_channel_members (
    channel_id       bigint NOT NULL REFERENCES wa_channels(id) ON DELETE CASCADE,
    user_id          bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    last_assigned_at timestamptz,
    PRIMARY KEY (channel_id, user_id)
);
CREATE INDEX wa_channel_members_user_idx ON wa_channel_members (user_id);

-- Every webhook call is written here before anything else happens, so a
-- message that failed to process is never lost.
CREATE TABLE wa_webhook_events (
    id           bigserial PRIMARY KEY,
    channel_id   bigint REFERENCES wa_channels(id) ON DELETE SET NULL,
    payload      jsonb       NOT NULL,
    status       varchar(10) NOT NULL DEFAULT 'pending', -- pending | done | failed
    attempts     integer     NOT NULL DEFAULT 0,
    last_error   text        NOT NULL DEFAULT '',
    received_at  timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz,
    next_try_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX wa_webhook_events_pending_idx ON wa_webhook_events (status, next_try_at) WHERE status <> 'done';

-- A customer, known by the WhatsApp number.
CREATE TABLE wa_contacts (
    id             bigserial PRIMARY KEY,
    wa_id          varchar(32)  NOT NULL UNIQUE,
    peer_key       varchar(20)  NOT NULL DEFAULT '',
    profile_name   varchar(200) NOT NULL DEFAULT '',
    name           varchar(200) NOT NULL DEFAULT '',
    tags           jsonb        NOT NULL DEFAULT '[]',
    note           text         NOT NULL DEFAULT '',
    opted_out      boolean      NOT NULL DEFAULT false,
    blocked        boolean      NOT NULL DEFAULT false,
    source         jsonb,                                -- the ad a customer came from, if any
    created_at     timestamptz  NOT NULL DEFAULT now(),
    updated_at     timestamptz  NOT NULL DEFAULT now()
);
CREATE INDEX wa_contacts_peer_idx ON wa_contacts (peer_key);

-- Every change a panel may need to catch up on bumps the conversation's
-- version from this sequence.
CREATE SEQUENCE wa_version_seq;

-- The long-lived thread between a device and a customer.
CREATE TABLE wa_conversations (
    id                bigserial PRIMARY KEY,
    channel_id        bigint NOT NULL REFERENCES wa_channels(id) ON DELETE CASCADE,
    contact_id        bigint NOT NULL REFERENCES wa_contacts(id) ON DELETE CASCADE,
    ticket_id         bigint,
    last_inbound_at   timestamptz,
    last_message_id   bigint,
    last_message_at   timestamptz,
    team_read_id      bigint NOT NULL DEFAULT 0,  -- read by anyone on our side up to here
    meta_read_id      bigint NOT NULL DEFAULT 0,  -- last inbound we marked read towards the customer
    unread            integer NOT NULL DEFAULT 0,
    version           bigint NOT NULL DEFAULT nextval('wa_version_seq'),
    created_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (channel_id, contact_id)
);
CREATE INDEX wa_conversations_version_idx ON wa_conversations (version);
CREATE INDEX wa_conversations_last_idx ON wa_conversations (channel_id, last_message_at DESC);

-- A support case inside a conversation.
CREATE SEQUENCE wa_ticket_number_seq START 1001;
CREATE TABLE wa_tickets (
    id                 bigserial PRIMARY KEY,
    number             bigint NOT NULL DEFAULT nextval('wa_ticket_number_seq') UNIQUE,
    conversation_id    bigint NOT NULL REFERENCES wa_conversations(id) ON DELETE CASCADE,
    channel_id         bigint NOT NULL REFERENCES wa_channels(id) ON DELETE CASCADE,
    contact_id         bigint NOT NULL REFERENCES wa_contacts(id) ON DELETE CASCADE,
    status             varchar(10) NOT NULL DEFAULT 'open', -- bot | open | pending | resolved
    owner_id           bigint REFERENCES users(id) ON DELETE SET NULL,
    team_id            bigint REFERENCES wa_teams(id) ON DELETE SET NULL,
    priority           varchar(10) NOT NULL DEFAULT 'normal', -- low | normal | high | urgent
    category           varchar(80) NOT NULL DEFAULT '',
    tags               jsonb NOT NULL DEFAULT '[]',
    awaiting_since     timestamptz,           -- the customer's first unanswered message
    waiting_listed_at  timestamptz,           -- when it entered "Cevap Bekleyenler"
    waiting_count      integer NOT NULL DEFAULT 0,
    longest_wait_sec   integer NOT NULL DEFAULT 0,
    reopen_count       integer NOT NULL DEFAULT 0,
    first_response_at  timestamptz,
    resolved_at        timestamptz,
    resolved_by        bigint REFERENCES users(id) ON DELETE SET NULL,
    survey_sent_at     timestamptz,
    rating             integer,
    rating_comment     text NOT NULL DEFAULT '',
    rated_at           timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX wa_tickets_conv_idx ON wa_tickets (conversation_id);
CREATE INDEX wa_tickets_owner_idx ON wa_tickets (owner_id, status);
CREATE INDEX wa_tickets_awaiting_idx ON wa_tickets (awaiting_since) WHERE awaiting_since IS NOT NULL;
CREATE INDEX wa_tickets_channel_status_idx ON wa_tickets (channel_id, status);
ALTER TABLE wa_conversations ADD CONSTRAINT wa_conversations_ticket_fk FOREIGN KEY (ticket_id) REFERENCES wa_tickets(id) ON DELETE SET NULL;

-- Everyone who worked on a ticket: its owner and those who came to help.
CREATE TABLE wa_ticket_participants (
    ticket_id      bigint NOT NULL REFERENCES wa_tickets(id) ON DELETE CASCADE,
    user_id        bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role           varchar(10) NOT NULL DEFAULT 'helper', -- owner | helper
    greeted        boolean NOT NULL DEFAULT false,
    first_reply_at timestamptz,
    joined_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (ticket_id, user_id)
);
CREATE INDEX wa_ticket_participants_user_idx ON wa_ticket_participants (user_id);

-- Every assignment change, for the history and the reports.
CREATE TABLE wa_assignments (
    id         bigserial PRIMARY KEY,
    ticket_id  bigint NOT NULL REFERENCES wa_tickets(id) ON DELETE CASCADE,
    kind       varchar(12) NOT NULL, -- auto | manual | claim | join | take | transfer | pool
    from_user  bigint REFERENCES users(id) ON DELETE SET NULL,
    to_user    bigint REFERENCES users(id) ON DELETE SET NULL,
    team_id    bigint REFERENCES wa_teams(id) ON DELETE SET NULL,
    by_user    bigint REFERENCES users(id) ON DELETE SET NULL,
    note       text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX wa_assignments_ticket_idx ON wa_assignments (ticket_id);

-- Messages, both ways, plus our internal notes and events. Outgoing rows
-- double as the send queue: status queued until Meta accepts them.
CREATE TABLE wa_messages (
    id               bigserial PRIMARY KEY,
    channel_id       bigint NOT NULL REFERENCES wa_channels(id) ON DELETE CASCADE,
    conversation_id  bigint NOT NULL REFERENCES wa_conversations(id) ON DELETE CASCADE,
    ticket_id        bigint REFERENCES wa_tickets(id) ON DELETE SET NULL,
    direction        varchar(6)  NOT NULL, -- in | out | note | event
    kind             varchar(16) NOT NULL, -- text | image | video | audio | document | sticker | location | contacts | interactive | button | reaction | template | unsupported | event
    wamid            varchar(200) UNIQUE,
    client_id        varchar(64) UNIQUE,   -- the sender's idempotency key
    sender_kind      varchar(12) NOT NULL DEFAULT 'customer', -- customer | agent | bot | automation | system
    sender_user_id   bigint REFERENCES users(id) ON DELETE SET NULL,
    sender_label     varchar(200) NOT NULL DEFAULT '',
    body             text NOT NULL DEFAULT '',
    media            jsonb,
    payload          jsonb,
    reply_to_wamid   varchar(200),
    status           varchar(10) NOT NULL DEFAULT 'received', -- received | queued | sent | delivered | read | failed
    error_code       integer,
    error_text       text NOT NULL DEFAULT '',
    attempts         integer NOT NULL DEFAULT 0,
    next_try_at      timestamptz,
    sent_at          timestamptz,
    delivered_at     timestamptz,
    read_at          timestamptz,
    failed_at        timestamptz,
    wa_timestamp     timestamptz,
    pricing          jsonb,
    created_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX wa_messages_conv_idx ON wa_messages (conversation_id, id);
CREATE INDEX wa_messages_ticket_idx ON wa_messages (ticket_id);
CREATE INDEX wa_messages_queue_idx ON wa_messages (next_try_at) WHERE status = 'queued';

-- A status that arrived before the message it belongs to waits here.
CREATE TABLE wa_pending_statuses (
    wamid      varchar(200) NOT NULL,
    status     varchar(10)  NOT NULL,
    payload    jsonb        NOT NULL,
    created_at timestamptz  NOT NULL DEFAULT now(),
    PRIMARY KEY (wamid, status)
);

-- How far each person has read each conversation.
CREATE TABLE wa_reads (
    conversation_id bigint NOT NULL REFERENCES wa_conversations(id) ON DELETE CASCADE,
    user_id         bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message_id      bigint NOT NULL,
    read_at         timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (conversation_id, user_id)
);

-- Message templates approved by Meta, per business account.
CREATE TABLE wa_templates (
    id              bigserial PRIMARY KEY,
    waba_id         varchar(64)  NOT NULL,
    meta_id         varchar(64)  NOT NULL DEFAULT '',
    name            varchar(512) NOT NULL,
    language        varchar(15)  NOT NULL,
    category        varchar(20)  NOT NULL,
    status          varchar(20)  NOT NULL DEFAULT 'PENDING',
    components      jsonb        NOT NULL DEFAULT '[]',
    rejected_reason text         NOT NULL DEFAULT '',
    quality         varchar(20)  NOT NULL DEFAULT '',
    created_by      bigint REFERENCES users(id) ON DELETE SET NULL,
    updated_at      timestamptz  NOT NULL DEFAULT now(),
    UNIQUE (waba_id, name, language)
);

-- Ready answers an agent pulls in with "/".
CREATE TABLE wa_quick_replies (
    id          bigserial PRIMARY KEY,
    shortcut    varchar(40)  NOT NULL,
    title       varchar(120) NOT NULL,
    body        text         NOT NULL,
    channel_ids jsonb        NOT NULL DEFAULT '[]',
    created_by  bigint REFERENCES users(id) ON DELETE SET NULL,
    updated_at  timestamptz  NOT NULL DEFAULT now()
);

-- One-step rules: when this happens and this holds, do that.
CREATE TABLE wa_automations (
    id            bigserial PRIMARY KEY,
    name          varchar(120) NOT NULL,
    active        boolean NOT NULL DEFAULT true,
    channel_ids   jsonb   NOT NULL DEFAULT '[]',
    trigger       varchar(30) NOT NULL,
    conditions    jsonb   NOT NULL DEFAULT '[]',
    actions       jsonb   NOT NULL DEFAULT '[]',
    cooldown_min  integer NOT NULL DEFAULT 0,
    position      integer NOT NULL DEFAULT 0,
    created_by    bigint REFERENCES users(id) ON DELETE SET NULL,
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE wa_automation_runs (
    id            bigserial PRIMARY KEY,
    automation_id bigint NOT NULL REFERENCES wa_automations(id) ON DELETE CASCADE,
    contact_id    bigint REFERENCES wa_contacts(id) ON DELETE CASCADE,
    ticket_id     bigint REFERENCES wa_tickets(id) ON DELETE SET NULL,
    ok            boolean NOT NULL DEFAULT true,
    detail        text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX wa_automation_runs_idx ON wa_automation_runs (automation_id, contact_id, created_at DESC);

-- Chatbots: a draw-your-own conversation flow, a draft and published
-- versions, and which devices use it.
CREATE TABLE wa_bots (
    id                bigserial PRIMARY KEY,
    name              varchar(120) NOT NULL,
    description       text NOT NULL DEFAULT '',
    active            boolean NOT NULL DEFAULT false,
    channel_ids       jsonb NOT NULL DEFAULT '[]',
    trigger           varchar(20) NOT NULL DEFAULT 'entry', -- entry | after_hours | keyword
    keywords          jsonb NOT NULL DEFAULT '[]',
    draft             jsonb NOT NULL DEFAULT '{"nodes":[],"edges":[]}',
    published_version integer NOT NULL DEFAULT 0,
    published_at      timestamptz,
    created_by        bigint REFERENCES users(id) ON DELETE SET NULL,
    updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE wa_bot_versions (
    bot_id       bigint NOT NULL REFERENCES wa_bots(id) ON DELETE CASCADE,
    version      integer NOT NULL,
    graph        jsonb NOT NULL,
    published_by bigint REFERENCES users(id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (bot_id, version)
);

-- Where a customer is inside a flow and what it has collected.
CREATE TABLE wa_bot_sessions (
    conversation_id bigint PRIMARY KEY REFERENCES wa_conversations(id) ON DELETE CASCADE,
    bot_id          bigint NOT NULL REFERENCES wa_bots(id) ON DELETE CASCADE,
    version         integer NOT NULL,
    node_id         varchar(64) NOT NULL,
    vars            jsonb NOT NULL DEFAULT '{}',
    tries           integer NOT NULL DEFAULT 0,
    started_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- Step by step what the flows did, for the reports.
CREATE TABLE wa_bot_events (
    id              bigserial PRIMARY KEY,
    bot_id          bigint NOT NULL REFERENCES wa_bots(id) ON DELETE CASCADE,
    version         integer NOT NULL,
    conversation_id bigint REFERENCES wa_conversations(id) ON DELETE CASCADE,
    node_id         varchar(64) NOT NULL,
    kind            varchar(16) NOT NULL, -- enter | answer | fail | handoff | end | timeout
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX wa_bot_events_idx ON wa_bot_events (bot_id, created_at);

-- Outside systems a flow may ask (order status, subscription lookup).
CREATE TABLE wa_integrations (
    id          bigserial PRIMARY KEY,
    name        varchar(120) NOT NULL,
    method      varchar(8)   NOT NULL DEFAULT 'GET',
    url         text         NOT NULL,
    headers_enc text         NOT NULL DEFAULT '',
    body        text         NOT NULL DEFAULT '',
    timeout_sec integer      NOT NULL DEFAULT 8,
    updated_at  timestamptz  NOT NULL DEFAULT now()
);

-- "Sizi arayalım" requests, picked up by the call centre.
CREATE TABLE wa_callbacks (
    id          bigserial PRIMARY KEY,
    channel_id  bigint REFERENCES wa_channels(id) ON DELETE SET NULL,
    contact_id  bigint REFERENCES wa_contacts(id) ON DELETE CASCADE,
    ticket_id   bigint REFERENCES wa_tickets(id) ON DELETE SET NULL,
    phone       varchar(40) NOT NULL,
    note        text NOT NULL DEFAULT '',
    status      varchar(10) NOT NULL DEFAULT 'open', -- open | done
    done_by     bigint REFERENCES users(id) ON DELETE SET NULL,
    done_at     timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE wa_callbacks;
DROP TABLE wa_integrations;
DROP TABLE wa_bot_events;
DROP TABLE wa_bot_sessions;
DROP TABLE wa_bot_versions;
DROP TABLE wa_bots;
DROP TABLE wa_automation_runs;
DROP TABLE wa_automations;
DROP TABLE wa_quick_replies;
DROP TABLE wa_templates;
DROP TABLE wa_reads;
DROP TABLE wa_pending_statuses;
DROP TABLE wa_messages;
DROP TABLE wa_assignments;
DROP TABLE wa_ticket_participants;
ALTER TABLE wa_conversations DROP CONSTRAINT wa_conversations_ticket_fk;
DROP TABLE wa_tickets;
DROP SEQUENCE wa_ticket_number_seq;
DROP TABLE wa_conversations;
DROP SEQUENCE wa_version_seq;
DROP TABLE wa_contacts;
DROP TABLE wa_webhook_events;
DROP TABLE wa_channel_members;
DROP TABLE wa_team_members;
DROP TABLE wa_teams;
DROP TABLE wa_channels;
