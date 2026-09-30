-- +goose Up
-- What follows a stored customer message (chatbot, automatic messages,
-- handing the chat to a person) runs after the message is saved. A row
-- here is written with the message and removed once that work is done, so
-- work cut off by a restart is picked up again instead of being lost.
CREATE TABLE wa_inbound_jobs (
    message_id bigint PRIMARY KEY REFERENCES wa_messages(id) ON DELETE CASCADE,
    created    boolean     NOT NULL DEFAULT false, -- the message opened a ticket
    reopened   boolean     NOT NULL DEFAULT false, -- it reopened a resolved one
    first      boolean     NOT NULL DEFAULT false, -- first message on this number
    opted_out  boolean     NOT NULL DEFAULT false, -- it asked to leave marketing
    attempts   int         NOT NULL DEFAULT 0,
    claimed_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX wa_inbound_jobs_claimed_idx ON wa_inbound_jobs (claimed_at);

-- +goose Down
DROP TABLE wa_inbound_jobs;
