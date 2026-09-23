-- +goose Up
-- Mini games inside Teams. Content (words, questions, scenarios) is data,
-- not code: administrators edit it, nothing is baked into the binary.
CREATE TABLE game_items (
    id         bigserial PRIMARY KEY,
    kind       varchar(20) NOT NULL,           -- draw | poll | solve | story | bingo
    text       text        NOT NULL,
    answer     text        NOT NULL DEFAULT '',
    options    jsonb,
    seconds    integer     NOT NULL DEFAULT 0, -- 0: the game's default
    active     boolean     NOT NULL DEFAULT true,
    created_by bigint REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX game_items_kind_idx ON game_items (kind, active);

CREATE TABLE games (
    id          bigserial PRIMARY KEY,
    group_id    bigint NOT NULL REFERENCES chat_groups(id) ON DELETE CASCADE,
    message_id  bigint REFERENCES chat_messages(id) ON DELETE SET NULL,
    kind        varchar(20) NOT NULL,
    status      varchar(10) NOT NULL DEFAULT 'lobby', -- lobby | playing | finished | cancelled
    host_id     bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    config      jsonb NOT NULL DEFAULT '{}',
    state       jsonb NOT NULL DEFAULT '{}',
    winners     jsonb NOT NULL DEFAULT '[]',
    version     integer NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now(),
    started_at  timestamptz,
    finished_at timestamptz
);
CREATE INDEX games_group_idx ON games (group_id, status);

CREATE TABLE game_players (
    game_id   bigint NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    user_id   bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    team      integer NOT NULL DEFAULT 0,
    score     integer NOT NULL DEFAULT 0,
    joined_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (game_id, user_id)
);

CREATE TABLE game_results (
    id          bigserial PRIMARY KEY,
    game_id     bigint NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    user_id     bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind        varchar(20) NOT NULL,
    won         boolean NOT NULL DEFAULT false,
    score       integer NOT NULL DEFAULT 0,
    finished_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX game_results_user_idx ON game_results (user_id, finished_at DESC);
CREATE INDEX game_results_time_idx ON game_results (finished_at DESC);

INSERT INTO system_settings (key, value) VALUES
    ('games_enabled', '1'),
    ('games_pause_on_call', '1'),
    ('games_break_only', '0')
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DELETE FROM system_settings WHERE key IN ('games_enabled', 'games_pause_on_call', 'games_break_only');
DROP TABLE game_results;
DROP TABLE game_players;
DROP TABLE games;
DROP TABLE game_items;
