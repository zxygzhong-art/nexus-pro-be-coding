# 單服務 Dockerfile

建置 context 必須是 `ops` 目錄。執行期設定仍然來自 [../.env](../.env)。

這些 Dockerfile 刻意不包含帳號、密碼、資料庫 URL、物件儲存根目錄、tracing endpoint 或主機相關設定。這些值都屬於執行期設定，必須保留在 [../.env](../.env)。

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
docker build --build-arg KEYCLOAK_IMAGE="${KEYCLOAK_IMAGE}" -f dockerfiles/keycloak/Dockerfile -t nexus-pro-be/keycloak:26.7.0 .
docker build --build-arg OPENFGA_IMAGE="${OPENFGA_IMAGE}" -f dockerfiles/openfga/Dockerfile -t nexus-pro-be/openfga:v1.18.1 .
docker build --build-arg TEMPORAL_IMAGE="${TEMPORAL_IMAGE}" -f dockerfiles/temporal/Dockerfile -t nexus-pro-be/temporal:1.29.7 .
docker build --build-arg TEMPORAL_UI_IMAGE="${TEMPORAL_UI_IMAGE}" -f dockerfiles/temporal-ui/Dockerfile -t nexus-pro-be/temporal-ui:2.52.1 .
docker build --build-arg TEMPORAL_ADMIN_TOOLS_IMAGE="${TEMPORAL_ADMIN_TOOLS_IMAGE}" -f dockerfiles/temporal-admin-tools/Dockerfile -t nexus-pro-be/temporal-admin-tools:1.31.2 .
docker build --build-arg NATS_IMAGE="${NATS_IMAGE}" -f dockerfiles/nats/Dockerfile -t nexus-pro-be/nats:2.14.3 .
docker build --build-arg TEMPO_IMAGE="${TEMPO_IMAGE}" -f dockerfiles/tempo/Dockerfile -t nexus-pro-be/tempo:3.0.2 .
docker build --build-arg PROMETHEUS_IMAGE="${PROMETHEUS_IMAGE}" -f dockerfiles/prometheus/Dockerfile -t nexus-pro-be/prometheus:v3.13.1 .
docker build --build-arg SFTPGO_IMAGE="${SFTPGO_IMAGE}" -f dockerfiles/sftpgo/Dockerfile -t nexus-pro-be/sftpgo:v2.7.4 .
```

建置參數按服務分組放在 [../.env](../.env)。例如 `POSTGRES_IMAGE` 在 PostgreSQL 區塊，`SFTPGO_IMAGE` 在 SFTPGo 區塊。

## 執行期說明

這些 Dockerfile 一次只包一個服務。服務在執行期仍然會有依賴關係：

- Redis 可被後端用於 authz snapshot cache 與 rate limiting。
- Keycloak 需要 PostgreSQL，並會把 traces 送到 `tempo:4317`。
- OpenFGA 需要 PostgreSQL，必須先執行 `migrate` 再執行 `run`，並會把 traces 送到 `tempo:4317`。
- Temporal 需要 PostgreSQL，並使用 `temporal` 與 `temporal_visibility` 兩個 database。
- NATS 使用 JetStream，資料會持久化到 volume 掛載的 `/data/jetstream`。
- Prometheus 會 scrape `host.docker.internal:9091`、`tempo:3200`、`keycloak:9000` 和 `openfga:2112`。
- 後端 JSON 日誌只直接輸出到 stdout / 控制台。
- Grafana 資料來源指向 `prometheus:9090` 和 `tempo:3200`。
- SFTPGo 提供後端業務文件儲存，預設遠端根目錄是 `nexus-hr-imports`。

單獨執行容器前，先建立共用 Docker network：

```bash
docker network create observability
```

SFTPGo 單獨執行範例（完整 `serve` 模式，啟用 REST API）：

```bash
./render-configs.sh
docker volume create sftpgo-data
docker volume create sftpgo-home
docker run -d --name sftpgo --network observability \
  -p 22022:2022 \
  -p 28080:8080 \
  -e SFTPGO_SFTPD__BINDINGS__0__PORT=2022 \
  -e SFTPGO_HTTPD__BINDINGS__0__PORT=8080 \
  -e SFTPGO_HTTPD__BINDINGS__0__ENABLE_REST_API=true \
  -e SFTPGO_DATA_PROVIDER__USERS_BASE_DIR=/srv/sftpgo/data \
  -v sftpgo-data:/srv/sftpgo/data \
  -v sftpgo-home:/var/lib/sftpgo \
  -v "$(pwd)/generated/sftpgo/loaddata.json:/etc/sftpgo/loaddata.json:ro" \
  nexus-pro-be/sftpgo:v2.7.4 \
  sftpgo serve --config-dir /var/lib/sftpgo --loaddata-from /etc/sftpgo/loaddata.json --loaddata-mode 0
```
