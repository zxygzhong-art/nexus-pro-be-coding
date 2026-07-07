# 單服務 Dockerfile

建置 context 必須是 `ops` 目錄。執行期設定仍然來自 [../.env](../.env)。

這些 Dockerfile 刻意不包含帳號、密碼、資料庫 URL、S3 bucket 名稱、tracing endpoint 或主機相關設定。這些值都屬於執行期設定，必須保留在 [../.env](../.env)。

啟動需要設定檔的服務前，先產生執行期設定檔：

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be/ops
./render-configs.sh
```

## 建置映像

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be/ops
set -a
source .env
set +a

docker build --build-arg POSTGRES_IMAGE="${POSTGRES_IMAGE}" -f dockerfiles/postgres/Dockerfile -t nexus-pro-be/postgres:18.4 .
docker build --build-arg REDIS_IMAGE="${REDIS_IMAGE}" -f dockerfiles/redis/Dockerfile -t nexus-pro-be/redis:8.8.0 .
docker build --build-arg GRAFANA_IMAGE="${GRAFANA_IMAGE}" -f dockerfiles/grafana/Dockerfile -t nexus-pro-be/grafana:13.1.0 .
docker build --build-arg KEYCLOAK_IMAGE="${KEYCLOAK_IMAGE}" -f dockerfiles/keycloak/Dockerfile -t nexus-pro-be/keycloak:26.6.4 .
docker build --build-arg OPENFGA_IMAGE="${OPENFGA_IMAGE}" -f dockerfiles/openfga/Dockerfile -t nexus-pro-be/openfga:v1.18.1 .
docker build --build-arg TEMPORAL_IMAGE="${TEMPORAL_IMAGE}" -f dockerfiles/temporal/Dockerfile -t nexus-pro-be/temporal:1.29.7 .
docker build --build-arg TEMPORAL_UI_IMAGE="${TEMPORAL_UI_IMAGE}" -f dockerfiles/temporal-ui/Dockerfile -t nexus-pro-be/temporal-ui:2.51.1 .
docker build --build-arg TEMPORAL_ADMIN_TOOLS_IMAGE="${TEMPORAL_ADMIN_TOOLS_IMAGE}" -f dockerfiles/temporal-admin-tools/Dockerfile -t nexus-pro-be/temporal-admin-tools:1.29.7-tctl-1.18.4-cli-1.7.2 .
docker build --build-arg NATS_IMAGE="${NATS_IMAGE}" -f dockerfiles/nats/Dockerfile -t nexus-pro-be/nats:2.14.3 .
docker build --build-arg OTEL_COLLECTOR_IMAGE="${OTEL_COLLECTOR_IMAGE}" -f dockerfiles/otel-collector/Dockerfile -t nexus-pro-be/otel-collector:0.155.0 .
docker build --build-arg TEMPO_IMAGE="${TEMPO_IMAGE}" -f dockerfiles/tempo/Dockerfile -t nexus-pro-be/tempo:3.0.2 .
docker build --build-arg PROMETHEUS_IMAGE="${PROMETHEUS_IMAGE}" -f dockerfiles/prometheus/Dockerfile -t nexus-pro-be/prometheus:v3.13.0 .
docker build --build-arg LOKI_IMAGE="${LOKI_IMAGE}" -f dockerfiles/loki/Dockerfile -t nexus-pro-be/loki:3.7.3 .
docker build --build-arg MINIO_IMAGE="${MINIO_IMAGE}" -f dockerfiles/minio/Dockerfile -t nexus-pro-be/minio:RELEASE.2025-09-07T16-13-09Z .
```

建置參數按服務分組放在 [../.env](../.env)。例如 `POSTGRES_IMAGE` 在 PostgreSQL 區塊，`MINIO_IMAGE` 在 MinIO 區塊。

## 執行期說明

這些 Dockerfile 一次只包一個服務。服務在執行期仍然會有依賴關係：

- Redis 可被後端用於 authz snapshot cache 與 rate limiting。
- Keycloak 需要 PostgreSQL，並會把 traces 送到 `otel-collector:4317`。
- OpenFGA 需要 PostgreSQL，必須先執行 `migrate` 再執行 `run`，並會把 traces 送到 `otel-collector:4317`。
- Temporal 需要 PostgreSQL，並使用 `temporal` 與 `temporal_visibility` 兩個 database。
- NATS 使用 JetStream，資料會持久化到 volume 掛載的 `/data/jetstream`。
- OTel Collector 預期 Tempo 位於 `tempo:4317`，Loki 位於 `loki:3100`。
- Prometheus 會 scrape `otel-collector:8889`、`tempo:3200`、`loki:3100`、`keycloak:9000`、`openfga:2112` 和 `minio:9000`。
- Grafana 資料來源指向 `prometheus:9090`、`loki:3100` 和 `tempo:3200`。
- MinIO 提供預設 S3-compatible bucket 給後端 `nexus-hr-imports`、Loki `loki` 和 Tempo `tempo`。

單獨執行容器前，先建立共用 Docker network：

```bash
docker network create observability
```

MinIO 單獨執行範例：

```bash
docker volume create minio-data
docker run -d --name minio --network observability \
  -p 9000:9000 -p 9001:9001 \
  -e MINIO_ROOT_USER=minioadmin \
  -e MINIO_ROOT_PASSWORD=minioadmin \
  -e MINIO_PROMETHEUS_AUTH_TYPE=public \
  -v minio-data:/data \
  nexus-pro-be/minio:RELEASE.2025-09-07T16-13-09Z
```

初始化預設 bucket：

```bash
docker run --rm --network observability \
  --entrypoint /bin/sh \
  minio/mc:RELEASE.2025-08-13T08-35-41Z \
  -ec 'until mc alias set local http://minio:9000 minioadmin minioadmin; do sleep 2; done; mc mb --ignore-existing local/nexus-hr-imports; mc mb --ignore-existing local/loki; mc mb --ignore-existing local/tempo'
```
