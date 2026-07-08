# SFTPGo Object Store Deployment

本仓库默认使用 SFTPGo 的 SFTP endpoint 作为后端业务文件存储。所有可调项统一在 [../.env](../.env)。不要直接改 `generated/*.yaml`，那些文件由 [../render-configs.sh](../render-configs.sh) 生成。

## Remote Root

| 变量 | 用途 |
| --- | --- |
| `OBJECT_STORE_BUCKET` | 后端业务文件、导入文件等远端根目录，默认 `nexus-hr-imports` |

`OBJECT_STORE_BUCKET` 这个名字沿用既有后端接口语义；在 SFTPGo 下它不是 bucket，而是远端目录。

## Start SFTPGo

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be/ops
COMPOSE_PROFILES=sftpgo docker compose --env-file .env up -d sftpgo
```

SFTPGo 默认入口：

```text
SFTP: sftp://127.0.0.1:22022
User: nexus
Pass: nexus-sftpgo-password
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

宿主机直接运行后端时使用 `.env` 里的默认值：

```bash
OBJECT_STORE_PROVIDER=sftpgo
OBJECT_STORE_ENDPOINT=sftp://127.0.0.1:22022
OBJECT_STORE_BUCKET=nexus-hr-imports
OBJECT_STORE_REGION=
OBJECT_STORE_ACCESS_KEY_ID=nexus
OBJECT_STORE_SECRET_ACCESS_KEY=nexus-sftpgo-password
OBJECT_STORE_SFTP_HOST_KEY=
OBJECT_STORE_USE_SSL=false
OBJECT_STORE_CREATE_BUCKET=true
```

如果后端也跑在同一个 Docker network，endpoint 改成容器名：

```bash
OBJECT_STORE_ENDPOINT=sftp://sftpgo:2022
```

后端会按 `OBJECT_STORE_PROVIDER=sftpgo` 初始化 SFTP client。`OBJECT_STORE_ACCESS_KEY_ID` 是 SFTPGo username，`OBJECT_STORE_SECRET_ACCESS_KEY` 是 password。`OBJECT_STORE_CREATE_BUCKET=true` 时，后端启动会确保 `OBJECT_STORE_BUCKET` 对应的远端根目录存在。

## Verify

确认 SFTPGo 可连接：

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be/ops
docker compose --env-file .env exec sftpgo sftpgo ping
```

启动后端时，日志里应看到 SFTPGo object store 已启用，root 为 `.env` 中的 `OBJECT_STORE_BUCKET`。

## Production Notes

生产环境建议：

- 使用强密码或改为 SSH key 认证。
- 填写 `OBJECT_STORE_SFTP_HOST_KEY`，避免后端信任任意 host key。
- 给 SFTPGo 数据卷做持久化、备份和恢复演练。
- 按租户隔离要求评估是否需要独立账号或独立根目录。

当前后端适配器使用 username/password 连接 SFTPGo。如果要改成 SSH private key 认证，需要另外扩展配置和代码路径。
