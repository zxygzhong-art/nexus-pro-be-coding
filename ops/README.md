# Ops 部署堆疊

本目錄是本地基礎設施與可觀測性服務的統一入口。所有執行期設定集中在一個檔案：[`.env`](.env)。不要直接修改產生出來的 YAML 檔。

這套堆疊包含 Grafana、OpenTelemetry Collector、Tempo、Prometheus、Loki、Keycloak、OpenFGA、Temporal、NATS JetStream、Redis、PostgreSQL 和 MinIO，所有映像都使用固定版本標籤。MinIO 是後端、Loki、Tempo 預設使用的 S3-compatible 物件儲存。

## 啟動

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be/ops
./render-configs.sh
docker compose --env-file .env up -d
```

[`.env`](.env) 內預設 `COMPOSE_PROFILES=all`，會啟動所有服務。修改 port、密碼、映像版本、bucket、retention 或 telemetry endpoint 後，需要重新執行 `./render-configs.sh`。

產生的設定檔會寫到 `ops/generated/`，並由 Git 忽略。這些檔案只存在於執行期，因為 Grafana、Prometheus、Loki、Tempo 和 OTel Collector 都需要各自的執行期設定格式。

單服務映像建置方式請看 [dockerfiles/README.md](dockerfiles/README.md)。

後端物件儲存與 S3-compatible 部署說明請看 [docs/s3-minio.md](docs/s3-minio.md)。

Keycloak 部署與前後端對接說明請看 [docs/keycloak.md](docs/keycloak.md)。

## Dockerfile 邊界

單服務 Dockerfile 會刻意保持很小，只固定基礎映像，並宣告標準 port。

不要把帳號、密碼、資料庫地址、bucket 名稱、tracing endpoint、對外主機名或環境相關設定寫進 Dockerfile。這些值屬於執行期設定，應該放在 [`.env`](.env)，再由 `compose.yaml` 注入，或由 `render-configs.sh` 產生到 `ops/generated/`。

這樣同一個映像才能同時用於本地、測試、正式環境和多主機拆分部署。例如同一個 Keycloak 映像可以連本地 PostgreSQL 容器，也可以連另一台主機上的 PostgreSQL，只需要修改 [`.env`](.env) 裡的 `POSTGRES_INTERNAL_HOST`、`POSTGRES_INTERNAL_PORT`、`POSTGRES_USER` 和 `POSTGRES_PASSWORD`。

## 設定檔結構

[`ops/.env`](.env) 按服務分組，而不是按設定類型分組：

```text
全域
PostgreSQL
Redis
Keycloak
OpenFGA
Temporal
NATS JetStream
MinIO
後端物件儲存
Loki
Tempo
OpenTelemetry Collector
Prometheus
Grafana
```

同一個服務的設定會放在一起。例如 PostgreSQL 區塊會集中放 `POSTGRES_IMAGE`、`POSTGRES_INTERNAL_HOST`、`POSTGRES_HOST_PORT`、`POSTGRES_USER` 和 PostgreSQL healthcheck 設定。

每個變數都有繁體中文註釋。主機相關變數遵循以下約定：

```text
*_INTERNAL_HOST: 容器或服務之間互相連線的地址；依賴服務已部署到其他主機時，改成該主機 IP 或 DNS。
*_BIND_HOST: 目前這台 Docker 主機發佈 port 時綁定的網卡地址。
```

## 部署單一服務

每個服務都有對應的 Docker Compose profile：

| 服務 | Profile | Compose 服務名稱 |
| --- | --- | --- |
| PostgreSQL | `postgres` | `postgres` |
| Redis | `redis` | `redis` |
| Keycloak | `keycloak` | `keycloak` |
| OpenFGA | `openfga` | `openfga`, `openfga-migrate` |
| Temporal | `temporal` | `temporal`, `temporal-ui`, `temporal-admin-tools` |
| NATS JetStream | `nats` | `nats` |
| MinIO | `minio` | `minio`, `minio-init` |
| Loki | `loki` | `loki` |
| Tempo | `tempo` | `tempo` |
| OpenTelemetry Collector | `otel` | `otel-collector` |
| Prometheus | `prometheus` | `prometheus` |
| Grafana | `grafana` | `grafana` |

只部署 PostgreSQL：

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be/ops
COMPOSE_PROFILES=postgres docker compose --env-file .env up -d postgres
```

只部署 Redis：

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be/ops
COMPOSE_PROFILES=redis docker compose --env-file .env up -d redis
```

只部署 MinIO 並初始化 bucket：

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be/ops
COMPOSE_PROFILES=minio docker compose --env-file .env up -d minio minio-init
```

只部署 NATS JetStream：

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be/ops
COMPOSE_PROFILES=nats docker compose --env-file .env up -d nats
```

PostgreSQL 已在其他主機部署，只部署 Temporal：

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be/ops
# 先在 .env 裡把 POSTGRES_INTERNAL_HOST、POSTGRES_INTERNAL_PORT、POSTGRES_USER、POSTGRES_PASSWORD 指向既有資料庫主機。
./render-configs.sh
COMPOSE_PROFILES=temporal docker compose --env-file .env up -d --no-deps temporal temporal-ui temporal-admin-tools
```

PostgreSQL 已在其他主機部署，只部署 Keycloak：

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be/ops
# 先在 .env 裡把 POSTGRES_INTERNAL_HOST、POSTGRES_INTERNAL_PORT、POSTGRES_USER、POSTGRES_PASSWORD 指向既有資料庫主機。
COMPOSE_PROFILES=keycloak docker compose --env-file .env up -d --no-deps keycloak
```

Prometheus、Loki、Tempo 已在其他主機部署，只部署 Grafana：

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be/ops
# 先在 .env 裡把 PROMETHEUS_INTERNAL_HOST、LOKI_INTERNAL_HOST、TEMPO_INTERNAL_HOST 指向既有服務主機。
./render-configs.sh
COMPOSE_PROFILES=grafana docker compose --env-file .env up -d --no-deps grafana
```

MinIO 已在其他主機部署，只部署 Loki：

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be/ops
# 先在 .env 裡把 MINIO_INTERNAL_HOST、MINIO_API_INTERNAL_PORT、MINIO_ROOT_USER、MINIO_ROOT_PASSWORD 指向既有 MinIO 主機。
./render-configs.sh
COMPOSE_PROFILES=loki docker compose --env-file .env up -d --no-deps loki
```

當某個依賴已經部署在此 compose project 外部時，使用 `--no-deps`。不加 `--no-deps` 時，Docker Compose 可能會嘗試啟動已宣告的依賴服務。

## 版本

映像標籤都設定在 [`.env`](.env)：

- Grafana：`GRAFANA_IMAGE`
- OpenTelemetry Collector Contrib：`OTEL_COLLECTOR_IMAGE`
- Tempo：`TEMPO_IMAGE`
- Prometheus：`PROMETHEUS_IMAGE`
- Loki：`LOKI_IMAGE`
- Keycloak：`KEYCLOAK_IMAGE`
- OpenFGA：`OPENFGA_IMAGE`
- Temporal：`TEMPORAL_IMAGE`
- Temporal UI：`TEMPORAL_UI_IMAGE`
- Temporal Admin Tools：`TEMPORAL_ADMIN_TOOLS_IMAGE`
- NATS JetStream：`NATS_IMAGE`
- Redis：`REDIS_IMAGE`
- PostgreSQL：`POSTGRES_IMAGE`
- MinIO：`MINIO_IMAGE`
- MinIO Client：`MINIO_CLIENT_IMAGE`

## 本地 URL

```text
Grafana:        http://127.0.0.1:24000
Redis:          127.0.0.1:26379
Keycloak:       http://127.0.0.1:8080
OpenFGA API:    http://127.0.0.1:24081
OpenFGA gRPC:   127.0.0.1:24082
OpenFGA UI:     http://127.0.0.1:24001
Temporal gRPC:  127.0.0.1:27233
Temporal UI:    http://127.0.0.1:24088
NATS client:    nats://127.0.0.1:24222
NATS monitor:   http://127.0.0.1:28222
Prometheus:     http://127.0.0.1:24090
Loki:           http://127.0.0.1:24100
Tempo:          http://127.0.0.1:24200
MinIO API:      http://127.0.0.1:24900
MinIO Console:  http://127.0.0.1:24901
```

預設本地帳密也在 [`.env`](.env)。任何非本地部署都必須先更換。

## 應用程式端點

後端的三類信號都以 OpenTelemetry Collector 為統一入口：

- traces：後端用 OTLP gRPC 上報到 Collector，再由 Collector 送 Tempo。
- metrics：Collector scrape 後端 `/metrics`，再暴露統一 Prometheus exporter 給 Prometheus。
- logs：後端 JSON stdout 落到 `logs/*.log`，Collector 用 filelog receiver 讀取後送 Loki。

在宿主機上執行 Go 後端時使用：

```bash
OTEL_ENABLED=true
OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:24317
OTEL_EXPORTER_OTLP_INSECURE=true
OTEL_SERVICE_NAME=nexus-pro-be
METRICS_ADDR=0.0.0.0:9091
```

同時把 stdout 寫入 Collector 掛載的本地日誌目錄：

```bash
mkdir -p /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be/logs
go run ./cmd/api 2>&1 | tee /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be/logs/nexus-pro-be.log
```

在同一個 Docker network 內執行 Go 後端時，trace endpoint 改成 Collector service name：

```bash
OTEL_ENABLED=true
OTEL_EXPORTER_OTLP_ENDPOINT=otel-collector:4317
OTEL_EXPORTER_OTLP_INSECURE=true
```

後端物件儲存預設值：

```bash
OBJECT_STORE_PROVIDER=minio
OBJECT_STORE_ENDPOINT=http://127.0.0.1:24900
OBJECT_STORE_BUCKET=nexus-hr-imports
OBJECT_STORE_REGION=us-east-1
OBJECT_STORE_ACCESS_KEY_ID=minioadmin
OBJECT_STORE_SECRET_ACCESS_KEY=minioadmin
OBJECT_STORE_USE_SSL=false
OBJECT_STORE_CREATE_BUCKET=true
```

後端 Redis 預設值：

```bash
REDIS_ADDR=127.0.0.1:26379
REDIS_PASSWORD=
REDIS_DB=0
```

Temporal 與 NATS 本地端點：

```bash
TEMPORAL_ADDRESS=127.0.0.1:27233
NATS_URL=nats://127.0.0.1:24222
```

後端不會自動載入 `.env`；從宿主機啟動 API 時需要明確 source：

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be
set -a
source ops/.env
set +a
export OTEL_ENABLED=true
export METRICS_ADDR=0.0.0.0:9091
mkdir -p logs
go run ./cmd/api 2>&1 | tee logs/nexus-pro-be.log
```

## 停止

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be/ops
docker compose --env-file .env down
```

同時移除持久化資料：

```bash
docker compose --env-file .env down -v
```
