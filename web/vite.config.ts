import { defineConfig, type Plugin } from "vite";
import react from "@vitejs/plugin-react";

// The pinned XR store dynamically imports an emulator even when emulate:false.
// Remove its opt-in branch at build time so production never ships synthetic
// hardware, developer UI, or bundled sample rooms. Fail if the upstream shape changes.
function realHeadsetsOnly(): Plugin {
  return {
    name: "agent-room-real-headsets",
    apply: "build",
    enforce: "pre",
    transform(code, id) {
      if (!id.split("\\").join("/").endsWith("/@pmndrs/xr/dist/store.js")) return null;
      const condition = "if (typeof window !== 'undefined' && emulate != false) {";
      if (code.split(condition).length !== 2) throw new Error("Review the pinned XR emulator boundary before building.");
      return { code: code.replace(condition, "if (false) {"), map: null };
    },
  };
}

export default defineConfig({
  plugins: [react(), realHeadsetsOnly()],
  server: {
    port: 4173,
    strictPort: true,
    proxy: {
      "/api": {
        target: process.env.AGENT_ROOM_API_URL ?? "http://127.0.0.1:8080",
        changeOrigin: false,
        ws: true,
      },
    },
  },
  build: {
    sourcemap: true,
    target: "es2022",
  },
});
