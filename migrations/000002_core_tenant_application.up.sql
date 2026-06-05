-- Global registries: tenants and applications. No RLS (cross-tenant by nature).

CREATE TABLE iam_tenants (
    id                 text PRIMARY KEY,
    name               text NOT NULL,
    status             text NOT NULL DEFAULT 'active',
    permission_version bigint NOT NULL DEFAULT 1,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE iam_applications (
    id               text PRIMARY KEY,
    application_code text NOT NULL UNIQUE,
    name             text,
    status           text NOT NULL DEFAULT 'active',
    resource_types   jsonb NOT NULL DEFAULT '[]',
    actions          jsonb NOT NULL DEFAULT '[]',
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
