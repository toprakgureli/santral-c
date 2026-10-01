-- +goose Up
-- The work after a customer message runs in its own workers now, one
-- conversation at a time and in order, so the job knows its conversation.
-- It also keeps what the ticket looked like when the message came (when it
-- had been resolved, who had it) and which steps are already done, so work
-- picked up again after a restart neither repeats a step nor undoes what a
-- person did in between.
ALTER TABLE wa_inbound_jobs ADD COLUMN conversation_id bigint;
ALTER TABLE wa_inbound_jobs ADD COLUMN resolved_at timestamptz;
ALTER TABLE wa_inbound_jobs ADD COLUMN owner_id bigint;
ALTER TABLE wa_inbound_jobs ADD COLUMN steps text NOT NULL DEFAULT '';
UPDATE wa_inbound_jobs j SET conversation_id = m.conversation_id FROM wa_messages m WHERE m.id = j.message_id;
CREATE INDEX wa_inbound_jobs_conversation_idx ON wa_inbound_jobs (conversation_id, message_id);

-- +goose Down
DROP INDEX IF EXISTS wa_inbound_jobs_conversation_idx;
ALTER TABLE wa_inbound_jobs DROP COLUMN steps;
ALTER TABLE wa_inbound_jobs DROP COLUMN owner_id;
ALTER TABLE wa_inbound_jobs DROP COLUMN resolved_at;
ALTER TABLE wa_inbound_jobs DROP COLUMN conversation_id;
