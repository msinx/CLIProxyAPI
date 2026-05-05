#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

"${repo_root}/scripts/build-usage-keeper-assets.sh"

if [ -n "$(git -C "${repo_root}" status --porcelain -- internal/usagekeeper/assets/dist)" ]; then
  git -C "${repo_root}" diff -- internal/usagekeeper/assets/dist
  git -C "${repo_root}" status --short -- internal/usagekeeper/assets/dist
  echo "embedded Usage Keeper assets are stale; run scripts/build-usage-keeper-assets.sh and commit internal/usagekeeper/assets/dist" >&2
  exit 1
fi
