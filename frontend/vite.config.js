import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import wails from "@wailsio/runtime/plugins/vite";

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue(), wails("./bindings")],
  build: {
    // 前端使用 WebCodecs 解码 H.264，目标设为 es2022（WebView2 基于新版 Chromium）
    target: "es2022",
  },
  optimizeDeps: {
    // dev 模式下预构建依赖也使用同一目标
    esbuildOptions: {
      target: "es2022",
    },
  },
});
