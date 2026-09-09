import { defineConfig } from "vite";

// Builds the two extension pages (offscreen + popup) as ES modules. The plain
// service worker and content script live in public/ and are copied verbatim,
// alongside manifest.json.
export default defineConfig({
  build: {
    outDir: "dist",
    emptyOutDir: true,
    rollupOptions: {
      input: {
        popup: "popup.html",
      },
      output: {
        entryFileNames: "assets/[name].js",
        chunkFileNames: "assets/[name].js",
        assetFileNames: "assets/[name][extname]",
      },
    },
  },
});
