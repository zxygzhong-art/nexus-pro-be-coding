# S3 / MinIO Deployment

本仓库默认使用 MinIO 作为 S3-compatible object store。AWS S3 或其他云厂商对象存储只作为生产替换选项；本地开发、联调、Agent Run 和这套观测部署默认都走 MinIO。

所有可调项统一在 [../.env](../.env)。不要直接改 `generated/*.yaml`，那些文件由 [../render-configs.sh](../render-configs.sh) 生成。

## Default Buckets

| Bucket | 用途 |
| --- | --- |
| `OBJECT_STORE_BUCKET` | 后端业务文件、导入文件等对象存储，默认 `nexus-hr-imports` |
| `LOKI_BUCKET` | Loki 日志块与索引对象存储，默认 `loki` |
| `TEMPO_BUCKET` | Tempo trace block 对象存储，默认 `tempo` |

## Start MinIO

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be/ops
./render-configs.sh
COMPOSE_PROFILES=minio docker compose --env-file .env up -d minio minio-init
```

完整观测栈也会默认启动 MinIO，并自动创建三个 bucket：

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be/ops
./render-configs.sh
docker compose --env-file .env up -d
```

MinIO 默认入口：

```text
S3 API:  http://127.0.0.1:24900
Console: http://127.0.0.1:24901
Login:   minioadmin / minioadmin
```

这些账号只适合本地开发。生产环境必须在 [../.env](../.env) 中换成强密码、开启 TLS，并限制网络入口。

如果 MinIO 部署在其他主机，调整 [../.env](../.env) 里的同一个 MinIO 区块：

```bash
MINIO_INTERNAL_HOST=<minio-host-or-ip>
OBJECT_STORE_ENDPOINT=http://<minio-host-or-ip>:9000
```

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
OBJECT_STORE_PROVIDER=minio
OBJECT_STORE_ENDPOINT=http://127.0.0.1:24900
OBJECT_STORE_BUCKET=nexus-hr-imports
OBJECT_STORE_REGION=us-east-1
OBJECT_STORE_ACCESS_KEY_ID=minioadmin
OBJECT_STORE_SECRET_ACCESS_KEY=minioadmin
OBJECT_STORE_USE_SSL=false
OBJECT_STORE_CREATE_BUCKET=true
```

如果后端也跑在同一个 Docker network，endpoint 改成容器名：

```bash
OBJECT_STORE_ENDPOINT=http://minio:9000
```

后端会按 `OBJECT_STORE_PROVIDER=minio` 初始化 S3-compatible client。`OBJECT_STORE_ENDPOINT` 带 `http://` 时会使用非 TLS，带 `https://` 时会使用 TLS；没有 scheme 时才会参考 `OBJECT_STORE_USE_SSL`。

## Verify

确认 bucket 已创建：

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be/ops
docker run --rm --network observability \
  -e MC_HOST_local=http://minioadmin:minioadmin@minio:9000 \
  minio/mc:RELEASE.2025-08-13T08-35-41Z \
  ls local
```

启动后端时，日志里应看到 S3-compatible object store 已启用，provider 为 `minio`，bucket 为 `.env` 中的 `OBJECT_STORE_BUCKET`。

## Production Notes

生产默认仍建议保留 `OBJECT_STORE_PROVIDER=minio`，但 MinIO 应部署成有持久化磁盘、备份、TLS、强凭证和访问控制的服务。不要使用 `minioadmin/minioadmin`。

如果必须切到 AWS S3 或云厂商 S3-compatible service，保持同一套后端环境变量，只替换 provider、endpoint、region 和凭证：

```bash
OBJECT_STORE_PROVIDER=s3
OBJECT_STORE_ENDPOINT=https://s3.<region>.amazonaws.com
OBJECT_STORE_BUCKET=<bucket>
OBJECT_STORE_REGION=<region>
OBJECT_STORE_ACCESS_KEY_ID=<access-key>
OBJECT_STORE_SECRET_ACCESS_KEY=<secret-key>
OBJECT_STORE_USE_SSL=true
OBJECT_STORE_CREATE_BUCKET=false
```

当前后端代码使用显式 endpoint 和 access key 初始化对象存储 client；如果要改成 IAM role / workload identity 免密模式，需要另外改代码路径。
