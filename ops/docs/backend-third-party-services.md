# 后端第三方服务部署清单

面向 SRE，范围仅包含 `nexus-pro-be` 运行时依赖。

镜像版本核对日期：2026-07-14。

## 1. 生产必须部署

| 服务 | 用途 | 需要提供 |
| --- | --- | --- |
| PostgreSQL + pgvector | 业务数据、审批投影、outbox、知识向量 | `DB_*`、TLS、备份；业务库启用 `pg_trgm`、`vector` |
| Temporal | 表单提交和审批 workflow | gRPC 地址、namespace、task queue；API 启动时必须可连接 |
| Keycloak | OIDC 登录、JWT/JWKS 校验 | HTTPS issuer、client ID、realm、token mapper |
| OpenFGA | 关系权限和 tuple 同步 | HTTPS API、store ID、已发布的 model ID |
| SFTPGo 或 local 持久卷 | 导入文件、头像、Agent/知识库附件 | SFTPGo endpoint/root/账号，或共享持久目录 |

说明：

- PostgreSQL 和 Temporal 是所有环境的启动硬依赖。
- `APP_ENV=production` 还强制要求 Keycloak、OpenFGA 和明确的对象存储。
- 表单审批已经是 Temporal-only，没有旧同步流程 fallback。

## 2. 按功能部署

| 服务 | 启用场景 | 关闭时影响 |
| --- | --- | --- |
| Redis | 多副本授权缓存、共享限流 | 限流退回进程内；授权快照缓存关闭 |
| LiteLLM + 上游模型 | Agent Chat、embedding、模型路由管理 | AI/知识库相关功能不可用，其他 API 可运行 |
| NATS JetStream | 异步事件和 OpenFGA durable consumer | 使用数据库 outbox + 直接 OpenFGA writer |
| eHRMS | 组织、岗位、员工、考勤、假期同步 | 仅同步功能不可用 |
| Prometheus | 指标采集 | 不影响业务 API |
| Tempo | Trace 存储 | 不影响业务 API |
| Grafana | 监控展示 | 不影响业务 API |

Keycloak 用户自动开通还需要 Admin client ID/secret；邀请或重置密码需要在 Keycloak 配置 SMTP。Google/Microsoft SSO 需要对应 IdP client ID/secret。

## 3. 后端主要配置

```text
PostgreSQL: DB_HOST DB_PORT DB_USERNAME DB_PASSWORD DB_NAME DB_SSLMODE
Temporal:   TEMPORAL_BASE_URL TEMPORAL_NAMESPACE TEMPORAL_TASK_QUEUE
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

另需生成并托管统一的 `ENCRYPTION_KEY`，用于加密 Agent 模型 API key、MCP/外部工具凭据以及其他持久化密钥。

## 4. 当前稳定镜像

| 服务 | 镜像 |
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

`temporalio/auto-setup` 当前最新可用稳定 tag 仍是 `1.29.7`；Temporal Server 的代码 release 虽更新，但该镜像仓库尚未发布对应新 tag。

## 5. 部署注意事项

- `ops/.env` 是基础设施配置，不能代替后端 `.env`；例如容器使用 `POSTGRES_*`，API 使用 `DB_*`。
- 不要使用 `COMPOSE_PROFILES=all` 直接部署生产，应按实际功能选择 profile。
- 已有环境升级 Keycloak 前先备份数据库，并按官方升级指南完成预发布验证。
- PostgreSQL、Redis、OpenFGA、Temporal、NATS、LiteLLM、SFTPGo 仅允许内网访问。
- Keycloak 对用户入口使用 HTTPS；management port 仅允许运维网访问。
- `/readyz` 检查 PostgreSQL、Temporal，以及已配置的 Keycloak、OpenFGA、Redis、NATS；不会检查 LiteLLM、eHRMS 和对象存储。
- OpenFGA、Temporal、NATS、Redis 当前客户端安全配置面有限，应放在可信私网或 service mesh 内。

## 6. 当前阻塞

`ops/render-configs.sh` 仍要求 `MINIO_*`，但当前 `.env` 和 compose 没有 MinIO，执行会失败：

```text
missing required .env value: MINIO_INTERNAL_HOST
```

启用 Tempo 前需提供 S3-compatible/MinIO 存储，或修改 Tempo 存储配置并删除脚本中的过期强制项。

## 7. 上线验收

- [ ] 执行业务数据库 migration，并确认 `pg_trgm`、`vector` 可用
- [ ] 创建 Keycloak realm/client/token mapper
- [ ] 创建 OpenFGA store，应用 [`../openfga/model.json`](../openfga/model.json)
- [ ] 确认 Temporal namespace 和 task queue 可用
- [ ] 验证 `/healthz`、`/readyz`
- [ ] 完成登录、文件上传、表单审批 smoke
- [ ] 按启用功能验证 LiteLLM、NATS、eHRMS
- [ ] 所有密钥进入 secrets manager，并完成备份/恢复策略

代码依据：[`../../internal/config/config.go`](../../internal/config/config.go)、[`../../cmd/api/bootstrap.go`](../../cmd/api/bootstrap.go)、[`../compose.yaml`](../compose.yaml)、[`../../.env.example`](../../.env.example)。
