# 後端第三方服務部署清單

面向 SRE，範圍僅包含 `nexus-pro-api` 運行時依賴。

鏡像版本覈對日期：2026-07-14。

## 1. 生產必須部署

| 服務 | 用途 | 需要提供 |
| --- | --- | --- |
| PostgreSQL + pgvector | 業務數據、審批投影、outbox、知識向量 | runtime `DB_*`、部署 job 的 `MIGRATION_DATABASE_URL`、TLS、備份；業務庫啓用 `pg_trgm`、`vector` |
| Temporal | 表單提交和審批 workflow | gRPC 地址、namespace、task queue；API 啓動時必須可連接 |
| Keycloak | OIDC 登錄、JWT/JWKS 校驗 | HTTPS issuer、client ID、realm、token mapper |
| OpenFGA | 關係權限和 tuple 同步 | HTTPS API、store ID、已發佈的 model ID |
| SFTPGo 或 local 持久卷 | 導入文件、頭像、Agent/知識庫附件 | SFTPGo endpoint/root/賬號，或共享持久目錄 |

說明：

- PostgreSQL 和 Temporal 是所有環境的啓動硬依賴。
- `APP_ENV=production` 還強制要求 Keycloak、OpenFGA 和明確的對象存儲。
- 表單審批已經是 Temporal-only，沒有舊同步流程 fallback。

## 2. 按功能部署

| 服務 | 啓用場景 | 關閉時影響 |
| --- | --- | --- |
| Redis | 多副本授權緩存、共享限流 | 限流退回進程內；授權快照緩存關閉 |
| LiteLLM + 上游模型 | Agent Chat、embedding、模型路由管理 | AI/知識庫相關功能不可用，其他 API 可運行 |
| NATS JetStream | 異步事件和 OpenFGA durable consumer | 使用數據庫 outbox + 直接 OpenFGA writer |
| eHRMS | 組織、崗位、員工、考勤、假期同步 | 僅同步功能不可用 |
| Prometheus | 指標採集 | 不影響業務 API |
| Tempo | Trace 存儲 | 不影響業務 API |
| Grafana | 監控展示 | 不影響業務 API |

Keycloak 用戶自動開通還需要 Admin client ID/secret；邀請或重置密碼需要在 Keycloak 配置 SMTP。Google/Microsoft SSO 需要對應 IdP client ID/secret。

## 3. 後端主要配置

```text
PostgreSQL runtime: DB_HOST DB_PORT DB_USERNAME DB_PASSWORD DB_NAME DB_SSLMODE
PostgreSQL deploy:  MIGRATION_DATABASE_URL MIGRATION_DB_OWNER
                    RUNTIME_DB_USERNAME RUNTIME_DB_PASSWORD
Temporal:   TEMPORAL_BASE_URL TEMPORAL_NAMESPACE TEMPORAL_TASK_QUEUE
            WORKFLOW_START_OUTBOX_ENABLED OUTBOX_DISPATCH_ENABLED
Keycloak:   KEYCLOAK_BASE_URL KEYCLOAK_CLIENT_ID
OpenFGA:    OPENFGA_BASE_URL OPENFGA_STORE_ID OPENFGA_MODEL_ID
SFTPGo:     OBJECT_STORE_PROVIDER SFTPGO_BASE_URL SFTPGO_ROOT_BUCKET
            SFTPGO_USERNAME SFTPGO_PASSWORD
Redis:      REDIS_HOST REDIS_PORT REDIS_PASSWORD REDIS_DB
LiteLLM:    LITELLM_BASE_URL LITELLM_API_KEY LITELLM_MASTER_KEY
NATS:       NATS_ENABLED NATS_BASE_URL NATS_STREAM NATS_CONSUMER_PREFIX
eHRMS:      EHRMS_BASE_URL EHRMS_API_KEY EHRMS_SYNC_*
Telemetry:  OTEL_ENABLED OTEL_BASE_URL METRICS_ADDR
```

另需生成並託管統一的 `ENCRYPTION_KEY`，用於加密 Agent 模型 API key、MCP/外部工具憑據以及其他持久化密鑰。

## 4. 當前穩定鏡像

| 服務 | 鏡像 |
| --- | --- |
| PostgreSQL + pgvector | `pgvector/pgvector:0.8.5-pg18-bookworm` |
| Redis | `redis:8.8.0` |
| Keycloak | `quay.io/keycloak/keycloak:26.7.0` |
| OpenFGA | `openfga/openfga:v1.18.1` |
| LiteLLM | `ghcr.io/berriai/litellm:v1.92.0` |
| SFTPGo | `drakkan/sftpgo:v2.7.4` |
| Prometheus | `prom/prometheus:v3.13.1` |
| Tempo | `grafana/tempo:3.0.2` |
| Grafana | `grafana/grafana:13.1.0` |
| Temporal Server | `temporalio/auto-setup:1.29.7` |
| Temporal UI | `temporalio/ui:2.52.1` |
| Temporal Admin Tools | `temporalio/admin-tools:1.31.2` |
| NATS | `nats:2.14.3` |

`temporalio/auto-setup` 當前最新可用穩定 tag 仍是 `1.29.7`；Temporal Server 的代碼 release 雖更新，但該鏡像倉庫尚未發佈對應新 tag。

## 5. 部署注意事項

- `ops/.env` 是基礎設施配置，不能代替後端 `.env`；例如容器使用 `POSTGRES_*`，API 使用 `DB_*`。
- `POSTGRES_*` / `MIGRATION_DATABASE_URL` 屬於初始化與遷移權限，不得注入 API。先執行 `make migrate-up` 與 `make db-provision-runtime-role`，API 僅使用開通後的 `nexus_app` runtime `DB_*`。
- 不要使用 `COMPOSE_PROFILES=all` 直接部署生產，應按實際功能選擇 profile。
- 已有環境升級 Keycloak 前先備份數據庫，並按官方升級指南完成預發佈驗證。
- PostgreSQL、Redis、OpenFGA、Temporal、NATS、LiteLLM、SFTPGo 僅允許內網訪問。
- Keycloak 對用戶入口使用 HTTPS；management port 僅允許運維網訪問。
- `/readyz` 檢查 PostgreSQL、Temporal，以及已配置的 Keycloak、OpenFGA、Redis、NATS；不會檢查 LiteLLM、eHRMS 和對象存儲。
- OpenFGA、Temporal、NATS、Redis 當前客戶端安全配置面有限，應放在可信私網或 service mesh 內。

## 6. 當前阻塞

`ops/render-configs.sh` 仍要求 `MINIO_*`，但當前 `.env` 和 compose 沒有 MinIO，執行會失敗：

```text
missing required .env value: MINIO_INTERNAL_HOST
```

啓用 Tempo 前需提供 S3-compatible/MinIO 存儲，或修改 Tempo 存儲配置並刪除腳本中的過期強制項。

## 7. 上線驗收

- [ ] 執行業務數據庫 migration，並確認 `pg_trgm`、`vector` 可用
- [ ] 創建 Keycloak realm/client/token mapper
- [ ] 創建 OpenFGA store，應用 [`../openfga/model.json`](../openfga/model.json)
- [ ] 確認 Temporal namespace 和 task queue 可用
- [ ] 驗證 `/healthz`、`/readyz`
- [ ] 完成登錄、文件上傳、表單審批 smoke
- [ ] 按啓用功能驗證 LiteLLM、NATS、eHRMS
- [ ] 所有密鑰進入 secrets manager，並完成備份/恢復策略

代碼依據：[`../../internal/config/config.go`](../../internal/config/config.go)、[`../../cmd/api/bootstrap.go`](../../cmd/api/bootstrap.go)、[`../compose.yaml`](../compose.yaml)、[`../../.env.example`](../../.env.example)。
