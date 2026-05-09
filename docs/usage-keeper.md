# Embedded CPA Usage Keeper

CLIProxyAPI includes an embedded CPA Usage Keeper dashboard at `/usage`.

## Runtime Behavior

- The dashboard is served from assets embedded in the CLIProxyAPI binary.
- The official CPAMC management panel is separate and continues to use its normal `management.html` asset flow.
- Usage data is stored in SQLite at `data/usage-keeper.db` by default.
- Backup files are stored under `data/usage-keeper-backups` by default.
- A maintenance worker removes old usage events, cleans upstream keeper storage tables, and writes SQLite backups when backups are enabled.

## Authentication

The `/usage` dashboard uses CPA Usage Keeper's session-cookie flow. Login accepts the existing CLIProxyAPI management key and creates an HTTP-only cookie scoped to `/usage`.

Public routes:

- `GET /usage`
- `GET /usage/`
- `GET /usage/assets/*`
- `GET /usage/api/v1/auth/session`
- `POST /usage/api/v1/auth/login`
- `POST /usage/api/v1/auth/logout`

All other `/usage/api/v1/*` routes require the `/usage` session cookie.

## Config Interaction

`usage-statistics-enabled` gates ingestion only. When it is `false`, the dashboard still mounts, the database still opens, and existing data remains readable, but new usage records are discarded.

The first-pass embedded integration intentionally does not add a broad `usage-keeper-*` YAML surface. Defaults are kept in `internal/usagekeeper/adapter/config.go`.

## Secret Handling

The embedded store must not persist raw API keys, management keys, or auth tokens. Provider grouping and lookup fields use stable non-secret identifiers derived from runtime auth metadata or hashed provider keys.

Dashboard API responses redact raw source values and expose stable public source keys instead.

## Frontend Assets

The React source snapshot is kept under `web/usage-keeper`. Its local `dist` directory is ignored. Release assets must be copied into `internal/usagekeeper/assets/dist`, which is the directory embedded into the Go binary.

After rebuilding the frontend, sync assets with:

```bash
scripts/build-usage-keeper-assets.sh
```

CI verifies the committed embed output with:

```bash
scripts/verify-usage-keeper-assets.sh
```

## Upstream Update Workflow

Import a fresh upstream snapshot with:

```bash
rm -rf /tmp/cpa-usage-keeper
git clone --depth=1 https://github.com/Willxup/cpa-usage-keeper.git /tmp/cpa-usage-keeper
rsync -a --delete /tmp/cpa-usage-keeper/internal/api/ internal/usagekeeper/upstream/api/
rsync -a --delete /tmp/cpa-usage-keeper/internal/auth/ internal/usagekeeper/upstream/auth/
rsync -a --delete /tmp/cpa-usage-keeper/internal/backup/ internal/usagekeeper/upstream/backup/
rsync -a --delete /tmp/cpa-usage-keeper/internal/cpa/ internal/usagekeeper/upstream/cpa/
rsync -a --delete /tmp/cpa-usage-keeper/internal/config/ internal/usagekeeper/upstream/config/
rsync -a --delete /tmp/cpa-usage-keeper/internal/entities/ internal/usagekeeper/upstream/entities/
rsync -a --delete /tmp/cpa-usage-keeper/internal/logging/ internal/usagekeeper/upstream/logging/
rsync -a --delete /tmp/cpa-usage-keeper/internal/poller/ internal/usagekeeper/upstream/poller/
rsync -a --delete /tmp/cpa-usage-keeper/internal/redact/ internal/usagekeeper/upstream/redact/
rsync -a --delete /tmp/cpa-usage-keeper/internal/repository/ internal/usagekeeper/upstream/repository/
rsync -a --delete /tmp/cpa-usage-keeper/internal/service/ internal/usagekeeper/upstream/service/
rsync -a --delete /tmp/cpa-usage-keeper/internal/updatecheck/ internal/usagekeeper/upstream/updatecheck/
rsync -a --delete /tmp/cpa-usage-keeper/internal/version/ internal/usagekeeper/upstream/version/
rsync -a --delete /tmp/cpa-usage-keeper/web/ web/usage-keeper/
scripts/build-usage-keeper-assets.sh
```

Then reapply the documented local patches from
`internal/usagekeeper/README_UPSTREAM.md`, update the recorded upstream commits,
and rerun the full Go and frontend verification suite.
