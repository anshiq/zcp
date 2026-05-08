#!/usr/bin/env bash
# Preseed for greenfield-node-postgres scenario.
#
# Intentionally a no-op for application code: agent scaffolds
# package.json / src/index.js / zerops.yaml / .gitignore / .env.local
# from scratch as part of bootstrap close-out. The harness writes
# .claude / .mcp.json / .zcp / .zcp-eval-workdir / CLAUDE.md before
# this script runs — those are intended state and stay.

set -euo pipefail
cd "${1:-.}"
echo "preseed: no-op; agent scaffolds the application from scratch"
