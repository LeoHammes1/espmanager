import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";

const go = (extra: Record<string, unknown> = {}) => ({
  target: "http://localhost:8080",
  changeOrigin: true,
  ...extra,
});

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": path.resolve(__dirname, "src") } },
  build: {
    // Embedded by Go via //go:embed all:dist in internal/web. emptyOutDir is
    // false so the committed .gitkeep that bootstraps go:embed is preserved.
    outDir: path.resolve(__dirname, "../internal/web/dist"),
    emptyOutDir: false,
  },
  server: {
    proxy: {
      "/api": go(),
      "/firmware": go(),
      "/webhook": go(),
      "/v1": go(),
      // Long-lived SSE: never time the stream out.
      "/events": go({ timeout: 0, proxyTimeout: 0 }),
    },
  },
});
