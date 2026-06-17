# Nexus Pro API Tester

一个轻量的本地接口测试页，专门按当前 `nexus-pro-be` 代码和 `docs/openapi.yaml` 整理。

## 代码分析摘要

- HTTP 入口在 `cmd/api/main.go`，默认监听 `:8080`。
- 路由集中在 `internal/api/v1`，公开 API 包括 `/healthz`、`/readyz`、`/openapi.yaml`、`/swagger/index.html` 和 36 个 `/v1` 业务端点。
- 成功 JSON 响应会统一包成 `{ "data": ... }`，错误响应包成 `{ "error": { "code", "message", ... } }`。
- 分页参数统一是 `page`、`page_size`、`sort`，`page_size` 最大 100。
- 高风险路由需要请求头 `X-Approval-Confirmed: true`，例如 IAM 写入、员工导入/导出/删除、Agent run 创建、审计日志读取。
- 本地身份来源取决于后端启动参数：`ALLOW_DEMO_CONTEXT=true` 可以不用身份头；`ALLOW_HEADER_CONTEXT=true` 才会接受 `X-Tenant-ID` / `X-Account-ID`；`ALLOW_UNSIGNED_JWT=true` 才会接受测试页生成的 unsigned JWT。
- 当前没有看到后端 CORS 中间件。建议用本目录的 Node 代理启动测试页，避免浏览器跨端口请求被拦。

## 启动后端

最快的内存仓库 demo 启动方式：

```sh
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be
ALLOW_DEMO_CONTEXT=true SEED_DEMO=true go run ./cmd/api
```

如果要测试 header 上下文：

```sh
ALLOW_HEADER_CONTEXT=true SEED_DEMO=true go run ./cmd/api
```

如果要测试测试页生成的 unsigned JWT：

```sh
ALLOW_UNSIGNED_JWT=true SEED_DEMO=true go run ./cmd/api
```

## 启动测试页

```sh
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be/tools/api-tester
npm start
```

打开：

```text
http://127.0.0.1:5178
```

默认使用 `Local proxy` 模式，请求会先打到测试页自己的 `/__proxy`，再由 Node 转发到 `http://localhost:8080`。如果以后后端加了 CORS，可以切到 `Direct browser fetch`。

## 使用建议

- 先点 `Run Smoke`，检查 `/healthz`、`/readyz`、`/v1/me`、员工列表和权限列表。
- 写接口默认会带示例 JSON；路径参数和查询参数可在页面中直接改。
- 勾选 `X-Approval-Confirmed` 后再测高风险接口。
- 用 `Unsigned JWT` 生成按钮时，后端必须以 `ALLOW_UNSIGNED_JWT=true` 启动。
