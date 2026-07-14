#!/usr/bin/env bash
set -e

psql -v ON_ERROR_STOP=1 \
  --username "$POSTGRES_USER" \
  --dbname "$POSTGRES_DB" \
  -v litellm_db="$LITELLM_DB_NAME" \
  -v keycloak_db="$KEYCLOAK_DB_NAME" \
  -v openfga_db="$OPENFGA_DB_NAME" \
  -v temporal_db="$TEMPORAL_DB_NAME" \
  -v temporal_visibility_db="$TEMPORAL_VISIBILITY_DB_NAME" <<'EOSQL'
SELECT format('CREATE DATABASE %I', :'keycloak_db')
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = :'keycloak_db')\gexec

SELECT format('CREATE DATABASE %I', :'litellm_db')
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = :'litellm_db')\gexec

SELECT format('CREATE DATABASE %I', :'openfga_db')
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = :'openfga_db')\gexec

SELECT format('CREATE DATABASE %I', :'temporal_db')
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = :'temporal_db')\gexec

SELECT format('CREATE DATABASE %I', :'temporal_visibility_db')
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = :'temporal_visibility_db')\gexec
EOSQL
