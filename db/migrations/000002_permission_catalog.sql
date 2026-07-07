-- +goose Up

CREATE TABLE permissions (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    application text NOT NULL,
    resource text NOT NULL,
    action text NOT NULL,
    permission_type text NOT NULL CHECK (permission_type IN ('menu', 'api', 'button', 'field', 'scope')),
    menu_key text NOT NULL DEFAULT '',
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    high_risk boolean NOT NULL DEFAULT false,
    severity text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    CONSTRAINT permissions_tenant_id_id_idx UNIQUE (tenant_id, id)
);

CREATE UNIQUE INDEX permissions_tenant_catalog_unique_idx ON permissions (
    tenant_id, application, resource, action, permission_type
);
CREATE INDEX permissions_tenant_id_idx ON permissions (tenant_id);
CREATE INDEX permissions_tenant_menu_key_idx ON permissions (tenant_id, menu_key) WHERE menu_key <> '';

CREATE TABLE menu_items (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    key text NOT NULL,
    label text NOT NULL,
    path text NOT NULL DEFAULT '',
    icon text NOT NULL DEFAULT '',
    parent_key text NOT NULL DEFAULT '',
    sort_order integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL,
    CONSTRAINT menu_items_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT menu_items_tenant_key_idx UNIQUE (tenant_id, key)
);

CREATE INDEX menu_items_tenant_parent_idx ON menu_items (tenant_id, parent_key, sort_order);

CREATE TABLE permission_set_items (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    permission_set_id text NOT NULL,
    permission_id text NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT permission_set_items_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT permission_set_items_unique_idx UNIQUE (tenant_id, permission_set_id, permission_id),
    CONSTRAINT permission_set_items_set_fk FOREIGN KEY (tenant_id, permission_set_id) REFERENCES permission_sets (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT permission_set_items_permission_fk FOREIGN KEY (tenant_id, permission_id) REFERENCES permissions (tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX permission_set_items_tenant_set_idx ON permission_set_items (tenant_id, permission_set_id);
CREATE INDEX permission_set_items_tenant_permission_idx ON permission_set_items (tenant_id, permission_id);

ALTER TABLE permissions ENABLE ROW LEVEL SECURITY;
ALTER TABLE permissions FORCE ROW LEVEL SECURITY;
ALTER TABLE menu_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE menu_items FORCE ROW LEVEL SECURITY;
ALTER TABLE permission_set_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE permission_set_items FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_permissions ON permissions USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_menu_items ON menu_items USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_permission_set_items ON permission_set_items USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

-- +goose Down

DROP TABLE IF EXISTS permission_set_items;
DROP TABLE IF EXISTS menu_items;
DROP TABLE IF EXISTS permissions;
