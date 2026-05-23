import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import path from "node:path";

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./__tests__/setup.ts"],
    include: ["__tests__/**/*.{test,spec}.{ts,tsx}", "src/**/*.{test,spec}.{ts,tsx}"],
    exclude: ["node_modules", "dist"],
  },
  resolve: {
    alias: {
      "@argus-dev/types": path.resolve(__dirname, "../types/src/index.ts"),
      "@argus-dev/types/graph": path.resolve(__dirname, "../types/src/graph.ts"),
      "@argus-dev/types/git": path.resolve(__dirname, "../types/src/git.ts"),
      "@argus-dev/types/docs": path.resolve(__dirname, "../types/src/docs.ts"),
      "@argus-dev/types/decisions": path.resolve(__dirname, "../types/src/decisions.ts"),
      "@argus-dev/types/dead-code": path.resolve(__dirname, "../types/src/dead-code.ts"),
      "@argus-dev/types/symbols": path.resolve(__dirname, "../types/src/symbols.ts"),
      "@argus-dev/types/chat": path.resolve(__dirname, "../types/src/chat.ts"),
      "@argus-dev/types/blast-radius": path.resolve(__dirname, "../types/src/blast-radius.ts"),
      "@argus-dev/types/jobs": path.resolve(__dirname, "../types/src/jobs.ts"),
      "@argus-dev/types/settings": path.resolve(__dirname, "../types/src/settings.ts"),
    },
  },
});
