#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
web_dir="${repo_root}/web/usage-keeper"
embed_dir="${repo_root}/internal/usagekeeper/assets/dist"
npm_cache="${NPM_CONFIG_CACHE:-${TMPDIR:-/tmp}/cliproxy-npm-cache}"
tmp_root="${TMPDIR:-/tmp}"
build_dir="$(mktemp -d "${tmp_root%/}/cliproxy-usage-keeper.XXXXXX")"

cleanup() {
  rm -rf "${build_dir}"
}
trap cleanup EXIT

if [ ! -f "${web_dir}/package.json" ]; then
  echo "missing Usage Keeper frontend snapshot: ${web_dir}/package.json" >&2
  exit 1
fi

if grep -R --exclude-dir=node_modules --exclude-dir=dist \
  "Cli-Proxy-API-Management-Center\|CPAMC\|management.html" \
  "${web_dir}" >/dev/null; then
  echo "usage dashboard snapshot appears to reference CPAMC or management assets" >&2
  exit 1
fi

rsync -a --delete \
  --exclude node_modules \
  --exclude dist \
  "${web_dir}/" "${build_dir}/"

(
  cd "${build_dir}"
  npm --cache "${npm_cache}" ci
  npm --cache "${npm_cache}" run build
)

rm -rf "${embed_dir}"
mkdir -p "${embed_dir}"
rsync -a --delete "${build_dir}/dist/" "${embed_dir}/"

if [ ! -f "${embed_dir}/index.html" ]; then
  echo "usage dashboard build did not produce ${embed_dir}/index.html" >&2
  exit 1
fi

if [ ! -d "${embed_dir}/assets" ]; then
  echo "usage dashboard build did not produce ${embed_dir}/assets" >&2
  exit 1
fi
