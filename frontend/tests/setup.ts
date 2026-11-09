import { config } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia, type Pinia } from 'pinia'
import { afterEach, beforeEach, vi } from 'vite-plus/test'

vi.mock('@wailsio/runtime', () => ({
  Call: {
    ByID: vi.fn(),
    ByName: vi.fn(),
  },
  CancellablePromise: Promise,
  Create: {
    Any: () => (v: unknown) => v,
    Array: () => (v: unknown) => v,
    ByteSlice: (v: unknown) => v,
    Map: () => (v: unknown) => v,
    Nullable: () => (v: unknown) => v,
    Struct: () => (v: unknown) => v,
  },
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
}))

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
