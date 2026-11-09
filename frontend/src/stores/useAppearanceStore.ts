import { computed, ref } from 'vue'
import { useColorMode } from '@vueuse/core'
import { defineStore } from 'pinia'
import { APPEARANCE_STORAGE_KEY } from '@/constants/storage'
import type { ThemeMode } from '@/types/appearance'
import { DEFAULT_APPEARANCE, parseCachedAppearance } from '@/validate/storage'

function readStoredAppearance() {
  try {
    return parseCachedAppearance(globalThis.localStorage?.getItem(APPEARANCE_STORAGE_KEY) ?? null)
  } catch {
    return { ...DEFAULT_APPEARANCE }
  }
}

export const useAppearanceStore = defineStore('appearance', () => {
  const initial = readStoredAppearance()
  const theme = ref<ThemeMode>(initial.mode)

  const storedMode = computed({
    get: () => (theme.value === 'system' ? ('auto' as const) : theme.value),
    set: (mode: 'auto' | 'light' | 'dark') => {
      theme.value = mode === 'auto' ? 'system' : mode
    },
  })

  const colorMode = useColorMode({
    storageRef: storedMode,
    modes: { light: '', dark: 'dark' },
    onChanged(mode, apply) {
      apply(mode)
      if (typeof document !== 'undefined') {
        document.documentElement.setAttribute('data-theme', mode)
      }
    },
  })

  const resolvedTheme = colorMode.state

  const persistCache = () => {
    try {
      globalThis.localStorage?.setItem(
        APPEARANCE_STORAGE_KEY,
        JSON.stringify({ mode: theme.value }),
      )
    } catch {
      // 忽略无痕/受限环境下的 localStorage 写入失败
    }
  }

  const setTheme = (mode: ThemeMode) => {
    theme.value = mode
    persistCache()
  }

  const cycleTheme = () => {
    const order: readonly ThemeMode[] = ['system', 'light', 'dark']
    const nextIdx = (order.indexOf(theme.value) + 1) % order.length
    setTheme(order[nextIdx] ?? 'system')
  }

  return {
    theme,
    resolvedTheme,
    setTheme,
    cycleTheme,
  }
})
