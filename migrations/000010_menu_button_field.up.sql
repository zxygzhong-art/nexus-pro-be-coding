-- UI-facing permission bindings: menus, buttons, and field policies.

CREATE TABLE iam_menu_items (
    id                     text PRIMARY KEY,
    tenant_id              text NOT NULL REFERENCES iam_tenants(id),
    application_code       text NOT NULL,
    parent_id              text,
    label                  text,
    route                  text,
    icon                   text,
    required_permission_id text REFERENCES iam_permissions(id),
    page_type              text,
    path_hierarchy         text,
    enabled_condition      jsonb NOT NULL DEFAULT '{}',
    sort_order             int NOT NULL DEFAULT 0,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_iam_menu_items_app ON iam_menu_items (tenant_id, application_code, sort_order);
CREATE INDEX ix_iam_menu_items_parent ON iam_menu_items (tenant_id, parent_id);

CREATE TABLE iam_button_actions (
    id                     text PRIMARY KEY,
    tenant_id              text NOT NULL REFERENCES iam_tenants(id),
    application_code       text,
    menu_item_id           text REFERENCES iam_menu_items(id),
    code                   text,
    label                  text,
    required_permission_id text REFERENCES iam_permissions(id),
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_iam_button_actions_menu ON iam_button_actions (tenant_id, menu_item_id);

CREATE TABLE iam_field_policies (
    id                     text PRIMARY KEY,
    tenant_id              text NOT NULL REFERENCES iam_tenants(id),
    application_code       text,
    resource_type          text NOT NULL,
    field                  text NOT NULL,
    effect                 text NOT NULL CHECK (effect IN ('visible','masked','hidden','readonly')),
    sensitivity            text,
    required_permission_id text REFERENCES iam_permissions(id),
    condition              jsonb NOT NULL DEFAULT '{}',
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_iam_field_policies_resource ON iam_field_policies (tenant_id, application_code, resource_type);
