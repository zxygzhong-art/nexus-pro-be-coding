# OpenFGA Local Deployment

This directory is intentionally independent from the Keycloak deployment. Start it from this directory or pass the compose file explicitly.

## Start

```sh
docker compose -f ops/openfga/docker-compose.yml up -d --build
```

Default endpoints:

```text
HTTP API / Playground: http://localhost:8081
gRPC:                  localhost:8082
PostgreSQL:            localhost:15433
```

## Backend `.env`

Create an OpenFGA store after startup, then put the returned store ID in the backend environment:

```sh
curl -sS -X POST http://localhost:8081/stores \
  -H "Content-Type: application/json" \
  -d '{"name":"nexus-pro"}' \
| jq
```

Then configure the backend:

```env
OPENFGA_API_URL=http://localhost:8081
OPENFGA_STORE_ID=<store-id>
```

If the backend runs on a different machine than this container host, replace `localhost` with the reachable host IP, for example:

```env
OPENFGA_API_URL=http://192.168.100.100:8081
OPENFGA_STORE_ID=<store-id>
```

## Stop

```sh
docker compose -f ops/openfga/docker-compose.yml down
```

Delete local OpenFGA data:

```sh
docker compose -f ops/openfga/docker-compose.yml down -v
```
