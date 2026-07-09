#!/usr/bin/env bash
set -euo pipefail

OPS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${OPS_DIR}/.env"
GENERATED_DIR="${OPS_DIR}/generated"

if [[ ! -f "${ENV_FILE}" ]]; then
  echo "missing ${ENV_FILE}" >&2
  exit 1
fi

set -a
# shellcheck source=/dev/null
source "${ENV_FILE}"
set +a

required_vars=(
  OTEL_GRPC_INTERNAL_PORT
  OTEL_HTTP_INTERNAL_PORT
  OTEL_SERVICE_NAME
  NEXUS_API_METRICS_TARGET
  NEXUS_API_DEPLOYMENT_ENVIRONMENT
  TEMPO_INTERNAL_HOST
  TEMPO_HTTP_INTERNAL_PORT
  PROMETHEUS_INTERNAL_HOST
  PROMETHEUS_INTERNAL_PORT
  MINIO_INTERNAL_HOST
  MINIO_API_INTERNAL_PORT
  KEYCLOAK_INTERNAL_HOST
  KEYCLOAK_MANAGEMENT_INTERNAL_PORT
  OPENFGA_INTERNAL_HOST
  OPENFGA_METRICS_INTERNAL_PORT
  MINIO_ROOT_USER
  MINIO_ROOT_PASSWORD
  TEMPO_BUCKET
  TEMPO_S3_INSECURE
  PROMETHEUS_SCRAPE_INTERVAL
  PROMETHEUS_EVALUATION_INTERVAL
  TEMPORAL_FORCE_SEARCH_ATTRIBUTES_CACHE_REFRESH
)

for var_name in "${required_vars[@]}"; do
  if [[ -z "${!var_name:-}" ]]; then
    echo "missing required .env value: ${var_name}" >&2
    exit 1
  fi
done

mkdir -p "${GENERATED_DIR}/grafana/provisioning/datasources" "${GENERATED_DIR}/temporal/dynamicconfig"
legacy_config="${GENERATED_DIR}/otel"col.yaml
legacy_log_store="${GENERATED_DIR}/lo"ki.yaml
legacy_log_agent="${GENERATED_DIR}/prom"tail.yaml
rm -f "${legacy_config}" "${legacy_log_store}" "${legacy_log_agent}"

cat > "${GENERATED_DIR}/prometheus.yml" <<EOF
global:
  scrape_interval: ${PROMETHEUS_SCRAPE_INTERVAL}
  evaluation_interval: ${PROMETHEUS_EVALUATION_INTERVAL}

scrape_configs:
  - job_name: prometheus
    static_configs:
      - targets: ["${PROMETHEUS_INTERNAL_HOST}:${PROMETHEUS_INTERNAL_PORT}"]

  - job_name: ${OTEL_SERVICE_NAME}
    metrics_path: /metrics
    static_configs:
      - targets: ["${NEXUS_API_METRICS_TARGET}"]

  - job_name: tempo
    static_configs:
      - targets: ["${TEMPO_INTERNAL_HOST}:${TEMPO_HTTP_INTERNAL_PORT}"]

  - job_name: keycloak
    metrics_path: /metrics
    static_configs:
      - targets: ["${KEYCLOAK_INTERNAL_HOST}:${KEYCLOAK_MANAGEMENT_INTERNAL_PORT}"]

  - job_name: openfga
    metrics_path: /metrics
    static_configs:
      - targets: ["${OPENFGA_INTERNAL_HOST}:${OPENFGA_METRICS_INTERNAL_PORT}"]
EOF

cat > "${GENERATED_DIR}/tempo.yaml" <<EOF
server:
  http_listen_port: ${TEMPO_HTTP_INTERNAL_PORT}

distributor:
  receivers:
    otlp:
      protocols:
        grpc:
          endpoint: 0.0.0.0:${OTEL_GRPC_INTERNAL_PORT}
        http:
          endpoint: 0.0.0.0:${OTEL_HTTP_INTERNAL_PORT}

storage:
  trace:
    backend: s3
    wal:
      path: /var/tempo/wal
    s3:
      endpoint: ${MINIO_INTERNAL_HOST}:${MINIO_API_INTERNAL_PORT}
      bucket: ${TEMPO_BUCKET}
      access_key: ${MINIO_ROOT_USER}
      secret_key: ${MINIO_ROOT_PASSWORD}
      insecure: ${TEMPO_S3_INSECURE}
EOF

cat > "${GENERATED_DIR}/grafana/provisioning/datasources/datasources.yaml" <<EOF
apiVersion: 1

datasources:
  - name: Prometheus
    uid: prometheus
    type: prometheus
    access: proxy
    url: http://${PROMETHEUS_INTERNAL_HOST}:${PROMETHEUS_INTERNAL_PORT}
    isDefault: true
    editable: true

  - name: Tempo
    uid: tempo
    type: tempo
    access: proxy
    url: http://${TEMPO_INTERNAL_HOST}:${TEMPO_HTTP_INTERNAL_PORT}
    editable: true
    jsonData:
      nodeGraph:
        enabled: true
      serviceMap:
        datasourceUid: prometheus
      tracesToMetrics:
        datasourceUid: prometheus
EOF

cat > "${GENERATED_DIR}/temporal/dynamicconfig/development-sql.yaml" <<EOF
system.forceSearchAttributesCacheRefreshOnRead:
  - value: ${TEMPORAL_FORCE_SEARCH_ATTRIBUTES_CACHE_REFRESH}
    constraints: {}
EOF

echo "generated configs under ${GENERATED_DIR}"
