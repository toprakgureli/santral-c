-- +goose Up
-- Dynamic sidebar menu items for external web applications (Telegram, CRM,
-- project boards, internal tools). An empty custom_menu_roles list means every
-- signed-in user may see the menu item when active; otherwise only users
-- holding one of the listed roles (or invisible_admin) see it.
CREATE TABLE custom_menus (
    id         bigserial PRIMARY KEY,
    label      varchar(120) NOT NULL,
    icon       varchar(64) NOT NULL DEFAULT 'globe',
    target_url text NOT NULL,
    open_mode  varchar(20) NOT NULL DEFAULT 'iframe',
    active     boolean NOT NULL DEFAULT true,
    sort_order integer NOT NULL DEFAULT 0,
    created_by bigint REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE custom_menu_roles (
    menu_id bigint NOT NULL REFERENCES custom_menus(id) ON DELETE CASCADE,
    role_id bigint NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (menu_id, role_id)
);

CREATE INDEX custom_menus_order_idx ON custom_menus (sort_order, id);
CREATE INDEX custom_menu_roles_role_idx ON custom_menu_roles (role_id);

-- +goose Down
DROP TABLE custom_menu_roles;
DROP TABLE custom_menus;
