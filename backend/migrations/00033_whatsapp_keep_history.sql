-- +goose Up
-- Removing a device or a chatbot must never take the conversation history or
-- the chatbot reports with it by accident. The database now refuses such a
-- delete while history exists; the panel turns a device or chatbot off
-- instead and wipes history only on an explicit, confirmed request.
ALTER TABLE wa_conversations DROP CONSTRAINT wa_conversations_channel_id_fkey,
    ADD CONSTRAINT wa_conversations_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES wa_channels(id) ON DELETE RESTRICT;
ALTER TABLE wa_tickets DROP CONSTRAINT wa_tickets_channel_id_fkey,
    ADD CONSTRAINT wa_tickets_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES wa_channels(id) ON DELETE RESTRICT;
ALTER TABLE wa_messages DROP CONSTRAINT wa_messages_channel_id_fkey,
    ADD CONSTRAINT wa_messages_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES wa_channels(id) ON DELETE RESTRICT;
ALTER TABLE wa_bot_versions DROP CONSTRAINT wa_bot_versions_bot_id_fkey,
    ADD CONSTRAINT wa_bot_versions_bot_id_fkey FOREIGN KEY (bot_id) REFERENCES wa_bots(id) ON DELETE RESTRICT;
ALTER TABLE wa_bot_events DROP CONSTRAINT wa_bot_events_bot_id_fkey,
    ADD CONSTRAINT wa_bot_events_bot_id_fkey FOREIGN KEY (bot_id) REFERENCES wa_bots(id) ON DELETE RESTRICT;

-- +goose Down
ALTER TABLE wa_bot_events DROP CONSTRAINT wa_bot_events_bot_id_fkey,
    ADD CONSTRAINT wa_bot_events_bot_id_fkey FOREIGN KEY (bot_id) REFERENCES wa_bots(id) ON DELETE CASCADE;
ALTER TABLE wa_bot_versions DROP CONSTRAINT wa_bot_versions_bot_id_fkey,
    ADD CONSTRAINT wa_bot_versions_bot_id_fkey FOREIGN KEY (bot_id) REFERENCES wa_bots(id) ON DELETE CASCADE;
ALTER TABLE wa_messages DROP CONSTRAINT wa_messages_channel_id_fkey,
    ADD CONSTRAINT wa_messages_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES wa_channels(id) ON DELETE CASCADE;
ALTER TABLE wa_tickets DROP CONSTRAINT wa_tickets_channel_id_fkey,
    ADD CONSTRAINT wa_tickets_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES wa_channels(id) ON DELETE CASCADE;
ALTER TABLE wa_conversations DROP CONSTRAINT wa_conversations_channel_id_fkey,
    ADD CONSTRAINT wa_conversations_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES wa_channels(id) ON DELETE CASCADE;
