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

Create an OpenFGA store after startup:

```sh
curl -sS -X POST http://localhost:8081/stores \
  -H "Content-Type: application/json" \
  -d '{"name":"nexus-pro"}' \
| jq
```

Apply the versioned authorization model and keep the returned `authorization_model_id`:

```sh
make openfga-apply-model \
  OPENFGA_API_URL=http://localhost:8081 \
  OPENFGA_STORE_ID=<store-id> \
| jq
```

You can verify the model ID later:

```sh
make openfga-check-model \
  OPENFGA_API_URL=http://localhost:8081 \
  OPENFGA_STORE_ID=<store-id> \
  OPENFGA_MODEL_ID=<authorization-model-id> \
| jq
```

Then configure the backend. `/readyz` checks that this model ID is readable from the configured store.

```env
OPENFGA_API_URL=http://localhost:8081
OPENFGA_STORE_ID=<store-id>
OPENFGA_MODEL_ID=<authorization-model-id>
```

If the backend runs on a different machine than this container host, replace `localhost` with the reachable host IP, for example:

```env
OPENFGA_API_URL=http://192.168.100.100:8081
OPENFGA_STORE_ID=<store-id>
OPENFGA_MODEL_ID=<authorization-model-id>
```

## Stop

```sh
docker compose -f ops/openfga/docker-compose.yml down
```

Delete local OpenFGA data:

```sh
docker compose -f ops/openfga/docker-compose.yml down -v
```
