export const THEME_MODES = ['system', 'light', 'dark'] as const

export const THEME_MODE_META = [
  { id: 'system', label: '跟随系统' },
  { id: 'light', label: '浅色' },
  { id: 'dark', label: '深色' },
] as const

export const DEFAULT_THEME_MODE = 'system' satisfies (typeof THEME_MODES)[number]
