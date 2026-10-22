import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// The dev server proxies /v1 to the API, so the browser sees one origin and
// the API needs no CORS headers. In production the API serves web/dist
// itself, which is the same thing without the proxy.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/v1": {
        target: "http://localhost:4000",
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
