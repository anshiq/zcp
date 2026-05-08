#!/usr/bin/env bash
# Preseed for recipe-nodejs-hello-world scenario.
#
# Intentionally a no-op for application code: the agent's
# bootstrap-recipe-local-clone atom drives the clone (rsync into
# the harness-prepopulated workdir). The harness writes
# .claude / .mcp.json / .zcp / .zcp-eval-workdir / CLAUDE.md before
# this script runs — those are intended state and stay.

set -euo pipefail
cd "${1:-.}"
echo "preseed: no-op; agent will rsync recipe app repo into the prepopulated workdir"
