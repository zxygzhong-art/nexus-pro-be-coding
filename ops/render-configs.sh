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
  OTEL_COLLECTOR_INTERNAL_HOST
  OTEL_PROMETHEUS_EXPORT_INTERNAL_PORT
  OTEL_CORS_ALLOWED_ORIGIN_1
  OTEL_CORS_ALLOWED_ORIGIN_2
  OTEL_MEMORY_LIMIT_MIB
  OTEL_MEMORY_SPIKE_LIMIT_MIB
  OTEL_BATCH_TIMEOUT
  OTEL_BATCH_SEND_SIZE
  OTEL_SERVICE_NAME
  NEXUS_API_METRICS_TARGET
  NEXUS_API_LOG_INCLUDE
  NEXUS_API_LOG_START_AT
  NEXUS_API_DEPLOYMENT_ENVIRONMENT
  TEMPO_INTERNAL_HOST
  TEMPO_HTTP_INTERNAL_PORT
  LOKI_INTERNAL_HOST
  LOKI_HTTP_INTERNAL_PORT
  LOKI_GRPC_INTERNAL_PORT
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
  OBJECT_STORE_REGION
  LOKI_BUCKET
  TEMPO_BUCKET
  LOKI_RETENTION
  LOKI_SCHEMA_FROM
  LOKI_INDEX_PREFIX
  LOKI_S3_INSECURE
  LOKI_S3_FORCE_PATH_STYLE
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

cat > "${GENERATED_DIR}/otelcol.yaml" <<EOF
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:${OTEL_GRPC_INTERNAL_PORT}
      http:
        endpoint: 0.0.0.0:${OTEL_HTTP_INTERNAL_PORT}
        cors:
          allowed_origins:
            - ${OTEL_CORS_ALLOWED_ORIGIN_1}
            - ${OTEL_CORS_ALLOWED_ORIGIN_2}
  prometheus/nexus-api:
    config:
      scrape_configs:
        - job_name: ${OTEL_SERVICE_NAME}
          scrape_interval: ${PROMETHEUS_SCRAPE_INTERVAL}
          metrics_path: /metrics
          static_configs:
            - targets: ["${NEXUS_API_METRICS_TARGET}"]
  filelog/nexus-api:
    include:
      - ${NEXUS_API_LOG_INCLUDE}
    start_at: ${NEXUS_API_LOG_START_AT}
    include_file_path: true
    include_file_name: true

processors:
  memory_limiter:
    check_interval: 1s
    limit_mib: ${OTEL_MEMORY_LIMIT_MIB}
    spike_limit_mib: ${OTEL_MEMORY_SPIKE_LIMIT_MIB}
  batch:
    timeout: ${OTEL_BATCH_TIMEOUT}
    send_batch_size: ${OTEL_BATCH_SEND_SIZE}
  resource/nexus-api:
    attributes:
      - key: service.name
        value: ${OTEL_SERVICE_NAME}
        action: insert
      - key: service.namespace
        value: nexus-pro
        action: insert
      - key: deployment.environment
        value: ${NEXUS_API_DEPLOYMENT_ENVIRONMENT}
        action: insert

exporters:
  otlp/tempo:
    endpoint: ${TEMPO_INTERNAL_HOST}:${OTEL_GRPC_INTERNAL_PORT}
    tls:
      insecure: true

  prometheus:
    endpoint: 0.0.0.0:${OTEL_PROMETHEUS_EXPORT_INTERNAL_PORT}
    resource_to_telemetry_conversion:
      enabled: true
    enable_open_metrics: true

  otlphttp/loki:
    endpoint: http://${LOKI_INTERNAL_HOST}:${LOKI_HTTP_INTERNAL_PORT}/otlp
    tls:
      insecure: true

  debug:
    verbosity: basic

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [memory_limiter, resource/nexus-api, batch]
      exporters: [otlp/tempo]

    metrics:
      receivers: [otlp, prometheus/nexus-api]
      processors: [memory_limiter, resource/nexus-api, batch]
      exporters: [prometheus]

    logs:
      receivers: [otlp, filelog/nexus-api]
      processors: [memory_limiter, resource/nexus-api, batch]
      exporters: [otlphttp/loki]
EOF

cat > "${GENERATED_DIR}/prometheus.yml" <<EOF
global:
  scrape_interval: ${PROMETHEUS_SCRAPE_INTERVAL}
  evaluation_interval: ${PROMETHEUS_EVALUATION_INTERVAL}

scrape_configs:
  - job_name: prometheus
    static_configs:
      - targets: ["${PROMETHEUS_INTERNAL_HOST}:${PROMETHEUS_INTERNAL_PORT}"]

  - job_name: otel-collector
    static_configs:
      - targets: ["${OTEL_COLLECTOR_INTERNAL_HOST}:${OTEL_PROMETHEUS_EXPORT_INTERNAL_PORT}"]

  - job_name: tempo
    static_configs:
      - targets: ["${TEMPO_INTERNAL_HOST}:${TEMPO_HTTP_INTERNAL_PORT}"]

  - job_name: loki
    static_configs:
      - targets: ["${LOKI_INTERNAL_HOST}:${LOKI_HTTP_INTERNAL_PORT}"]

  - job_name: minio
    metrics_path: /minio/v2/metrics/cluster
    static_configs:
      - targets: ["${MINIO_INTERNAL_HOST}:${MINIO_API_INTERNAL_PORT}"]

  - job_name: keycloak
    metrics_path: /metrics
    static_configs:
      - targets: ["${KEYCLOAK_INTERNAL_HOST}:${KEYCLOAK_MANAGEMENT_INTERNAL_PORT}"]

  - job_name: openfga
    metrics_path: /metrics
    static_configs:
      - targets: ["${OPENFGA_INTERNAL_HOST}:${OPENFGA_METRICS_INTERNAL_PORT}"]
EOF

cat > "${GENERATED_DIR}/loki.yaml" <<EOF
auth_enabled: false

server:
  http_listen_port: ${LOKI_HTTP_INTERNAL_PORT}
  grpc_listen_port: ${LOKI_GRPC_INTERNAL_PORT}

common:
  path_prefix: /loki
  replication_factor: 1
  ring:
    kvstore:
      store: inmemory

schema_config:
  configs:
    - from: ${LOKI_SCHEMA_FROM}
      store: tsdb
      object_store: s3
      schema: v13
      index:
        prefix: ${LOKI_INDEX_PREFIX}
        period: 24h

storage_config:
  aws:
    endpoint: ${MINIO_INTERNAL_HOST}:${MINIO_API_INTERNAL_PORT}
    region: ${OBJECT_STORE_REGION}
    bucketnames: ${LOKI_BUCKET}
    access_key_id: ${MINIO_ROOT_USER}
    secret_access_key: ${MINIO_ROOT_PASSWORD}
    insecure: ${LOKI_S3_INSECURE}
    s3forcepathstyle: ${LOKI_S3_FORCE_PATH_STYLE}
  tsdb_shipper:
    active_index_directory: /loki/index
    cache_location: /loki/index_cache

compactor:
  working_directory: /loki/compactor
  retention_enabled: true
  delete_request_store: s3

limits_config:
  retention_period: ${LOKI_RETENTION}
  allow_structured_metadata: true
  volume_enabled: true

analytics:
  reporting_enabled: false
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

  - name: Loki
    uid: loki
    type: loki
    access: proxy
    url: http://${LOKI_INTERNAL_HOST}:${LOKI_HTTP_INTERNAL_PORT}
    editable: true
    jsonData:
      derivedFields:
        - name: trace_id
          datasourceUid: tempo
          matcherRegex: '"trace_id"\\s*:\\s*"([a-f0-9]+)"'
          url: '\$\${__value.raw}'
          urlDisplayLabel: Open trace
        - name: trace_id_plain
          datasourceUid: tempo
          matcherRegex: 'trace_id=([a-f0-9]+)'
          url: '\$\${__value.raw}'
          urlDisplayLabel: Open trace

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
      tracesToLogsV2:
        datasourceUid: loki
        spanStartTimeShift: "-5m"
        spanEndTimeShift: "5m"
        filterByTraceID: true
        filterBySpanID: false
        tags:
          - key: service.name
            value: service_name
      tracesToMetrics:
        datasourceUid: prometheus
EOF

cat > "${GENERATED_DIR}/temporal/dynamicconfig/development-sql.yaml" <<EOF
system.forceSearchAttributesCacheRefreshOnRead:
  - value: ${TEMPORAL_FORCE_SEARCH_ATTRIBUTES_CACHE_REFRESH}
    constraints: {}
EOF

echo "generated configs under ${GENERATED_DIR}"
