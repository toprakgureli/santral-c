import path from "node:path";
import { defineConfig } from "vitest/config";

// Unit tests run in a simulated browser page; nothing reaches a server.
export default defineConfig({
  resolve: {
    alias: { "@": path.resolve(__dirname, "src") },
  },
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.{ts,tsx}"],
  },
});
