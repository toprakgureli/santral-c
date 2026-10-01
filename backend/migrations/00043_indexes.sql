-- +goose NO TRANSACTION
-- +goose Up
-- Indexes for queries that slow down as the data grows, built without
-- locking the tables, so the panel keeps working while they are made.

-- Reports by period and the sweep that fetches missed customer files.
CREATE INDEX CONCURRENTLY IF NOT EXISTS wa_messages_created_idx ON wa_messages (created_at);
-- The inbox reads open tickets and those resolved in the last days.
CREATE INDEX CONCURRENTLY IF NOT EXISTS wa_tickets_resolved_idx ON wa_tickets (resolved_at) WHERE status = 'resolved';
-- Columns that point at messages, tickets and conversations, so following
-- or clearing those links does not read whole tables.
CREATE INDEX CONCURRENTLY IF NOT EXISTS wa_conversations_ticket_idx ON wa_conversations (ticket_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS wa_call_surveys_message_idx ON wa_call_surveys (message_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS wa_call_surveys_conversation_idx ON wa_call_surveys (conversation_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS wa_automation_runs_ticket_idx ON wa_automation_runs (ticket_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS wa_callbacks_ticket_idx ON wa_callbacks (ticket_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS wa_callbacks_open_idx ON wa_callbacks (created_at) WHERE status = 'open';
CREATE INDEX CONCURRENTLY IF NOT EXISTS wa_bot_events_conversation_idx ON wa_bot_events (conversation_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS wa_user_conversations_conversation_idx ON wa_user_conversations (conversation_id);
-- The audit page filters by action and lists newest first.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_audit_log_action_created ON audit_log (action, created_at DESC);

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS idx_audit_log_action_created;
DROP INDEX CONCURRENTLY IF EXISTS wa_user_conversations_conversation_idx;
DROP INDEX CONCURRENTLY IF EXISTS wa_bot_events_conversation_idx;
DROP INDEX CONCURRENTLY IF EXISTS wa_callbacks_open_idx;
DROP INDEX CONCURRENTLY IF EXISTS wa_callbacks_ticket_idx;
DROP INDEX CONCURRENTLY IF EXISTS wa_automation_runs_ticket_idx;
DROP INDEX CONCURRENTLY IF EXISTS wa_call_surveys_conversation_idx;
DROP INDEX CONCURRENTLY IF EXISTS wa_call_surveys_message_idx;
DROP INDEX CONCURRENTLY IF EXISTS wa_conversations_ticket_idx;
DROP INDEX CONCURRENTLY IF EXISTS wa_tickets_resolved_idx;
DROP INDEX CONCURRENTLY IF EXISTS wa_messages_created_idx;
