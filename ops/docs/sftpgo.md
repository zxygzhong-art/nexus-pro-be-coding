# SFTPGo Object Store Deployment

本倉庫默認使用 SFTPGo 的 HTTP/HTTPS REST API 作爲後端業務文件存儲。所有可調項統一在 [../.env](../.env)。不要直接改 `generated/*.yaml` / `generated/sftpgo/*`，那些文件由 [../render-configs.sh](../render-configs.sh) 生成。

本地部署使用完整 `sftpgo serve`（不是 `portable`），以便啓用 `/api/v2/user/token` 等 REST API。啓動時通過 `generated/sftpgo/loaddata.json` 注入 `SFTPGO_USERNAME` / `SFTPGO_PASSWORD` 對應的業務用戶。

## Config

| 變量 | 用途 |
| --- | --- |
| `OBJECT_STORE_PROVIDER=sftpgo` | 啓用 SFTPGo 對象存儲 |
| `SFTPGO_BASE_URL` | HTTP/HTTPS endpoint；本地可用 `http://`，production 用 `https://` |
| `SFTPGO_ROOT_BUCKET` | 遠端根目錄，默認 `nexus-bucket` |
| `SFTPGO_USERNAME` / `SFTPGO_PASSWORD` | 後端連接賬號（由 loaddata 自動創建） |
| `OBJECT_STORE_CREATE_BUCKET` | 啓動時是否自動創建根目錄 |

## Start SFTPGo

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be/ops
./render-configs.sh
COMPOSE_PROFILES=sftpgo docker compose --env-file .env up -d sftpgo
```

SFTPGo 默認入口：

```text
HTTP: http://127.0.0.1:28080
User: nexus-service
Pass: nexus-service
```

這些賬號只適合本地開發。生產環境必須在 [../.env](../.env) 中換成強密碼，並限制網絡入口。

## Backend Environment

後端不會自動加載 `.env`。本地啓動 API 前需要手動 source `ops/.env`：

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be
set -a
source ops/.env
set +a
go run ./cmd/api
```

宿主機直接運行後端時使用：

```bash
OBJECT_STORE_PROVIDER=sftpgo
OBJECT_STORE_CREATE_BUCKET=true
SFTPGO_BASE_URL=http://127.0.0.1:28080
SFTPGO_ROOT_BUCKET=nexus-bucket
SFTPGO_USERNAME=nexus-service
SFTPGO_PASSWORD=nexus-service
```

如果後端也跑在同一個 Docker network：

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

成功時應返回帶 `access_token` 的 JSON，而不是 `{"message":"Not Found"}`。

啓動後端時，日誌裏應看到 SFTPGo object store 已啓用，root 爲 `SFTPGO_ROOT_BUCKET`。

## Production Notes

- 使用強密碼。
- `SFTPGO_BASE_URL` 使用 `https://`。
- 給 SFTPGo 數據卷做持久化、備份和恢復演練（`sftpgo-data` 存文件，`sftpgo-home` 存 SQLite 元數據）。
- 按租戶隔離要求評估是否需要獨立賬號或獨立根目錄。
