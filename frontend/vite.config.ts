import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The API base defaults to the local backend; override with VITE_API_TARGET.
const apiTarget = process.env.VITE_API_TARGET ?? "http://localhost:8080";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: apiTarget,
        changeOrigin: true,
      },
    },
  },
});
