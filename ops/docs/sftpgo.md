# SFTPGo Object Store Deployment

本仓库默认使用 SFTPGo 的 HTTP/HTTPS REST API 作为后端业务文件存储。所有可调项统一在 [../.env](../.env)。不要直接改 `generated/*.yaml` / `generated/sftpgo/*`，那些文件由 [../render-configs.sh](../render-configs.sh) 生成。

本地部署使用完整 `sftpgo serve`（不是 `portable`），以便启用 `/api/v2/user/token` 等 REST API。启动时通过 `generated/sftpgo/loaddata.json` 注入 `SFTPGO_USERNAME` / `SFTPGO_PASSWORD` 对应的业务用户。

## Config

| 变量 | 用途 |
| --- | --- |
| `OBJECT_STORE_PROVIDER=sftpgo` | 启用 SFTPGo 对象存储 |
| `SFTPGO_BASE_URL` | HTTP/HTTPS endpoint；本地可用 `http://`，production 用 `https://` |
| `SFTPGO_ROOT_BUCKET` | 远端根目录，默认 `nexus-bucket` |
| `SFTPGO_USERNAME` / `SFTPGO_PASSWORD` | 后端连接账号（由 loaddata 自动创建） |
| `OBJECT_STORE_CREATE_BUCKET` | 启动时是否自动创建根目录 |

## Start SFTPGo

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be/ops
./render-configs.sh
COMPOSE_PROFILES=sftpgo docker compose --env-file .env up -d sftpgo
```

SFTPGo 默认入口：

```text
HTTP: http://127.0.0.1:28080
User: nexus-service
Pass: nexus-service
```

这些账号只适合本地开发。生产环境必须在 [../.env](../.env) 中换成强密码，并限制网络入口。

## Backend Environment

后端不会自动加载 `.env`。本地启动 API 前需要手动 source `ops/.env`：

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be
set -a
source ops/.env
set +a
go run ./cmd/api
```

宿主机直接运行后端时使用：

```bash
OBJECT_STORE_PROVIDER=sftpgo
OBJECT_STORE_CREATE_BUCKET=true
SFTPGO_BASE_URL=http://127.0.0.1:28080
SFTPGO_ROOT_BUCKET=nexus-bucket
SFTPGO_USERNAME=nexus-service
SFTPGO_PASSWORD=nexus-service
```

如果后端也跑在同一个 Docker network：

```bash
SFTPGO_BASE_URL=http://sftpgo:8080
```

production 示例：

```bash
SFTPGO_BASE_URL=https://sftpgo.example.com
```

## Verify

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be/ops
docker compose --env-file .env exec sftpgo sftpgo ping
curl -u nexus-service:nexus-service http://127.0.0.1:28080/api/v2/user/token
```

成功时应返回带 `access_token` 的 JSON，而不是 `{"message":"Not Found"}`。

启动后端时，日志里应看到 SFTPGo object store 已启用，root 为 `SFTPGO_ROOT_BUCKET`。

## Production Notes

- 使用强密码。
- `SFTPGO_BASE_URL` 使用 `https://`。
- 给 SFTPGo 数据卷做持久化、备份和恢复演练（`sftpgo-data` 存文件，`sftpgo-home` 存 SQLite 元数据）。
- 按租户隔离要求评估是否需要独立账号或独立根目录。
