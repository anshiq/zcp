#!/usr/bin/env bash
# Preseed for greenfield-node-postgres scenario.
#
# Intentionally a no-op — greenfield means empty CWD. Agent must
# scaffold package.json, src/index.js, zerops.yaml, .gitignore, and
# initial .env.local from scratch as part of the bootstrap close-out.

set -euo pipefail
cd "${1:-.}"

if [ -n "$(ls -A 2>/dev/null | grep -v '^\.zcp$' || true)" ]; then
  echo "preseed: workdir not empty (excluding .zcp/) — greenfield scenario requires empty CWD" >&2
  exit 1
fi

echo "preseed: workdir empty — agent scaffolds project from scratch"
