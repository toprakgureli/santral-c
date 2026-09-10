-- +goose Up
-- Customer escalation: an admin-managed catalog of situations (category ->
-- reason) that agents pick during a call, plus the per-number escalation
-- records so the next agent who dials a customer sees the history.

-- Top-level grouping, e.g. "Memnuniyet".
CREATE TABLE escalation_categories (
    id         bigserial PRIMARY KEY,
    name       varchar(160) NOT NULL,
    created_by bigint REFERENCES users (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE UNIQUE INDEX idx_escalation_categories_name ON escalation_categories (lower(name)) WHERE deleted_at IS NULL;
CREATE INDEX idx_escalation_categories_deleted_at ON escalation_categories (deleted_at);

-- A concrete situation under a category, e.g. "Yazılım ekibiyle ilgili
-- memnuniyetsizlik iletti".
CREATE TABLE escalation_reasons (
    id          bigserial PRIMARY KEY,
    category_id bigint NOT NULL REFERENCES escalation_categories (id) ON DELETE CASCADE,
    name        varchar(240) NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);
CREATE INDEX idx_escalation_reasons_category_id ON escalation_reasons (category_id);
CREATE UNIQUE INDEX idx_escalation_reasons_unique ON escalation_reasons (category_id, lower(name)) WHERE deleted_at IS NULL;

-- One escalation logged by an agent against the customer's number. number_key
-- is the last ten digits, so the same customer is found regardless of how the
-- number was dialed (0530..., 90530..., +90530..., 530...).
CREATE TABLE call_escalations (
    id            bigserial PRIMARY KEY,
    number_key    varchar(20) NOT NULL,
    number        varchar(32) NOT NULL,
    category_id   bigint REFERENCES escalation_categories (id) ON DELETE SET NULL,
    reason_id     bigint REFERENCES escalation_reasons (id) ON DELETE SET NULL,
    category_name varchar(160) NOT NULL,
    reason_name   varchar(240) NOT NULL,
    note          text,
    agent_id      bigint REFERENCES users (id) ON DELETE SET NULL,
    agent_name    varchar(160) NOT NULL,
    call_uuid     varchar(80),
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_call_escalations_number_key ON call_escalations (number_key, created_at DESC);
CREATE INDEX idx_call_escalations_agent_id ON call_escalations (agent_id);

-- +goose Down
DROP TABLE call_escalations;
DROP TABLE escalation_reasons;
DROP TABLE escalation_categories;
