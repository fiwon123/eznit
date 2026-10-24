import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// The dev server proxies /v1 to the API, so the browser sees one origin and
// the API needs no CORS headers. In production the API serves web/dist
// itself, which is the same thing without the proxy.
//
// The target is configurable because "the API" has two addresses that depend on
// where Vite runs: on the host it is loopback, and inside the `web-dev`
// container it is the `dev` service on the Compose network. See the service in
// docker-compose.yml and scripts/web-dev-entrypoint.sh.
const apiTarget = process.env.VITE_API_TARGET ?? "http://localhost:4000";

// The browser connects to the *host* port. That is 5173 unless WEB_PORT moved
// it, in which case the HMR websocket has to be told where to reconnect — the
// container port is not the published one.
const hmrClientPort = Number(process.env.VITE_HMR_CLIENT_PORT ?? "") || undefined;

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/v1": {
        target: apiTarget,
        changeOrigin: true,
      },
    },
    hmr: hmrClientPort === undefined ? undefined : { clientPort: hmrClientPort },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
