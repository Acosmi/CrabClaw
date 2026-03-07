#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SRC_DIR="${ROOT_DIR}/dist/control-ui"
DST_DIR="${ROOT_DIR}/backend/cmd/desktop/frontend/dist"

if [[ ! -f "${SRC_DIR}/index.html" ]]; then
  echo "control UI build output not found: ${SRC_DIR}/index.html" >&2
  echo "run 'npm --prefix ui run build' first" >&2
  exit 1
fi

rm -rf "${DST_DIR}"
mkdir -p "${DST_DIR}"
cp -R "${SRC_DIR}/." "${DST_DIR}/"

echo "staged control UI assets to ${DST_DIR}"
