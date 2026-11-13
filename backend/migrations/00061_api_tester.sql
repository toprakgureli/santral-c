-- +goose Up
INSERT INTO permissions (key, module, description)
VALUES
    ('api_tester.view', 'api_tester', 'API test koleksiyonlarını ve menü butonunu görür'),
    ('api_tester.execute', 'api_tester', 'Koleksiyonlardaki API isteklerini parametre doldurup çalıştırır'),
    ('api_tester.manage', 'api_tester', 'API test menüsünü açıp kapatır, koleksiyon ve istek şablonu oluşturup yönetir')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.key IN ('api_tester.view', 'api_tester.execute', 'api_tester.manage')
WHERE r.name IN ('invisible_admin', 'manager')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.key IN ('api_tester.view', 'api_tester.execute')
WHERE r.name = 'technical_team'
ON CONFLICT DO NOTHING;

INSERT INTO system_settings (key, value)
VALUES ('api_tester_enabled', '1')
ON CONFLICT (key) DO NOTHING;

CREATE TABLE IF NOT EXISTS api_collections (
    id          bigserial PRIMARY KEY,
    name        varchar(120) NOT NULL,
    description text NOT NULL DEFAULT '',
    project_tag varchar(64) NOT NULL DEFAULT '',
    base_url    varchar(500) NOT NULL DEFAULT '',
    sort_order  integer NOT NULL DEFAULT 0,
    active      boolean NOT NULL DEFAULT true,
    created_by  bigint REFERENCES users (id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS api_requests (
    id                    bigserial PRIMARY KEY,
    collection_id         bigint NOT NULL REFERENCES api_collections (id) ON DELETE CASCADE,
    name                  varchar(120) NOT NULL,
    description           text NOT NULL DEFAULT '',
    method                varchar(12) NOT NULL DEFAULT 'GET',
    url_template          text NOT NULL,
    headers_enc           text NOT NULL DEFAULT '',
    query_params_json     text NOT NULL DEFAULT '[]',
    body_template         text NOT NULL DEFAULT '',
    content_type          varchar(120) NOT NULL DEFAULT 'application/json',
    variables_json        text NOT NULL DEFAULT '[]',
    execution_mode        varchar(20) NOT NULL DEFAULT 'client',
    allow_server_fallback boolean NOT NULL DEFAULT true,
    runbook               text NOT NULL DEFAULT '',
    sort_order            integer NOT NULL DEFAULT 0,
    active                boolean NOT NULL DEFAULT true,
    created_by            bigint REFERENCES users (id) ON DELETE SET NULL,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_api_collections_order ON api_collections (sort_order, id);
CREATE INDEX IF NOT EXISTS idx_api_requests_collection ON api_requests (collection_id, sort_order, id);

CREATE TABLE IF NOT EXISTS api_run_logs (
    id              bigserial PRIMARY KEY,
    request_id      bigint REFERENCES api_requests (id) ON DELETE SET NULL,
    request_name    varchar(120) NOT NULL DEFAULT '',
    collection_name varchar(120) NOT NULL DEFAULT '',
    user_id         bigint REFERENCES users (id) ON DELETE SET NULL,
    user_name       varchar(120) NOT NULL DEFAULT '',
    method          varchar(12) NOT NULL,
    target_url      text NOT NULL DEFAULT '',
    execution_mode  varchar(24) NOT NULL DEFAULT 'client',
    status_code     integer NOT NULL DEFAULT 0,
    duration_ms     integer NOT NULL DEFAULT 0,
    error_message   text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_api_run_logs_time ON api_run_logs (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_api_run_logs_request ON api_run_logs (request_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS api_run_logs;
DROP TABLE IF EXISTS api_requests;
DROP TABLE IF EXISTS api_collections;
DELETE FROM system_settings WHERE key = 'api_tester_enabled';
DELETE FROM permissions WHERE key IN ('api_tester.view', 'api_tester.execute', 'api_tester.manage');
