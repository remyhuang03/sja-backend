# SJA Plus Go 后端

## 架构

后端采用 Go 标准库 `net/http` 和 pgx PostgreSQL 连接池。运行时无需 Node.js、Python、MySQL 或外部 SVG 工具。

| 目录 | 职责 |
| --- | --- |
| `internal/analyzer` | 安全读取作品、积木统计、SVG 报告、结构相似度 |
| `internal/httpapi` | HTTP、上传限制、参数校验、管理员鉴权 |
| `internal/store` | PostgreSQL 查询、审核事务和嵌入式版本迁移 |

分析逻辑不共享请求状态。迭代遍历避免循环引用导致栈溢出；ZIP 仅读取根目录的 `project.json`，不解压素材。每个文件限制 48 MiB，解压后的 JSON 限制 64 MiB，积木限制 200,000 个。分析、对比和图片解码共用两个处理名额，满载返回 429。图片按内容验证并转为 PNG，限制像素数。

申请和展示使用同一数据库。审核先锁定申请行，再在一个事务中更新状态和插入展示项，唯一约束防止重复发布。审核接口需要 `Authorization: Bearer <ADMIN_TOKEN>`；不会把数据库错误或磁盘路径返回给客户端。

## 运行

推荐使用相邻 `sja-v3` 仓库内的 `compose.yaml`，它统一启动 PostgreSQL、后端和前端。

独立开发需要 Go 1.27 和 PostgreSQL 18：

```sh
export DATABASE_URL='postgresql://sja:password@localhost:5432/sja?sslmode=disable'
export ADMIN_TOKEN='至少32字符的随机密钥'
go run .
```

支持 `BACKEND_PORT`（默认 `8080`）、`DATA_DIR`（默认 `./var`）。数据库连接必填；审核密钥未配置时，审核接口返回 503。首次启动自动执行迁移。容器使用非 root 用户，数据写入 `/data` 卷。原作品请求结束后删除临时文件，报告保留 30 天，申请图片持久保存。

## API

| 方法 | 路径 | 请求与响应 |
| --- | --- | --- |
| GET | `/api/healthz` | 进程存活 |
| GET | `/api/readyz` | PostgreSQL 连接就绪 |
| POST | `/api/v2/analyze` | multipart：`file`、`is_sort`、`is_high_rank_cate`；返回 `token` 报告 URL 和 `report` |
| GET | `/api/report-img?stamp=...` | SVG 报告 |
| POST | `/api/v2/compare` | multipart：`original`、`compared`；返回 `data` 相似度与两个报告 |
| POST | `/api/v2/project-display-apply` | multipart：`meta` JSON、`cover`、`avatar`；返回 201 和申请 ID |
| GET | `/api/v2/projects-display?n=5` | 最新审核通过的作品，`n` 为 1–100 |
| GET | `/api/v2/project-display-review?limit=20&offset=0` | 管理员申请列表，按时间倒序分页 |
| POST | `/api/v2/project-display-review` | 管理员 JSON：`id`、`status`、`notes` |
| GET | `/api/media/{id}/{file}` | 申请图片，文件名为 `cover.png` 或 `avatar.png` |

分析排序支持 `desc`、`asc`、`none`；分类支持 `top12`、`classic`。兼容旧布尔值 `0`、`1`。统计中的有效脚本由积木目录中的事件帽和自制积木定义识别；不从有效脚本可达的实体积木仍计入总数。

`meta` 格式：

```json
{
  "project_name": "作品名称",
  "author_name": "作者",
  "author_link": "https://scratch.mit.edu/users/example",
  "brief": "不超过20字的简介",
  "links": [{"platform": "scratch", "url": "https://scratch.mit.edu/projects/123", "is_default": true}]
}
```

封面最大 5 MiB，头像最大 2 MiB；接受 JPEG、PNG、WebP，最长边不超过 4096 像素，像素总数不超过 12,000,000。作品链接为 1–10 个，必须恰有一个默认链接。审核状态只能由 `pending` 变为 `approved` 或 `rejected`，重复审核返回 409，拒绝时须填写备注。

相似度使用积木 opcode 和相邻 opcode 的多重集合 Dice 系数，忽略 ID、坐标、输入常量、素材和角色名。两个分量等权；两侧都没有连接时仅使用 opcode 分量。这是结构筛查工具，不能独立判断抄袭，也不等同于语义相似度。

## 测试与发布

```sh
go vet ./...
go test -race ./...
TEST_DATABASE_URL='postgresql://sja:password@localhost:5432/sja_test?sslmode=disable' go test -race ./...
```

未设置 `TEST_DATABASE_URL` 时跳过数据库集成测试。集成测试创建并清理专用 schema，验证重复迁移、并发审核和展示一致性。请使用独立测试数据库。GitHub Actions 自动提供 PostgreSQL 服务，在 `main` 检查通过后发布后端镜像。

前端和后端通过同域 `/api` 通信，无需跨域白名单。部署详情见 [前端部署文档](https://github.com/remyhuang03/sja-v3/blob/main/deploy/README.md)。
