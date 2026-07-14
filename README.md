# TODO

一个使用 Go、Wails、React 和 SQLite 构建的本地优先 Windows 待办与协作笔记应用。

## 目录结构

- `backend/`：SQLite 数据层、备份恢复、领域模型和 Wails 服务。
- `frontend/`：React + TypeScript 用户界面与 Wails 生成绑定。
- `syncserver/`：PostgreSQL、WebSocket 与 S3 兼容对象存储同步服务。
- `cmd/todo-server/`：自托管服务入口和管理员命令。
- `main.go`：桌面应用入口和窗口配置。
- `tools/`：构建辅助工具。

## 开发

环境要求：Go 1.25+、Node.js 20+、WebView2 和 Wails CLI 2.13.0。

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
wails doctor
wails dev
```

前端也可以单独在浏览器中运行。浏览器模式使用 localStorage 作为预览数据源，桌面应用仍使用 SQLite。

```powershell
cd frontend
npm install
npm run dev
```

## 悬浮小窗

侧栏底部的“悬浮小窗”会把应用切换为置顶的紧凑任务视图。点击小窗标题可以切换收集箱、今天、即将到期、全部任务、已完成、自定义清单和标签。快速新增会自动应用当前视图的日期、清单或标签，退出后会在主窗口继续显示同一视图，并恢复原来的窗口大小、位置和最大化状态。也可以使用 `Ctrl+Shift+M` 在两种窗口模式之间切换。

## 测试

```powershell
go test ./...
cd frontend
npm run test:run
npm run test:e2e
```

## 构建

```powershell
wails build -clean -trimpath -platform windows/amd64
wails build -clean -trimpath -platform windows/amd64 -nsis -installscope user
```

构建产物位于 `build/bin`。

## 自托管同步

复制 `.env.example` 为 `.env`，替换其中所有密码、JWT 密钥和 MinIO KMS 密钥，然后启动服务：

```powershell
docker compose up -d --build
docker compose ps
```

首次启动后创建管理员账号。密码至少 10 个字符：

```powershell
$env:TODO_ADMIN_USERNAME = "admin"
$env:TODO_ADMIN_PASSWORD = "replace-with-a-strong-password"
docker compose run --rm -e TODO_ADMIN_USERNAME -e TODO_ADMIN_PASSWORD server admin create-user
```

服务默认只监听宿主机 `127.0.0.1:8080`，适合本机测试。跨设备部署必须通过 Caddy、Nginx 或其他反向代理提供 HTTPS，再将桌面端连接到 HTTPS 地址。不要直接把 8080、PostgreSQL、MinIO API 或 MinIO 控制台暴露到公网。

`minio-init` 会创建 `todo-attachments` 桶并启用 SSE-S3。生产环境应使用外部 KMS，并将 PostgreSQL 与 MinIO 数据目录放在 BitLocker、LUKS 或云厂商加密卷上。

## 服务端备份

PostgreSQL 和对象存储必须来自同一静止时间点。最简单的维护窗口流程是先停止写入服务，再分别备份数据库和附件：

```powershell
docker compose stop server
New-Item -ItemType Directory -Force server-backup
docker compose exec postgres pg_dump -U todo -d todo -Fc -f /tmp/todo-postgres.dump
docker compose cp postgres:/tmp/todo-postgres.dump ./server-backup/todo-postgres.dump
docker compose run --rm -v ./server-backup/attachments:/backup --entrypoint /bin/sh minio-init -c 'mc alias set local http://minio:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" && mc mirror --overwrite local/todo-attachments /backup'
docker compose start server
```

将 `server-backup` 移到受保护的外部备份目录或把 `mc mirror` 目标改为远端 S3。恢复时保持 `server` 停止，先恢复 PostgreSQL，再恢复同一批次的对象数据，最后启动服务并检查 `/healthz`。恢复前应保留当前数据库和对象存储的完整副本。

当前开发环境没有安装 Docker，因此仓库内可完成服务编译与单元测试，但 Docker Compose 的 PostgreSQL/MinIO 集成冒烟需要在安装 Docker Desktop 后执行。

## Dev 多架构镜像

推送到 `dev` 分支后，GitHub Actions 会先运行 Go、Vitest、Playwright 和前端构建，再通过 Buildx 构建并发布以下服务端镜像：

```text
ghcr.io/mon3tr-v/todo-server:dev
linux/amd64
linux/arm64
```

Compose 默认使用这个 dev 镜像。需要本地重新构建时仍可运行 `docker compose up -d --build`，也可以通过 `TODO_SERVER_IMAGE` 指定其他标签。

## 桌面预发布安装包

推送 `v*` 标签后，GitHub Actions 会在对应平台生成并上传预发布安装包：

- Windows amd64/arm64：便携 EXE 和 NSIS 安装器。
- Linux amd64/arm64：Debian 安装包和便携 tar.gz。
- macOS amd64/arm64：DMG 和 `.app.zip`。
- Release 同时包含 `SHA256SUMS.txt`，用于验证下载文件。

这些 dev 产物没有商业代码签名或 Apple 公证。Windows SmartScreen 和 macOS Gatekeeper 可能显示未知发布者提示。
