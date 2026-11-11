import { config } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia, type Pinia } from 'pinia'
import { afterEach, beforeEach, vi } from 'vite-plus/test'

vi.mock('@wailsio/runtime', async () => {
  const { createRequire } = await import('node:module')
  const { pathToFileURL } = await import('node:url')
  // 只加载已安装运行时的纯转换器，避免启动桌面桥接或请求 custom.js。
  const entry = pathToFileURL(createRequire(import.meta.url).resolve('@wailsio/runtime'))
  const converters = (await import(
    /* @vite-ignore */ new URL('./create.js', entry).href
  )) as (typeof import('@wailsio/runtime'))['Create']
  return {
    Call: {
      ByID: vi.fn(),
      ByName: vi.fn(),
    },
    CancellablePromise: Promise,
    Create: { ...converters, Events: {} },
    Events: {
      On: vi.fn(() => () => {}),
      Emit: vi.fn(),
    },
    Browser: {
      OpenURL: vi.fn(),
    },
    System: {
      IsMac: vi.fn(() => true),
      IsWindows: vi.fn(() => false),
      IsLinux: vi.fn(() => false),
    },
    Window: {
      Minimise: vi.fn(),
      ToggleMaximise: vi.fn(),
      Close: vi.fn(),
      IsMaximised: vi.fn(async () => false),
      IsFullscreen: vi.fn(async () => false),
    },
  }
})

let testPinia: Pinia | undefined

beforeEach(async () => {
  testPinia = createPinia()
  setActivePinia(testPinia)
  if (typeof window !== 'undefined' && window.localStorage) {
    window.localStorage.clear()
  }
  const { router } = await import('@/router')
  config.global.plugins = [testPinia, router]
  await router.replace('/workspace')
})

afterEach(() => {
  if (testPinia) {
    disposePinia(testPinia)
    testPinia = undefined
  }
})
