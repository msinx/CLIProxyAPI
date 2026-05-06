# Embedded CPA Usage Keeper Upstream Snapshot

Upstream repository: https://github.com/Willxup/cpa-usage-keeper
Imported branch: main
Imported backend/frontend snapshot commit: 5808976c06c0ce5c1d72377750be7baabb05b00a

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
   - non-secret usage source and usage identity lookup keys
   - cancellable maintenance worker
   - embedded dashboard asset binding
5. Run:

```bash
npm --prefix web/usage-keeper run typecheck
npm --prefix web/usage-keeper run test
npm --prefix web/usage-keeper run lint
scripts/verify-usage-keeper-assets.sh
gofmt -w internal/usagekeeper internal/api internal/config internal/watcher
go test ./internal/usagekeeper/... ./internal/api/... ./internal/redisqueue/... ./internal/config/... ./internal/watcher/... ./test/...
go build -o test-output ./cmd/server
rm test-output
```

The release path is intentionally local-only: `/usage` assets are built from
`web/usage-keeper`, synced into `internal/usagekeeper/assets/dist`, and embedded
into the Go binary. Do not fetch CPAMC, `management.html`, or any remote release
asset for the usage dashboard.

## Exact Import Commands

```bash
rm -rf /tmp/cpa-usage-keeper
git clone --depth=1 https://github.com/Willxup/cpa-usage-keeper.git /tmp/cpa-usage-keeper
rsync -a --delete /tmp/cpa-usage-keeper/internal/api/ internal/usagekeeper/upstream/api/
rsync -a --delete /tmp/cpa-usage-keeper/internal/auth/ internal/usagekeeper/upstream/auth/
rsync -a --delete /tmp/cpa-usage-keeper/internal/backup/ internal/usagekeeper/upstream/backup/
rsync -a --delete /tmp/cpa-usage-keeper/internal/cpa/ internal/usagekeeper/upstream/cpa/
rsync -a --delete /tmp/cpa-usage-keeper/internal/config/ internal/usagekeeper/upstream/config/
rsync -a --delete /tmp/cpa-usage-keeper/internal/logging/ internal/usagekeeper/upstream/logging/
rsync -a --delete /tmp/cpa-usage-keeper/internal/models/ internal/usagekeeper/upstream/models/
rsync -a --delete /tmp/cpa-usage-keeper/internal/redact/ internal/usagekeeper/upstream/redact/
rsync -a --delete /tmp/cpa-usage-keeper/internal/repository/ internal/usagekeeper/upstream/repository/
rsync -a --delete /tmp/cpa-usage-keeper/internal/service/ internal/usagekeeper/upstream/service/
rsync -a --delete /tmp/cpa-usage-keeper/web/ web/usage-keeper/
scripts/build-usage-keeper-assets.sh
```

## Local Patch Policy

Keep CLIProxyAPI-specific code in `internal/usagekeeper/adapter/` and `internal/api/modules/usagekeeper/`.
Avoid editing upstream snapshot files except for import paths and small compatibility fixes recorded here.
