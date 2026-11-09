import type { THEME_MODES } from '@/constants/appearance'

export type ThemeMode = (typeof THEME_MODES)[number]
export type ResolvedTheme = 'light' | 'dark'

export interface CachedAppearance {
  mode: ThemeMode
}
