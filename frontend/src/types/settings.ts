import type { ThemeMode } from '@/types/appearance'

export type SettingsTabKey = 'general' | 'appearance' | 'about'

export interface AppSettings {
  theme: ThemeMode
  autoCheckUpdate: boolean
}

export interface SettingsPatch {
  theme?: ThemeMode
  autoCheckUpdate?: boolean
}
