# Embedded CPA Usage Keeper Upstream Snapshot

Upstream repository: https://github.com/Willxup/cpa-usage-keeper
Imported branch: main
Imported commit: a6abe9021aaab39a7a9e2fb18386c9f0208fb34c

## Purpose

This directory vendors CPA Usage Keeper functionality into CLIProxyAPI so the personal fork can serve a full usage dashboard at `/usage` without modifying the official CPAMC management frontend.

## Build Strategy

CLIProxyAPI release and container builds currently use `CGO_ENABLED=0`. The embedded keeper implementation therefore uses the pure-Go SQLite path for runtime storage compatibility instead of depending on `github.com/mattn/go-sqlite3`.

## Update Procedure

1. Fetch the latest upstream commit.
2. Copy reusable upstream backend packages into `internal/usagekeeper/upstream/`.
3. Copy the React dashboard into `web/usage-keeper/`.
4. Reapply only documented local patches:
   - import path rewrites
   - pure-Go SQLite driver compatibility
   - `/usage` base path configuration
   - management-key-to-session authentication bridge
   - in-process usage ingestion adapter
   - embedded dashboard asset binding
5. Run:

```bash
npm --prefix web/usage-keeper ci
npm --prefix web/usage-keeper run typecheck
npm --prefix web/usage-keeper run test
npm --prefix web/usage-keeper run lint
npm --prefix web/usage-keeper run build
gofmt -w internal/usagekeeper internal/api internal/config internal/watcher
go test ./internal/usagekeeper/... ./internal/api/... ./internal/redisqueue/... ./internal/config/... ./internal/watcher/... ./test/...
go build -o test-output ./cmd/server
rm test-output
```

## Local Patch Policy

Keep CLIProxyAPI-specific code in `internal/usagekeeper/adapter/` and `internal/api/modules/usagekeeper/`.
Avoid editing upstream snapshot files except for import paths and small compatibility fixes recorded here.
