# Luminous

高校教务系统通用 API 网关，为**光序 Lumaris**提供多校统一注册与发现接口。

## 技术栈

| 组件 | 选型 |
|------|------|
| 语言 | Go 1.26 |
| Web 框架 | Gin v1.12 |
| 配置 | 环境变量 + `.env` 文件 |
| 数据库 | PostgreSQL (pgx/v5 连接池) |
| 日志 | `log/slog` |

## 目录结构

```
Luminous/
├── cmd/
│   ├── server/main.go              # HTTP 服务入口，优雅关闭
│   ├── migrate/main.go             # JSON → PostgreSQL 数据迁移工具
│   └── server/main.go               # HTTP 服务及 MCP /mcp 入口
├── data/
│   └── schools.json                # 种子数据（4 所西安高校）
├── internal/
│   ├── config/
│   │   └── config.go               # 配置加载（.env + 环境变量）
│   ├── handler/
│   │   ├── school.go               # 公开学校接口（列表 / 详情）
│   │   ├── admin.go                # 管理员 CRUD 接口（需认证）
│   │   ├── app.go                  # App 版本信息代理接口
│   │   └── school_test.go          # 公开接口单元测试
│   ├── httpclient/
│   │   ├── client.go               # MCP 工具用 HTTP 客户端（Bearer token、超时）
│   │   └── client_test.go          # HTTP 客户端单元测试
│   ├── mcp/
│   │   ├── protocol.go             # JSON-RPC 2.0 / MCP 类型定义
│   │   ├── handler.go              # MCP 方法路由（initialize / tools/list / tools/call）
│   │   ├── server.go               # Stdio 传输层
│   │   └── server_test.go          # MCP 协议集成测试
│   ├── middleware/
│   │   ├── auth.go                 # Bearer Token 认证中间件
│   │   ├── cors.go                 # CORS 跨域中间件
│   │   ├── ratelimit.go            # 令牌桶速率限制中间件
│   │   ├── request_id.go           # X-Request-ID 生成 / 转发中间件
│   │   └── bodylimit.go            # 请求体大小限制中间件
│   ├── model/
│   │   ├── school.go               # School 实体、Feature 枚举、请求结构体
│   │   └── releaseInfo.go          # App 版本发布信息结构体
│   ├── repository/
│   │   ├── school_repo.go          # SchoolRepository 接口 + JSON 文件实现
│   │   ├── school_repo_pg.go       # PostgreSQL 实现（pgxpool）
│   │   ├── school_repo_test.go     # JSON 仓库单元测试
│   │   └── school_repo_pg_test.go  # PG 仓库集成测试（build tag: integration）
│   ├── response/
│   │   └── response.go             # 统一 JSON 响应格式
│   ├── router/
│   │   └── router.go               # Gin 路由注册与中间件挂载
│   ├── skill/
│   │   ├── skill.go                # Tool 接口 + ToolDef / InputSchema 定义
│   │   └── registry.go             # 工具注册表（线程安全）
│   ├── skills/
│   │   ├── query_school.go         # query_school 工具（查询学校详情）
│   │   └── query_school_test.go    # query_school 单元测试
│   └── util/
│       └── httpclient.go           # 通用 HTTP 客户端（重试、UA 轮换、SSRF 防护）
├── .claude/
│   └── skills/
│       └── query_school/
│           └── SKILL.md            # Claude Code Skill 说明文档
├── examples/
│   └── mcp-config.example.json     # MCP Client 配置示例
├── .github/workflows/              # CI：测试 + 构建推送镜像（不部署）
├── deploy/                         # 服务器部署：compose 定义 + build_from_ghcr.sh
├── .env.example                    # 环境变量示例
├── .gitignore
├── .dockerignore
├── Dockerfile                      # 多阶段构建，非 root 用户
├── Makefile                        # run / build / clean / test
├── go.mod
├── go.sum
└── LICENSE                         # GPLv3
```

## 各模块说明

### 入口层 (`cmd/`)

| 文件 | 说明 |
|------|------|
| `cmd/server/main.go` | 服务主入口。加载配置 → 设置 Gin 模式 → 连接 PostgreSQL → 创建三层 handler → 注册路由 → 启动 HTTP（可选 TLS）→ 监听 SIGINT/SIGTERM 优雅关闭（10s 超时）。关闭时停止限流清理协程。 |
| `cmd/migrate/main.go` | 一次性数据迁移工具。读取种子 JSON，逐条 INSERT 到 PostgreSQL（`ON CONFLICT DO NOTHING`）。默认路径 `./data/schools.json`，可通过命令行参数覆盖。 |
| `cmd/server/main.go` | HTTP 服务入口，同时挂载 Streamable HTTP MCP `/mcp`。 |

### 配置层 (`internal/config/`)

| 文件 | 说明 |
|------|------|
| `config/config.go` | 定义 `AppConfig` 全局配置结构体（含 `ServerConfig`、`AuthConfig`、`DatabaseConfig`、`ReleaseConfig`、`RateLimitConfig`）。`LoadConfig()` 先加载 `.env` 文件（不覆盖已设环境变量），再从环境变量读取，最后校验必填项（`AdminToken`）。 |

### 模型层 (`internal/model/`)

| 文件 | 说明 |
|------|------|
| `model/school.go` | 核心数据模型。`School` 结构体（code、name、website、edu_system_url、features、enabled、时间戳）；`Feature` 字符串枚举（12 种教务功能）及 `IsValidFeature()`；`CreateSchoolRequest`（必填）和 `UpdateSchoolRequest`（指针字段部分更新）；`IsValidSchoolCode()` 正则校验；`IsValidURL()` 校验（拒绝私网地址，用于会被服务端代理抓取的 `website`）；`IsValidEduSystemURL()` 校验（允许空串与私网地址，用于仅由客户端打开的 `edu_system_url`）。 |
| `model/releaseInfo.go` | App 版本信息结构体：`ReleaseInfo`、`AuthorInfo`、`AssetInfo`、`RawApiResponse`。映射上游 App 更新 API 的 JSON。 |

### 仓库层 (`internal/repository/`)

| 文件 | 说明 |
|------|------|
| `repository/school_repo.go` | 定义 `SchoolRepository` 接口（7 个方法：`FindAll` 含分页、`Count`、`FindEnabled`、`FindByCode`、`Create`、`Update`、`Delete`）。`ErrNotFound` 哨兵错误。`JSONSchoolRepository` 实现：内存 map + 磁盘 JSON 持久化，`sync.RWMutex` 并发安全。适合测试和开发。 |
| `repository/school_repo_pg.go` | 生产级 PostgreSQL 实现。DSN 必填，`pgxpool` 连接池，自动建表 + 列补全（`ALTER TABLE ADD COLUMN IF NOT EXISTS`）。`FindAll` 支持 `LIMIT/OFFSET` 分页，所有 CRUD 参数化查询。 |
| `repository/school_repo_test.go` | JSON 仓库完整单测覆盖：CRUD、去重、持久化、`Count`、分页、`ErrNotFound` 校验。 |
| `repository/school_repo_pg_test.go` | PG 仓库集成测试（`//go:build integration`）。自动建表 / 删表，DB 不可用时 `t.Skip`。 |

### 处理器层 (`internal/handler/`)

| 文件 | 说明 |
|------|------|
| `handler/school.go` | `SchoolHandler` — 公开查询。`ListSchools` 返回启用学校；`GetSchool` 按 code 查询，区分 `ErrNotFound`（404）与 DB 错误（500），禁用学校同样返回 404。 |
| `handler/admin.go` | `AdminHandler` — 管理 CRUD。`AdminListSchools` 支持 `?page=&page_size=` 分页（SQL 层 `LIMIT/OFFSET`），通过 `Count()` 返回真实 total。`CreateSchool` 强制 `Content-Type: application/json`（否则 415），校验 Feature、Code、URL 后入库（重复返回 409）。`UpdateSchool` 指针字段部分更新。`DeleteSchool` 按 code 删除。 |
| `handler/app.go` | `AppHandler` — App 版本代理。SSRF 防护（hostname 白名单），响应体 1MB 限制，统一 `response.SuccessList` 格式。 |
| `handler/school_service.go` | `SchoolServiceHandler` 接口：`Code() + RegisterRoutes(*gin.RouterGroup)`。为新学校类型接入预留的扩展点。 |

### 中间件层 (`internal/middleware/`)

| 文件 | 说明 |
|------|------|
| `middleware/auth.go` | Bearer Token 鉴权。`crypto/subtle.ConstantTimeCompare` 防时序攻击。Token 未配置返回 503，无效返回 401。 |
| `middleware/cors.go` | CORS 跨域。`Access-Control-Allow-Origin` 可配置，默认 `*`。OPTIONS 预检返回 204，缓存 24h。 |
| `middleware/ratelimit.go` | 令牌桶速率限制。每 IP 独立计数，速率和突发可配置（默认 10 req/s / 30）。后台协程清理过期记录，支持 `StopRateLimiter()` 优雅停止。设置 `X-RateLimit-Limit`、`X-RateLimit-Remaining`、`Retry-After` 响应头。 |
| `middleware/request_id.go` | 请求链路追踪。优先使用 `X-Request-ID` 请求头（防注入清理：去非打印字符、限 64 字符），无则 `crypto/rand` 生成 16 位 hex。 |
| `middleware/bodylimit.go` | 请求体大小限制。Admin 路由组使用 1MB 限制，防止大 JSON 攻击。 |

### MCP Server 层

| 文件 | 说明 |
|------|------|
| `internal/mcpserver/server.go` | 基于官方 Go SDK 的 Streamable HTTP MCP Server。挂载 `/mcp`，提供学校查询、课程、成绩、考试、校车和电费工具；教务请求按学校 `website` 动态代理。 |
| `skill/skill.go` | `Tool` 接口定义（`Definition()` + `Execute()`）及 MCP 工具元数据类型：`ToolDef`、`InputSchema`、`PropertyDef`。 |
| `skill/registry.go` | 线程安全的工具注册表。`Register()` 防重复、`Get()` 按名查找、`List()` 返回全部工具定义。 |
| `httpclient/client.go` | 轻量 HTTP 客户端。支持可选 Bearer Token、可配置超时、context 控制。错误信息仅包含 HTTP 状态码，不泄露响应体。 |
| `skills/query_school.go` | `query_school` 工具实现。调用 `GET /api/v1/schools/{school_code}`，返回学校名称、网站、教务功能列表等结构化 JSON。参数校验、JSON 合法性检查。 |

### 辅助层

| 文件 | 说明 |
|------|------|
| `response/response.go` | 统一 JSON 响应。`Success()`、`Error()`、`SuccessList()` 三个辅助函数。 |
| `router/router.go` | 路由注册。中间件链：Logger → Recovery → RequestID → CORS → RateLimit。`NoRoute` 返回统一 404 格式。`/healthz` 健康检查。公开路由（`/api/v1/schools`、`/api/v1/app`）。Admin 路由组额外应用 BodyLimit + Auth。支持反向代理 CIDR 配置。 |
| `util/httpclient.go` | HTTP 客户端。30s 超时，最多 3 次重试（超时 / DeadlineExceeded / 5xx，指数退避）。随机 User-Agent 轮换。重试时通过 `GetBody` 重置请求体。 |

## 配置参考

所有配置通过 `LUMINOUS_` 前缀环境变量注入。支持 `.env` 文件（不覆盖已有环境变量）。

### 服务配置

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `LUMINOUS_SERVER_PORT` | `8080` | HTTP 监听端口 |
| `LUMINOUS_SERVER_MODE` | `release` | Gin 运行模式（debug / release / test） |
| `LUMINOUS_SERVER_CORS_ORIGIN` | `""` | CORS 允许来源，空则默认 `*` |
| `LUMINOUS_SERVER_TLS_CERT` | `""` | TLS 证书路径（与 TLS_KEY 同时设置启用 HTTPS） |
| `LUMINOUS_SERVER_TLS_KEY` | `""` | TLS 私钥路径 |
| `LUMINOUS_SERVER_TRUSTED_PROXIES` | `""` | 反向代理 CIDR，逗号分隔（如 `10.0.0.0/8`）。未设置时 `ClientIP` 取直连 IP |

### 认证配置

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `LUMINOUS_AUTH_ADMIN_TOKEN` | 无（**必填**） | 管理员 Bearer Token |

### 数据库配置

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `LUMINOUS_DATABASE_DSN` | `""` | PostgreSQL 连接串 |
| `LUMINOUS_DATABASE_POOL_MAX_CONNS` | `20` | 连接池最大连接数 |
| `LUMINOUS_DATABASE_POOL_MIN_CONNS` | `5` | 连接池最小连接数 |

### 发布信息配置

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `LUMINOUS_RELEASE_API_URL` | `""` | 上游 App 信息 API 完整 URL（设置后覆盖以下两项） |
| `LUMINOUS_RELEASE_APP_UUID` | `5f278ffc-...` | App UUID |
| `LUMINOUS_RELEASE_CHANNEL_ID` | `9e1a198a-...` | 渠道 ID |

### 速率限制配置

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `LUMINOUS_RATE_LIMIT_RATE` | `10` | 令牌桶填充速率（令牌/秒） |
| `LUMINOUS_RATE_LIMIT_BURST` | `30` | 令牌桶最大容量（突发请求数） |

## 快速开始

### 前置条件

- Go 1.26+
- PostgreSQL（本地或远程）
- `make`（可选，也可直接用 `go` 命令）

### 1. 准备数据库

```bash
createdb luminous
# 或: psql -c "CREATE DATABASE luminous;"
```

### 2. 配置环境

复制示例文件并编辑：

```bash
cp .env.example .env
```

编辑 `.env`，填入最小配置：

```env
LUMINOUS_SERVER_MODE=debug
LUMINOUS_AUTH_ADMIN_TOKEN=my-dev-token
LUMINOUS_DATABASE_DSN=postgresql://postgres:postgres@localhost:5432/luminous?sslmode=disable
```

`.env` 已在 `.gitignore` 中忽略，不会提交到仓库。

### 3. 启动服务

```bash
go run ./cmd/server/
```

输出：

```
INFO Starting Luminous server
INFO Server listening addr=:8080
WARN TLS not configured — use a reverse proxy for production
```

### 4. 导入种子数据

```bash
go run ./cmd/migrate/ ./data/schools.json
# → migrated=4 failed=0
```

### 5. 验证

```bash
# 健康检查
curl http://localhost:8080/healthz
# → {"status":"ok"}

# 查询学校列表
curl http://localhost:8080/api/v1/schools

# 查询单个学校
curl http://localhost:8080/api/v1/schools/XAUAT

# 管理员接口
curl http://localhost:8080/api/v1/admin/schools \
  -H "Authorization: Bearer my-dev-token"
```

### 6. 运行测试

```bash
# 单元测试（无需数据库）
go test ./... -short

# 集成测试（需要 .env 中配置 DSN）
go test -tags=integration ./internal/repository/

# 代码检查
go vet ./...
```

## 架构

```
请求 → Logger → Recovery → RequestID → CORS → RateLimit
                                                     │
                                   ┌─────────────────┼──────────────────┐
                                   ▼                  ▼                  ▼
                             /api/v1/*        /api/v1/admin/*        /healthz
                             (公开路由)         (BodyLimit+Auth)
                                   │                  │
                                   ▼                  ▼
                             SchoolHandler      AdminHandler
                             AppHandler
                                   │                  │
                                   └────────┬─────────┘
                                            ▼
                                   SchoolRepository (interface)
                                   ┌─────────┴──────────┐
                                   ▼                    ▼
                         JSONSchoolRepository    PGSchoolRepository
                             (内存+文件)          (pgxpool → PostgreSQL)
```

## API 文档

**Base URL:** `http://localhost:8080`
**Content-Type:** `application/json`

### 通用响应格式

```json
// 单条数据
{ "code": 200, "message": "success", "data": {...} }

// 错误
{ "code": 404, "message": "school not found", "data": null }

// 列表
{ "code": 200, "message": "success", "data": { "total": 3, "items": [...] } }
```

### 公开接口

#### `GET /healthz`

健康检查。

#### `GET /api/v1/schools`

返回所有已启用学校列表。

#### `GET /api/v1/schools/:code`

返回指定学校详情及支持的功能。

| 状态码 | 说明 |
|--------|------|
| 200 | 成功 |
| 400 | 无效的 school code |
| 404 | 学校不存在或未启用 |
| 500 | 服务器内部错误 |

#### `GET /api/v1/app`

获取 App 版本更新信息（代理上游 API）。

### 管理员接口

挂载在 `/api/v1/admin/` 下，需 Bearer Token 鉴权，请求体限制 1MB。

```
Authorization: Bearer <admin_token>
```

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/admin/schools` | 列出所有学校（含未启用），支持 `?page=1&page_size=50` |
| POST | `/api/v1/admin/schools` | 新增学校（需 `Content-Type: application/json`） |
| PUT | `/api/v1/admin/schools/:code` | 部分更新学校（需 `Content-Type: application/json`） |
| DELETE | `/api/v1/admin/schools/:code` | 删除学校 |

**状态码：** 415 — Content-Type 非 `application/json`；409 — 重复 code；422 — 无效字段值。

**分页：** `page` 默认 1，`page_size` 默认 50，最大 200。无效值自动回退默认值并记录警告。

**新增学校：**

```bash
curl -X POST http://localhost:8080/api/v1/admin/schools \
  -H "Authorization: Bearer my-dev-token" \
  -H "Content-Type: application/json" \
  -d '{
    "code": "XAUAT",
    "name": "西安建筑科技大学",
    "website": "https://xauatapi.xauat.site",
    "edu_system_url": "https://jwc.xauat.edu.cn",
    "features": ["login", "timetable", "grade_query", "exam_schedule"]
  }'
```

**部分更新：**

```bash
curl -X PUT http://localhost:8080/api/v1/admin/schools/XAUAT \
  -H "Authorization: Bearer my-dev-token" \
  -H "Content-Type: application/json" \
  -d '{"enabled": false}'
```

**字段说明：** `website` 是学校后端 API 地址（会被服务端代理抓取，故不允许私网地址）；`edu_system_url` 是学生登录教务系统的地址，用于客户端指引与 HTML 课表导入，**可选**，允许为空也允许校园网内网地址，为空时客户端回退到 `website`。


### 功能枚举 (Features)

| 值 | 说明 |
|----|------|
| `login` | 登录 |
| `timetable` | 日历功能 |
| `grade_query` | 成绩查询 |
| `gpa_calculation` | GPA 计算 |
| `exam_schedule` | 考试安排 |
| `course_schedule` | 课程显示 |
| `bus_schedule` | 校车时刻表 |
| `program` | 培养方案 |
| `study_progress` | 学业进度 |
| `electricity` | 电费查询 |
| `payment` | 校园卡查询 |
| `map` | 校园地图 |

## 数据迁移

将 `data/schools.json` 的种子数据导入 PostgreSQL：

```bash
go run ./cmd/migrate/ ./data/schools.json
# 输出: migrated=4 failed=0
```

重复运行安全（`ON CONFLICT DO NOTHING` 跳过已存在记录）。

## Docker

```bash
docker build -t luminous .
docker run -d \
  -p 8080:8080 \
  --env-file .env \
  luminous
```

或逐个设置环境变量：

```bash
docker run -d \
  -p 8080:8080 \
  -e LUMINOUS_DATABASE_DSN="postgresql://..." \
  -e LUMINOUS_AUTH_ADMIN_TOKEN="your-token" \
  luminous
```

镜像特点：多阶段构建、静态二进制、非 root 用户（65534）、含 `HEALTHCHECK`。

### 服务器部署（从 ghcr 拉取）

CI（`.github/workflows/build-image.yml`）在推送到 `master` 时跑 `go vet` 与 `go test`，
再把镜像推送到 `ghcr.io/lumaristeam/luminous`。**它不部署**——服务器上的发布手动执行：

```bash
cd deploy
./build_from_ghcr.sh                                          # 拉 :latest
./build_from_ghcr.sh ghcr.io/lumaristeam/luminous:<sha>       # 指定版本，也是回滚方式
APP_PORT=8080 ./build_from_ghcr.sh                            # 改宿主机端口（默认 23467）
```

同目录需要 `docker-compose.production.yml`（或 `docker-compose.yml`）和 `.env`。
`LUMINOUS_DATABASE_DSN` 是必需项——缺失或库连不上时进程直接退出，不会降级启动。

镜像在 ghcr 上是私有的，服务器拉取前需要一张只读凭据：
在 GitHub → `Settings` → `Developer settings` → `Personal access tokens` → **Tokens (classic)**
生成，**只勾 `read:packages`**（GitHub Packages 不支持 fine-grained token），然后
`GHCR_PULL_TOKEN=<token> GHCR_USERNAME=<用户名> ./build_from_ghcr.sh`；脚本用完即登出。

> 本服务刻意不加入 `xauat-net`：没有别的容器需要按名字解析它，接进共享网络只会给部署
> 多一个前置条件。它是唯一由外部直接访问的服务，因此映射宿主机端口。

## 常用命令

```bash
# 开发运行
go run ./cmd/server/

# 编译
make build          # → bin/luminous

# 运行全部测试
make test

# 运行单元测试（跳过 PG 集成测试）
go test -short ./...

# 运行含 PG 集成测试
go test -tags=integration ./internal/repository/

# 代码检查
go vet ./...
go fmt ./...
```

---

## MCP Server

本项目通过 `GET/POST /mcp` 提供 MCP Streamable HTTP 服务，允许 Claude Code、Claude Desktop 等客户端调用学校发现和教务工具。

设置 `LUMINOUS_MCP_TOKEN` 后，客户端必须发送对应的 `Authorization: Bearer ...`；不设置时端点不要求 Token。工具参数中的 `school_code` 决定要调用的学校，认证类工具另外接收教务系统 Cookie。

### 客户端配置

服务启动后，将 `http://localhost:8080/mcp` 填入 MCP 客户端配置；示例见 `examples/mcp-config.example.json`。

### 快速开始

```bash
# 设置环境变量
export API_BASE_URL=http://localhost:8080
export API_TOKEN=""               # 可选
export HTTP_TIMEOUT_SECONDS=10
export LOG_LEVEL=info

# 启动 MCP Server
go run ./cmd/luminous-mcp-server
```

启动后服务器在 stdio 上等待 JSON-RPC 请求（由 MCP Client 自动管理）。

### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `API_BASE_URL` | `http://localhost:8080` | 后端 API 基础 URL |
| `API_TOKEN` | `""` | 可选，Bearer Token。设置后所有请求携带 `Authorization: Bearer ${API_TOKEN}` |
| `HTTP_TIMEOUT_SECONDS` | `10` | HTTP 请求超时秒数 |
| `LOG_LEVEL` | `info` | 日志级别：`debug`、`info`、`warn`、`error` |

日志统一输出到 **stderr**，MCP 协议消息输出到 **stdout**，两者互不干扰。

### 接入 Claude Code

1. 在项目根目录创建或编辑 `.claude/mcp.json`（或全局 `~/.claude/mcp.json`）：

```json
{
  "mcpServers": {
    "luminous-mcp-server": {
      "command": "go",
      "args": ["run", "./cmd/luminous-mcp-server"],
      "cwd": "/path/to/Luminous",
      "env": {
        "API_BASE_URL": "http://localhost:8080",
        "API_TOKEN": "",
        "HTTP_TIMEOUT_SECONDS": "10",
        "LOG_LEVEL": "info"
      }
    }
  }
}
```

2. 重新启动 Claude Code，工具 `query_school` 将自动可用。

3. 参考配置示例：`examples/mcp-config.example.json`

### 接入 Claude Desktop

在 Claude Desktop 的配置文件中添加 MCP Server 配置：

```json
{
  "mcpServers": {
    "luminous-mcp-server": {
      "command": "go",
      "args": ["run", "./cmd/luminous-mcp-server"],
      "cwd": "/absolute/path/to/Luminous",
      "env": {
        "API_BASE_URL": "http://localhost:8080"
      }
    }
  }
}
```

### 接入其他 MCP Client

任何支持 MCP stdio 传输的客户端都可以接入。仅需配置 `command` 和 `args` 即可。

### 已注册的工具

#### query_school

根据学校代码查询学校详情（名称、网站、教务功能列表），对应 Luminous API `GET /api/v1/schools/:code`。

**输入：**
```json
{ "school_code": "XAUAT" }
```

**输出：**
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "code": "XAUAT",
    "name": "西安建筑科技大学",
    "website": "https://xauatapi.xauat.site",
    "edu_system_url": "https://jwc.xauat.edu.cn",
    "features": ["login", "timetable", "grade_query"],
    "enabled": true
  }
}
```

**详细说明：** 参见 `.claude/skills/query_school/SKILL.md`

### tools/list 示例

```bash
# 模拟 JSON-RPC tools/list 请求
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' | go run ./cmd/luminous-mcp-server
```

响应：
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "tools": [
      {
        "name": "query_school",
        "description": "根据学校代码查询学校详情。调用时机：当用户询问某所学校的基本信息、网站地址、支持的功能列表时使用...",
        "inputSchema": {
          "type": "object",
          "properties": {
            "school_code": { "type": "string", "description": "学校代码，例如 XAUAT" }
          },
          "required": ["school_code"]
        }
      }
    ]
  }
}
```

### tools/call 示例

```bash
echo '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"query_school","arguments":{"school_code":"XAUAT"}}}' | go run ./cmd/luminous-mcp-server
```

### 如何新增一个 Tool

1. 在 `internal/skills/` 下创建新文件（如 `query_user.go`）
2. 实现 `skill.Tool` 接口：

```go
type MyTool struct {
    client *httpclient.Client
}

func (t *MyTool) Definition() skill.ToolDef {
    return skill.ToolDef{
        Name:        "my_tool",
        Description: "工具描述",
        InputSchema: skill.InputSchema{
            Type: "object",
            Properties: map[string]skill.PropertyDef{
                "param1": {Type: "string", Description: "参数说明"},
            },
            Required: []string{"param1"},
        },
    }
}

func (t *MyTool) Execute(ctx context.Context, args map[string]any) (string, error) {
    // 1. 从 args 中提取参数
    // 2. 调用 c.client.Get(ctx, "path") 或自定义 HTTP 逻辑
    // 3. 返回 JSON 字符串
    return `{"result":"ok"}`, nil
}
```

3. 在 `cmd/luminous-mcp-server/main.go` 中注册：

```go
registry.Register(skills.NewMyTool(httpClient))
```

4. 如果需要 Claude Code Skill 说明，在 `.claude/skills/<tool_name>/SKILL.md` 中添加文档。

### 如何把已有 HTTP API 包装成 MCP Tool

1. 确定 API 的路径、参数和返回值
2. 在 `internal/skills/` 下创建 Tool 实现
3. 在 `Execute` 方法中调用 `httpclient.Client.Get(ctx, path)`
4. 注册到 Registry
5. 不需要修改 MCP Server 核心代码

### 测试

```bash
# 运行所有测试（包括 MCP 相关）
go test ./internal/...

# 仅运行 MCP Server 测试
go test ./internal/mcp/ -v

# 仅运行 HTTP Client 测试
go test ./internal/httpclient/ -v

# 仅运行 query_school 工具测试
go test ./internal/skills/ -v
```

### 常见问题

**Q: 启动后看不到任何输出？**

A: 正常。MCP Server 在 stdio 模式下等待 JSON-RPC 请求，日志输出到 stderr。只有收到 MCP Client 发送的请求时才会有响应。

**Q: Claude Code 中看不到工具？**

A: 检查：
1. `mcp.json` 中 `cwd` 路径是否正确
2. `go run ./cmd/luminous-mcp-server` 是否能单独启动（无错误退出）
3. 环境变量是否正确配置
4. Claude Code 版本是否支持 MCP

**Q: 请求后端返回 404？**

A: 确认 `API_BASE_URL` 配置正确，且后端服务正在运行。

**Q: 如何调试？**

A: 设置 `LOG_LEVEL=debug`，日志将输出到 stderr。可以重定向 stderr 查看：
```bash
go run ./cmd/luminous-mcp-server 2>debug.log
```

**Q: 如何避免泄露 API_TOKEN？**

A: 
- 不要在日志中打印 token
- 不要在 HTTP 错误信息中包含响应体
- 使用环境变量注入 token，不要硬编码
- 代码中已实现上述安全措施
