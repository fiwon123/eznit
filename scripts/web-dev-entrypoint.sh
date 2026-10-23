#!/bin/sh
# Entrypoint for the `web-dev` compose service: the Vite dev server.
#
# POSIX sh on purpose — the Node image's /bin/sh is dash and nothing here needs
# more than that. Unlike scripts/dev-entrypoint.sh this one needs no run-time
# CRLF guard: it is COPYed into the image rather than bind-mounted, and the
# build normalises the bytes it copies (see web/Dockerfile.dev). A bind-mounted
# script cannot do that, which is why the Go sandbox has to detect the case
# while it runs.
set -eu

cd /workspace/web

# `npm ci` only when the volume is cold or the lockfile is newer than the last
# install. The named volume survives `make dev-down`, so an unconditional
# install would add seconds to every start for nothing.
if [ ! -f node_modules/.package-lock.json ] || [ package-lock.json -nt node_modules/.package-lock.json ]; then
    echo "[web-dev] installing the pinned dependencies (npm ci)..."
    npm ci --no-audit --no-fund
fi

# --host 0.0.0.0: the server has to listen on the container interface, not only
# on loopback, or the published port reaches nothing.
# --strictPort: Vite otherwise picks the next free port, and a different port
# inside the container is not published — the UI would be silently dead.
exec npm run dev -- --host 0.0.0.0 --port 5173 --strictPort
