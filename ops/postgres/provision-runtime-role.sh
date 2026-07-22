#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
psql_bin="${PSQL:-psql}"
migration_database_url="${MIGRATION_DATABASE_URL:-${MIGRATION_DATABASE_URL_EFFECTIVE:-}}"
runtime_role="${RUNTIME_DB_USERNAME:-nexus_app}"
runtime_password="${RUNTIME_DB_PASSWORD:-}"
migration_owner="${MIGRATION_DB_OWNER:-}"

if [[ -z "$migration_database_url" ]]; then
  echo "MIGRATION_DATABASE_URL is required" >&2
  exit 1
fi
if [[ -z "$runtime_role" ]]; then
  echo "RUNTIME_DB_USERNAME is required" >&2
  exit 1
fi
if [[ -z "$runtime_password" ]]; then
  echo "RUNTIME_DB_PASSWORD is required" >&2
  exit 1
fi

if [[ -z "$migration_owner" ]]; then
  migration_owner="$($psql_bin "$migration_database_url" -v ON_ERROR_STOP=1 -Atqc 'select current_user')"
fi

export RUNTIME_DB_PASSWORD
"$psql_bin" "$migration_database_url" \
  -v ON_ERROR_STOP=1 \
  -v runtime_role="$runtime_role" \
  -v migration_owner="$migration_owner" \
  -f "$script_dir/provision-runtime-role.sql"
