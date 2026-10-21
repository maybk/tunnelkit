import { fileURLToPath } from 'node:url'
import tailwindcss from '@tailwindcss/vite'
import vue from '@vitejs/plugin-vue'
import wails from '@wailsio/runtime/plugins/vite'
import { defineConfig } from 'vitest/config'

// https://vitejs.dev/config/
export default defineConfig(({ mode }) => ({
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    // Wails v3 dev 模式下存在两套连接路径：
    //   1. 健康检查 → FRONTEND_DEVSERVER_URL=http://localhost:PORT
    //      Go 在 macOS 上解析 localhost 优先 ::1 (IPv6)
    //   2. 资产代理 → 内部硬编码 dial tcp4 127.0.0.1:PORT (IPv4)
    //
    // 只监听 localhost / 127.0.0.1 / 0.0.0.0 都会导致其中一条路径不通。
    // host: "::" (IPv6 通配地址) 在 macOS 上默认 IPV6_V6ONLY=0，
    // 同时接受 IPv4 和 IPv6 连接，满足 Wails 两条路径的需求。
    host: '::',
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  // 测试环境不加载 Wails 与 Tailwind 插件：单元测试只覆盖 composable 逻辑，
  // 不需要绑定注入，也不需要生成样式。
  plugins: mode === 'test' ? [vue()] : [tailwindcss(), vue(), wails('./bindings')],
  test: {
    environment: 'happy-dom',
    // 测试放在 src 之外，避免与业务代码混在一起。
    include: ['tests/**/*.spec.ts'],
  },
}))
