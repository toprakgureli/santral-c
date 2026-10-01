-- +goose Up
-- How much of a room's earlier conversation a member may read. history_from
-- is the first line id the seat sees; 0 means the whole history. Seats that
-- exist today keep the whole history, as before.
ALTER TABLE chat_members ADD COLUMN history_from bigint NOT NULL DEFAULT 0;

-- What the person who sent an invite chose for the newcomer: nothing
-- earlier, the last 50 or 100 lines, or everything. It is applied when the
-- invite is accepted. Invites already waiting were sent when everyone saw
-- the whole history, so they keep that; new ones default to nothing.
ALTER TABLE chat_invites ADD COLUMN history varchar(4) NOT NULL DEFAULT 'all'
    CHECK (history IN ('none', '50', '100', 'all'));
ALTER TABLE chat_invites ALTER COLUMN history SET DEFAULT 'none';

-- +goose Down
ALTER TABLE chat_invites DROP COLUMN history;
ALTER TABLE chat_members DROP COLUMN history_from;
