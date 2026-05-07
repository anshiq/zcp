#!/usr/bin/env bash
# Preseed for recipe-nodejs-hello-world scenario.
#
# Intentionally a no-op: CWD must stay empty so the agent's
# bootstrap-recipe-local-clone atom guidance triggers (verify-empty +
# git clone <repo> .). Pre-populating any files would mask the
# clone-into-empty-CWD path the recipe-local flow depends on.
#
# The Runner sets up the workdir; this script just confirms the cwd
# argument and exits clean.

set -euo pipefail
cd "${1:-.}"

# Sanity: refuse to seed if the dir already has content the runner
# didn't clean up. Better to fail early than to mask the clone step.
if [ -n "$(ls -A 2>/dev/null | grep -v '^\.zcp$' || true)" ]; then
  echo "preseed: workdir not empty (excluding .zcp/) — recipe-local scenario requires empty CWD" >&2
  exit 1
fi

echo "preseed: workdir empty — agent will clone recipe app repo on its own"
