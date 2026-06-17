# Keycloak Local Deployment

This directory is intentionally independent from the OpenFGA deployment. Start it from this directory or pass the compose file explicitly.

## Start

```sh
docker compose -f ops/keycloak/docker-compose.yml up -d --build
```

Default endpoints:

```text
Admin console: http://localhost:18080/admin
Realm base:    http://localhost:18080/realms/<realm>
```

Default local admin credentials are set by the compose defaults:

```text
KEYCLOAK_ADMIN_USER=admin
KEYCLOAK_ADMIN_PASSWORD=local-keycloak-admin
```

Override them in your shell for a non-default deployment:

```sh
export KEYCLOAK_ADMIN_USER=admin
export KEYCLOAK_ADMIN_PASSWORD='<local-secret>'
docker compose -f ops/keycloak/docker-compose.yml up -d --build
```

## Backend `.env`

For the current project realm:

```env
KEYCLOAK_ISSUER_URL=http://localhost:18080/realms/nexus-pro
KEYCLOAK_CLIENT_ID=nexus-pro-connect-api
```

If the backend runs on a different machine than this container host, replace `localhost` with the reachable host IP, for example:

```env
KEYCLOAK_ISSUER_URL=http://192.168.100.100:18080/realms/nexus-pro
KEYCLOAK_CLIENT_ID=nexus-pro-connect-api
```

## Realm HTTP Setting

For local HTTP-only development, set the realm representation field:

```json
{"sslRequired":"none"}
```

The full Keycloak setup guide is in `docs/keycloak-local-setup.md`.

## Stop

```sh
docker compose -f ops/keycloak/docker-compose.yml down
```

Delete local Keycloak data:

```sh
docker compose -f ops/keycloak/docker-compose.yml down -v
```
