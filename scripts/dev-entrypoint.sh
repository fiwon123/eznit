#!/bin/bash
# CRLF guard — deliberately the first executable statement, above
# `set -euo pipefail`. There is no blank line between here and the guard on
# purpose: an empty line in a CRLF copy is a bare `\r` command, and bash would
# report `$'\r': command not found` before the guard could say anything useful.
# Comment lines absorb it; use `#` on its own for a paragraph break, never an
# empty line.
#
# This script runs through bash from the bind mount, so bash parses the *host*
# working-tree bytes. A Windows checkout whose working tree predates the
# `.gitattributes` `* text=auto eol=lf` rule keeps stale CRLF bytes, and
# `git status` stays clean throughout, because git compares normalized content.
# Bash then reads `set -euo pipefail\r` and dies with `set: pipefail: invalid
# option name` — a message that names neither the cause nor a repair.
#
# The guard has to survive the CRLF it reports. Bash parses a whole compound
# command before executing it, so a plain multi-line `if` would fail to parse
# before its own check ever ran. Every line of this block therefore ends with
# the `# crlf-guard` comment tag, which absorbs the trailing `\r`.
if [[ -f "${BASH_SOURCE[0]}" ]] && grep -q $'\r$' "${BASH_SOURCE[0]}"; then # crlf-guard
    printf '%s\n' "[dev] ${BASH_SOURCE[0]##*/} has CRLF line endings, so bash cannot parse it — the sandbox would die with 'set: pipefail: invalid option name'." "      The bind mount serves the host checkout as-is. Re-check it out with LF endings (README.md -> Troubleshooting):" >&2 # crlf-guard
    printf '%s\n' "        git rm --cached -r . >/dev/null && git reset --hard" >&2; exit 1 # crlf-guard
fi # crlf-guard
set -euo pipefail

# Dev sandbox entrypoint.
#
# The default is to sleep and wait for a shell: the API is a server you start
# when you want it (`make run_api`), not something the container should launch
# behind your back. The container's job is to give a pinned toolchain and a
# shell in which `make check`, `make run_api` and the migrations behave the
# same on every machine.
#
# The marker files below are the opt-in "start something" path: create
# `.dev/sandbox-start` in the workspace and the container runs the API instead
# of sleeping. Neither file exists in the repository, so the default behaviour
# is the sleep.

DEV_DIR="${DEV_DIR:-/workspace}"
SLEEP_ON_IDLE="${SLEEP_ON_IDLE:-true}"

# --- Passwd entry for the runtime user ---------------------------------------
if ! getent passwd "$(id -u)" >/dev/null; then
    if [[ "$(id -u)" -eq 0 ]]; then
        echo "[dev] UID 0 has no passwd entry — adding a fallback identity."
        printf 'sandbox:x:%s:%s:Sandbox User:/home/%s:/bin/bash\n' \
            "$(id -u)" "$(id -g)" "${DEV_USER:-devuser}" >> /etc/passwd
    else
        echo "[dev] WARNING: UID $(id -u) has no passwd entry." >&2
    fi
fi

# --- Start dev command if available, else sleep -------------------------------
if [ "${SLEEP_ON_IDLE}" = "true" ] && [ ! -f "${DEV_DIR}/.dev/sandbox-start" ] && [ ! -f "${DEV_DIR}/.dev/sandbox-run" ]; then
    echo "[dev] No start marker found; sleeping indefinitely for shell access."
    echo "[dev]   Run 'make check' for the gate, or 'make run_api' to start the API."
    exec sleep infinity
fi

echo "[dev] Starting the eznit API..."
cd "${DEV_DIR}" && exec make run_api
