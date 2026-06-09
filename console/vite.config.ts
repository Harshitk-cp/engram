import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Served at the site root by the Go binary (e.g. console.engram.to). In dev,
// requests to /v1 and /auth are proxied to the local API server.
export default defineConfig({
  base: "/",
  plugins: [react()],
  server: {
    // Dev server (npm run dev, :5177) proxies API + auth to the live Go server.
    proxy: {
      "/v1": "http://localhost:8081",
      "/auth": "http://localhost:8081",
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
