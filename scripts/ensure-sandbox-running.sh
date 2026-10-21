#!/bin/bash
set -euo pipefail

# ensure-sandbox-running.sh — bring the dev sandbox up without gratuitously rebuilding
#
# Three cases:
#   1. Never initialized (no `dev` container) -> up -d --build
#   2. Exists but not running -> up -d (no rebuild)
#   3. Already running -> do nothing, wait for healthy

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT_DIR"

COMPOSE="${COMPOSE:-docker compose}"
DEV_SERVICE="${SANDBOX_DEV_SERVICE:-dev}"
# The sandbox is not standalone here: the API runs against the `db` service on
# the compose network, so the stack comes up with it. The status check below
# still watches the `dev` service alone — that is the one worth waiting for.
read -r -a DEV_SERVICES <<< "${SANDBOX_DEV_SERVICES:-dev db migrate}"
WAIT_TIMEOUT="${SANDBOX_WAIT_TIMEOUT:-180}"

log() { echo "[sandbox] $*" >&2; }

service_state() {
    local service="$1" state
    state="$($COMPOSE ps --all --format '{{.State}}' "$service" 2>/dev/null | head -n 1 || true)"
    printf '%s' "$state"
}

container_id() {
    local service="$1" id
    id="$($COMPOSE ps --all --quiet "$service" 2>/dev/null | head -n 1 || true)"
    printf '%s' "$id"
}

state="$(service_state "$DEV_SERVICE")"

if [ -z "$state" ]; then
    log "never initialized; starting dev with build"
    $COMPOSE up -d --build "${DEV_SERVICES[@]}"
else
    case "$state" in
        running)
            log "already running; waiting for healthy"
            ;;
        *)
            log "exists ($state); starting"
            $COMPOSE up -d "${DEV_SERVICES[@]}"
            ;;
    esac
fi
