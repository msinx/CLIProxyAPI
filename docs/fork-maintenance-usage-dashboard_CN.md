# 个人 Fork 维护说明：SQLite Usage Dashboard

本文用于后续 rebase upstream 或再次发布个人 fork 时快速恢复上下文。核心目标是：在尽量贴近 upstream 的前提下，保留个人 fork 中内置的 SQLite usage 统计、管理端 Usage dashboard，以及配套 release 流程。

## 仓库关系

后端仓库：

- 本地路径：`/Users/m/Projects/CLIProxyAPI/.worktrees/usage-web-sqlite`
- 个人 fork：`git@github.com:msinx/CLIProxyAPI.git`
- 官方 upstream：`https://github.com/router-for-me/CLIProxyAPI.git`
- 当前功能分支：`usage-web-sqlite`
- 个人 fork release tag 使用 4 段版本号，例如 `v6.10.1.1`，用于和官方 3 段版本号区分。

前端管理中心仓库：

- 本地路径：`/Users/m/Projects/Cli-Proxy-API-Management-Center`
- 个人 fork：`git@github.com:msinx/Cli-Proxy-API-Management-Center.git`
- 官方 upstream：`https://github.com/router-for-me/Cli-Proxy-API-Management-Center.git`
- 当前功能分支：`usage-dashboard-sqlite-api`
- 个人 fork release tag 使用 4 段版本号，例如 `v1.8.2.2`。

原则：

- 只向 `origin` 推送个人 fork，不能误推 `upstream`。
- 前端 release 必须先完成，再发布后端。后端默认管理面板资产会从个人前端 fork 的 release 下载 `management.html`。
- 不使用 MySQL 保存 usage 数据。本功能使用 SQLite，默认位置是后端进程工作目录下的 `./data/usage.db`，可通过 `usage-sqlite-path` 修改。

## 功能背景

个人 fork 需要把 usage 统计重新做回 CLIProxyAPI 进程内，而不是依赖外部 CPA Usage Keeper 服务。实现目标：

- 后端在请求完成后把 usage 事件写入 SQLite。
- 管理 API 提供 overview、analysis、events、credentials、filter-options 等查询接口。
- 前端管理中心增加 `/usage` 页面，直接消费后端 management usage API。
- 空数据场景必须稳定：后端返回空数组，前端也要兼容 `null` 或缺失数组字段。
- dashboard 主要用于查看 usage trend、模型/provider/credential breakdown、credential 健康状态和近期事件。

## 后端关键改动边界

尽量把 fork 专属 usage 改动集中在这些位置，方便 rebase 时保留：

- `internal/usagesqlite/`
  - SQLite schema、store、plugin、retention、salt/hash。
  - 这里是 fork 专属 usage 存储的主边界。
- `internal/api/handlers/management/usage.go`
  - 管理端 usage 查询 API。
- `internal/api/handlers/management/handler.go`
  - 注册 usage handler。
- `internal/api/server.go`
  - 路由注册。
- `internal/config/config.go`
  - usage SQLite 相关配置默认值和结构。
- `config.example.yaml`
  - 暴露用户可见配置项。
- `internal/watcher/diff/config_diff.go`
  - 让配置 diff 能识别 usage SQLite 配置。
- `internal/managementasset/updater.go`
  - 默认管理面板 release 仓库指向 `msinx/Cli-Proxy-API-Management-Center`。
  - fallback management asset URL 在个人 fork 中应保持禁用，避免回退到官方前端资产。
- `go.mod` / `go.sum`
  - SQLite driver 依赖。

关键配置：

- `usage-statistics-enabled`：默认 `true`。
- `usage-sqlite-enabled`：默认 `true`。
- `usage-sqlite-path`：默认 `./data/usage.db`。
- `usage-retention-days`：默认 `30`。
- `usage-api-key-salt`：用于稳定脱敏 API key 身份。
- `usage-sqlite-buffer-size`
- `usage-sqlite-batch-size`
- `usage-sqlite-flush-interval`

关键 API：

- `GET /v0/management/usage/overview`
- `GET /v0/management/usage/analysis`
- `GET /v0/management/usage/events`
- `GET /v0/management/usage/credentials`
- `GET /v0/management/usage/filter-options`
- `GET /v0/management/usage-sqlite-enabled`

rebase 时要特别注意：

- upstream 如果重构 management routes，需要重新确认上述 endpoint 是否仍注册在 `/v0/management` 下。
- upstream 如果改动 usage、redisqueue 或请求日志链路，需要确认 SQLite plugin 仍能收到完整 usage event。
- upstream 如果改动 config 默认值加载、watcher diff 或 config basic handler，需要保留 SQLite usage 配置的可见性和热更新行为。
- upstream 如果改动 management asset updater，需要保留个人前端 fork release 源。

## 前端关键改动边界

前端 fork 专属 usage dashboard 主要集中在：

- `src/pages/UsagePage.tsx`
  - Usage dashboard 页面主体。
- `src/pages/UsagePage.module.scss`
  - 页面样式。
- `src/services/api/usage.ts`
  - Management usage API client 和响应归一化。
- `src/services/api/index.ts`
  - 导出 usage API。
- `src/router/MainRoutes.tsx`
  - 注册 `/usage` 路由。
- `src/components/layout/MainLayout.tsx`
  - 侧边栏 Usage 入口。
- `src/i18n/locales/en.json`
- `src/i18n/locales/zh-CN.json`
- `src/i18n/locales/zh-TW.json`
- `src/i18n/locales/ru.json`
  - Usage 页面文案。

rebase 时要特别注意：

- upstream 如果改动路由结构或 layout nav，需要重新接入 `/usage`。
- upstream 如果改动 API client 约定，需要确认 `src/services/api/usage.ts` 的 request helper、错误处理和类型仍匹配。
- upstream 如果改动 i18n key 结构，需要保留 usage 文案但按新结构迁移。
- `dist/` 是构建产物，不应作为源码提交。release 时只上传构建后的 `management.html`。

## Release 顺序

固定顺序：

1. 前端：验证、提交、推送分支。
2. 前端：创建并推送 4 段 tag。
3. 前端：构建 `dist/index.html`，作为 `management.html` 上传到前端 GitHub Release。
4. 后端：验证、提交、推送分支。
5. 后端：创建并推送 4 段 tag。
6. 后端：等待 goreleaser GitHub Actions 自动生成 release 和二进制资产。

前端 release 命令示例：

```bash
npm run type-check
npm run lint
npm run build
git diff --check

git push -u origin usage-dashboard-sqlite-api
git tag -a v1.8.2.2 -m "Release v1.8.2.2"
git push origin v1.8.2.2

cp dist/index.html /private/tmp/management.html
gh release create v1.8.2.2 /private/tmp/management.html \
  --repo msinx/Cli-Proxy-API-Management-Center \
  --title "v1.8.2.2" \
  --notes "SQLite usage dashboard for the management center."
```

如果 release 已存在，覆盖资产：

```bash
gh release upload v1.8.2.2 /private/tmp/management.html \
  --clobber \
  --repo msinx/Cli-Proxy-API-Management-Center
```

后端 release 命令示例：

```bash
go test ./internal/usagesqlite ./internal/config ./internal/managementasset ./internal/api/handlers/management ./internal/api ./internal/watcher/diff ./internal/redisqueue
go build -o test-output ./cmd/server
rm test-output
git diff --check

git push -u origin usage-web-sqlite
git tag -a v6.10.1.1 -m "Release v6.10.1.1"
git push origin v6.10.1.1

gh run list --repo msinx/CLIProxyAPI --workflow goreleaser --limit 5
gh run watch <run-id> --repo msinx/CLIProxyAPI --exit-status
gh release view v6.10.1.1 --repo msinx/CLIProxyAPI
```

后端 `.github/workflows/release.yaml` 会在任意 tag push 时触发 goreleaser。正常情况下不需要手工上传后端二进制。

## Rebase upstream 建议流程

当 upstream 有新版本时，不要直接在个人 fork 上盲目打 tag。先判断官方前后端是否都需要同步，再按“前端资产先可用，后端再指向它”的顺序发布。

### Upstream 出新版本后的处理顺序

推荐顺序：

1. 查看官方后端和前端最新 tag。
2. 分别 fetch 两个仓库的 `upstream` 和 `origin`。
3. 先 rebase 后端功能分支，解决 SQLite usage 后端冲突并验证。
4. 再 rebase 前端功能分支，解决 Usage dashboard 冲突并验证。
5. 先发布前端 4 段 tag 和 `management.html` release asset。
6. 更新或确认后端默认管理面板资产源仍指向前端个人 fork release。
7. 发布后端 4 段 tag，等待 goreleaser 自动生成后端 release。
8. 最后做端到端检查：后端 release 存在、前端 `management.html` 可下载、后端配置默认值仍启用 SQLite usage。

为什么前端要先发：

- 后端内置管理面板更新器会去前端 release 下载 `management.html`。
- 如果后端先发布，而前端对应 tag/release asset 还不存在，用户启动后端时可能拿不到匹配的管理页面。
- 前端 release 是一个单文件资产，失败时更容易重试；后端 release 会触发 goreleaser 多平台构建，放在后面更稳。

### 查询 upstream 最新版本

后端：

```bash
cd /Users/m/Projects/CLIProxyAPI/.worktrees/usage-web-sqlite
git fetch upstream --tags
git fetch origin --tags
git tag -l 'v*' --sort=-version:refname | sed -n '1,20p'
git ls-remote --tags upstream 'v*'
```

前端：

```bash
cd /Users/m/Projects/Cli-Proxy-API-Management-Center
git fetch upstream --tags
git fetch origin --tags
git tag -l 'v*' --sort=-version:refname | sed -n '1,20p'
git ls-remote --tags upstream 'v*'
```

如果本地 tag 列表和远端不一致，以 `git ls-remote --tags upstream 'v*'` 的结果为准。

### 个人 fork 版本号选择

个人 fork 始终使用 4 段 tag，避免和官方 3 段 tag 混淆。

规则：

- 官方后端 `v6.10.2` 的第一次个人 fork 发布：`v6.10.2.1`。
- 同一个官方后端版本下再次修复发布：`v6.10.2.2`。
- 官方前端 `v1.8.3` 的第一次个人 fork 发布：`v1.8.3.1`。
- 同一个官方前端版本下再次修复发布：`v1.8.3.2`。

发布前必须确认本地和远端都没有同名 tag：

```bash
git tag -l v6.10.2.1
git ls-remote --tags origin v6.10.2.1
```

如果 tag 已经存在：

- 如果 release 内容正确，不要重打 tag。
- 如果只是 release asset 错了，优先覆盖 asset，例如前端 `gh release upload --clobber`。
- 如果 tag 指错 commit，先停下来人工确认；不要自动删除远端 tag，删除 tag 属于破坏性操作。

### Rebase 分支

后端：

```bash
git fetch upstream --tags
git fetch origin --tags
git switch usage-web-sqlite
git rebase upstream/main
```

前端：

```bash
git fetch upstream --tags
git fetch origin --tags
git switch usage-dashboard-sqlite-api
git rebase upstream/main
```

如果 upstream 的默认分支不是 `main`，先用下面命令确认：

```bash
git remote show upstream
```

处理冲突时优先级：

1. 保留 upstream 的通用修复和结构调整。
2. 重新套用个人 fork 的 SQLite usage 和 Usage dashboard 改动。
3. 不为了兼容旧实现引入额外分支，除非 upstream 新行为确实需要。
4. 保持改动集中在上面列出的文件边界内。
5. rebase 后必须跑前后端各自的验证命令。

### Rebase 后的后端检查

重点看这些点：

- `internal/usagesqlite/` 是否仍能编译并通过测试。
- management usage endpoints 是否仍挂在 `/v0/management` 下。
- `usage-sqlite-enabled` 等配置是否仍出现在默认配置、example config、management config basic 和 config diff 中。
- request/usage 事件链路是否仍会调用 SQLite plugin。
- `internal/managementasset/updater.go` 是否仍默认指向 `msinx/Cli-Proxy-API-Management-Center`。
- fallback management asset URL 是否仍保持禁用，避免回退到官方前端。

后端 rebase 后先不要发布，先跑：

```bash
go test ./internal/usagesqlite ./internal/config ./internal/managementasset ./internal/api/handlers/management ./internal/api ./internal/watcher/diff ./internal/redisqueue
go build -o test-output ./cmd/server
rm test-output
git diff --check
```

### Rebase 后的前端检查

重点看这些点：

- `/usage` 路由是否存在。
- sidebar/nav 是否仍有 Usage 入口。
- `src/services/api/usage.ts` 是否仍符合当前 API client 写法。
- Usage 页面是否仍消费后端这些接口：
  - `/usage/overview`
  - `/usage/analysis`
  - `/usage/events`
  - `/usage/credentials`
  - `/usage/filter-options`
  - `/usage-sqlite-enabled`
- i18n key 是否和 upstream 最新结构一致。
- 空数据、`null` 数组、缺失数组字段仍不会让页面崩溃。

前端 rebase 后先跑：

```bash
npm run type-check
npm run lint
npm run build
git diff --check
```

### Upstream 新版本发布 playbook

下面是完整操作模板。实际版本号按当时 upstream 最新 tag 替换。

1. 后端 rebase 并验证：

```bash
cd /Users/m/Projects/CLIProxyAPI/.worktrees/usage-web-sqlite
git fetch upstream --tags
git fetch origin --tags
git switch usage-web-sqlite
git rebase upstream/main

go test ./internal/usagesqlite ./internal/config ./internal/managementasset ./internal/api/handlers/management ./internal/api ./internal/watcher/diff ./internal/redisqueue
go build -o test-output ./cmd/server
rm test-output
git diff --check
```

2. 前端 rebase 并验证：

```bash
cd /Users/m/Projects/Cli-Proxy-API-Management-Center
git fetch upstream --tags
git fetch origin --tags
git switch usage-dashboard-sqlite-api
git rebase upstream/main

npm run type-check
npm run lint
npm run build
git diff --check
```

3. 先发布前端：

```bash
git status --short --branch
git push -u origin usage-dashboard-sqlite-api

git tag -l v1.8.3.1
git ls-remote --tags origin v1.8.3.1
git tag -a v1.8.3.1 -m "Release v1.8.3.1"
git push origin v1.8.3.1

cp dist/index.html /private/tmp/management.html
gh release create v1.8.3.1 /private/tmp/management.html \
  --repo msinx/Cli-Proxy-API-Management-Center \
  --title "v1.8.3.1" \
  --notes "SQLite usage dashboard for the management center."

gh release view v1.8.3.1 \
  --repo msinx/Cli-Proxy-API-Management-Center \
  --json tagName,name,url,assets
```

如果前端 release 已存在但需要更新 `management.html`：

```bash
gh release upload v1.8.3.1 /private/tmp/management.html \
  --clobber \
  --repo msinx/Cli-Proxy-API-Management-Center
```

4. 再发布后端：

```bash
cd /Users/m/Projects/CLIProxyAPI/.worktrees/usage-web-sqlite
git status --short --branch
git push -u origin usage-web-sqlite

git tag -l v6.10.2.1
git ls-remote --tags origin v6.10.2.1
git tag -a v6.10.2.1 -m "Release v6.10.2.1"
git push origin v6.10.2.1

gh run list --repo msinx/CLIProxyAPI --workflow goreleaser --limit 5
gh run watch <run-id> --repo msinx/CLIProxyAPI --exit-status
gh release view v6.10.2.1 --repo msinx/CLIProxyAPI --json tagName,name,url,assets
```

5. 最终确认：

```bash
gh release view v1.8.3.1 --repo msinx/Cli-Proxy-API-Management-Center --json tagName,url,assets
gh release view v6.10.2.1 --repo msinx/CLIProxyAPI --json tagName,url,assets
```

确认内容：

- 前端 release 有且只有当前需要的 `management.html`。
- 后端 release 有 `checksums.txt` 和各平台二进制包。
- 两个仓库 `git status --short --branch` 没有未提交变更。
- 后端 tag 对应的 commit 包含管理面板 updater 指向个人前端 fork 的改动。

## 当前已发布版本快照

前端：

- 分支：`usage-dashboard-sqlite-api`
- 提交：`ecd1293 Restore SQLite usage dashboard in the management center`
- tag：`v1.8.2.2`
- release：https://github.com/msinx/Cli-Proxy-API-Management-Center/releases/tag/v1.8.2.2
- release asset：`management.html`
- asset sha256：`cdfc230645f426152daf75f2182ec4e38b1e9630efb80b23ca7069208fc38a68`

后端：

- 分支：`usage-web-sqlite`
- 提交：`e3a9bd07 Restore in-process SQLite usage tracking for the management dashboard`
- tag：`v6.10.1.1`
- release：https://github.com/msinx/CLIProxyAPI/releases/tag/v6.10.1.1
- release 由 goreleaser 自动生成，包含 `checksums.txt` 和各平台二进制包。

## 验证基线

前端发布前至少运行：

```bash
npm run type-check
npm run lint
npm run build
git diff --check
```

后端发布前至少运行：

```bash
go test ./internal/usagesqlite ./internal/config ./internal/managementasset ./internal/api/handlers/management ./internal/api ./internal/watcher/diff ./internal/redisqueue
go build -o test-output ./cmd/server
rm test-output
git diff --check
```

已知情况：

- 后端完整 `go test ./...` 曾出现与本功能无关的失败，涉及 `internal/registry` 的 Codex free model 测试和 `internal/runtime/executor` 的 Antigravity credits 测试。rebase 后如果完整测试失败，需要先区分是否仍是这些 upstream/外部数据相关问题。
- `go build` 在沙箱中可能输出 Go module stat cache 的 `operation not permitted` warning；只要退出码为 0 且二进制生成成功，该 warning 不代表编译失败。
- GitHub Actions 可能提示 Node.js 20 actions 未来弃用，这是 workflow 维护项，不影响已成功的 goreleaser release。

## 未来让 Codex 处理时的提示词建议

可以直接这样描述任务：

```text
请基于 docs/fork-maintenance-usage-dashboard_CN.md 恢复上下文。
我要把个人 fork rebase 到 upstream 最新版本，并保留 SQLite usage dashboard。
请先 review upstream 差异和冲突风险，不要推 upstream。
完成后按文档里的验证和 release 顺序处理，tag 继续用 4 段版本号。
```

如果只是检查发布状态：

```text
请基于 docs/fork-maintenance-usage-dashboard_CN.md 检查当前前后端 fork 的分支、tag、release asset 和后端 goreleaser 状态。
```
