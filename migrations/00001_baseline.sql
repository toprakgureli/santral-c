-- +goose Up
-- Auth / RBAC / MFA foundation. Telephony tables (calls, cdr, contacts,
-- queues) and Asterisk PJSIP realtime tables arrive in later migrations.

CREATE TABLE users (
    id                   bigserial PRIMARY KEY,
    name                 varchar(120) NOT NULL,
    email                varchar(255) NOT NULL,
    password             varchar(255) NOT NULL,           -- argon2id PHC string
    active               boolean NOT NULL DEFAULT true,
    must_change_password boolean NOT NULL DEFAULT false,
    mfa_secret           varchar(255),                    -- AES-GCM encrypted TOTP secret
    mfa_enabled          boolean NOT NULL DEFAULT false,
    mfa_exempt           boolean NOT NULL DEFAULT false,
    sip_extension        varchar(32),                     -- softphone identity
    sip_provisioned      boolean NOT NULL DEFAULT false,  -- PJSIP realtime row written
    last_login_at        timestamptz,
    onboarded_at         timestamptz,
    locked_until         timestamptz,
    failed_count         bigint NOT NULL DEFAULT 0,
    created_by           bigint,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    deleted_at           timestamptz
);
CREATE UNIQUE INDEX idx_users_email ON users (lower(email));
CREATE UNIQUE INDEX idx_users_sip_extension ON users (sip_extension) WHERE sip_extension IS NOT NULL;
CREATE INDEX idx_users_deleted_at ON users (deleted_at);

CREATE TABLE permissions (
    id          bigserial PRIMARY KEY,
    key         varchar(100) NOT NULL,
    module      varchar(60) NOT NULL,
    description varchar(255),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_permissions_key ON permissions (key);
CREATE INDEX idx_permissions_module ON permissions (module);

CREATE TABLE roles (
    id           bigserial PRIMARY KEY,
    name         varchar(60) NOT NULL,
    display_name varchar(120) NOT NULL,
    description  varchar(255),
    system       boolean NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    deleted_at   timestamptz
);
CREATE UNIQUE INDEX idx_roles_name ON roles (name);
CREATE INDEX idx_roles_deleted_at ON roles (deleted_at);

CREATE TABLE role_permissions (
    role_id       bigint NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    permission_id bigint NOT NULL REFERENCES permissions (id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE user_roles (
    user_id bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role_id bigint NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);

CREATE TABLE sessions (
    id           bigserial PRIMARY KEY,
    user_id      bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash   varchar(64) NOT NULL,                    -- SHA-256 hex of refresh token
    ip           varchar(45),
    user_agent   varchar(255),
    expires_at   timestamptz NOT NULL,
    revoked_at   timestamptz,
    last_used_at timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_sessions_token_hash ON sessions (token_hash);
CREATE INDEX idx_sessions_user_id ON sessions (user_id);
CREATE INDEX idx_sessions_expires_at ON sessions (expires_at);

CREATE TABLE login_attempts (
    id         bigserial PRIMARY KEY,
    email      varchar(255) NOT NULL,
    user_id    bigint,
    ip         varchar(45) NOT NULL,
    user_agent varchar(255),
    success    boolean NOT NULL,
    reason     varchar(60),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_login_attempts_email ON login_attempts (email);
CREATE INDEX idx_login_attempts_ip ON login_attempts (ip);
CREATE INDEX idx_login_attempts_created_at ON login_attempts (created_at);

CREATE TABLE ip_bans (
    id         bigserial PRIMARY KEY,
    ip         varchar(45) NOT NULL,
    reason     varchar(160),
    attempts   bigint NOT NULL DEFAULT 0,
    until      timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_ip_bans_ip ON ip_bans (ip);
CREATE INDEX idx_ip_bans_until ON ip_bans (until);

-- Every privileged action is recorded here (invisible_admin included).
CREATE TABLE audit_log (
    id          bigserial PRIMARY KEY,
    actor_id    bigint REFERENCES users (id) ON DELETE SET NULL,
    action      varchar(80) NOT NULL,
    target_type varchar(60),
    target_id   varchar(64),
    ip          varchar(45),
    detail      jsonb NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_log_actor_id ON audit_log (actor_id);
CREATE INDEX idx_audit_log_action ON audit_log (action);
CREATE INDEX idx_audit_log_created_at ON audit_log (created_at);

CREATE TABLE system_settings (
    key        varchar(60) PRIMARY KEY,
    value      varchar(200) NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO system_settings (key, value) VALUES ('mfa_required', '0') ON CONFLICT (key) DO NOTHING;

-- +goose Down
DROP TABLE system_settings;
DROP TABLE audit_log;
DROP TABLE ip_bans;
DROP TABLE login_attempts;
DROP TABLE sessions;
DROP TABLE user_roles;
DROP TABLE role_permissions;
DROP TABLE roles;
DROP TABLE permissions;
DROP TABLE users;
