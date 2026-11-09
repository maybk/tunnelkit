import { fileURLToPath } from 'node:url'
import tailwindcss from '@tailwindcss/vite'
import vue from '@vitejs/plugin-vue'
import wails from '@wailsio/runtime/plugins/vite'
import { defineConfig } from 'vite-plus'
import { runtimeBoundary } from './config/runtime-boundary.ts'

// https://viteplus.dev/config/
export default defineConfig(({ mode }) => {
  const frontendRoot = fileURLToPath(new URL('.', import.meta.url))

  return {
    staged: {
      '*': 'vp check --fix',
    },
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
        '@bindings': fileURLToPath(new URL('./bindings', import.meta.url)),
      },
    },
    server: {
      // Wails v3 dev 模式下存在两套连接路径：
      //   1. 健康检查 → FRONTEND_DEVSERVER_URL=http://localhost:PORT
      //      Go 在 macOS 上解析 localhost 优先 ::1 (IPv6)
      //   2. 资产代理 → 内部硬编码 dial tcp4 127.0.0.1:PORT (IPv4)
      //
      // host: "::" (IPv6 通配地址) 在 macOS 上默认 IPV6_V6ONLY=0，
      // 同时接受 IPv4 和 IPv6 连接，满足 Wails 两条路径的需求。
      host: '::',
      port: Number(process.env.WAILS_VITE_PORT) || 9245,
      strictPort: true,
    },
    plugins:
      mode === 'test'
        ? [vue()]
        : [runtimeBoundary(frontendRoot), tailwindcss(), vue(), wails('./bindings')],
    test: {
      environment: 'happy-dom',
      projects: [
        {
          extends: true,
          test: {
            name: 'unit',
            include: ['tests/{unit,architecture}/**/*.spec.ts'],
          },
        },
        {
          extends: true,
          test: {
            name: 'integration',
            include: ['tests/{api,stores,composables,components,views}/**/*.spec.ts'],
            setupFiles: ['tests/setup.ts'],
          },
        },
      ],
    },
    fmt: {
      printWidth: 100,
      semi: false,
      singleQuote: true,
      tabWidth: 2,
      useTabs: false,
      trailingComma: 'all',
      ignorePatterns: ['dist', 'bindings', 'node_modules'],
    },
    lint: {
      ignorePatterns: ['dist/**', 'bindings/**', 'node_modules/**'],
      jsPlugins: [{ name: 'vite-plus', specifier: 'vite-plus/oxlint-plugin' }],
      rules: {
        'vite-plus/prefer-vite-plus-imports': 'error',
      },
      options: {
        typeAware: true,
        typeCheck: true,
      },
    },
  }
})
