import { ref } from 'vue'
import { defineStore } from 'pinia'
import { profileApi } from '@/api/profile'
import { DEFAULT_THEME_MODE } from '@/constants/appearance'
import { useAppearanceStore } from '@/stores/useAppearanceStore'
import { useNoticeStore } from '@/stores/useNoticeStore'
import type { AppSettings, SettingsPatch } from '@/types/settings'

const AUTO_CHECK_UPDATE_REPLACE_KEY = 'setting:autoCheckUpdate'
const THEME_REPLACE_KEY = 'setting:theme'

const DEFAULT_SETTINGS: AppSettings = {
  theme: DEFAULT_THEME_MODE,
  autoCheckUpdate: true,
}

export const useSettingsStore = defineStore('settings', () => {
  const appearanceStore = useAppearanceStore()
  const noticeStore = useNoticeStore()

  const settings = ref<AppSettings>({
    ...DEFAULT_SETTINGS,
    theme: appearanceStore.theme,
  })
  const loaded = ref(false)

  let saveQueue: Promise<boolean> = Promise.resolve(true)
  let confirmedBaseline: AppSettings | null = null
  let mutationVersion = 0
  const pendingPatches: SettingsPatch[] = []

  const applyProjected = (next: AppSettings) => {
    settings.value = next
    appearanceStore.setTheme(next.theme)
  }

  const projectPending = () => {
    if (!confirmedBaseline) return
    const projected = pendingPatches.reduce<AppSettings>(
      (current, patch) => ({
        theme: patch.theme ?? current.theme,
        autoCheckUpdate: patch.autoCheckUpdate ?? current.autoCheckUpdate,
      }),
      confirmedBaseline,
    )
    applyProjected(projected)
  }

  const loadSettings = async (): Promise<AppSettings> => {
    const startVersion = mutationVersion
    const next = await profileApi.getSettings()
    loaded.value = true
    // 加载期间若已有更新的用户保存操作，避免旧快照回写覆盖最新状态。
    if (mutationVersion !== startVersion) {
      return settings.value
    }
    if (pendingPatches.length > 0) {
      confirmedBaseline = next
      projectPending()
    } else {
      applyProjected(next)
    }
    return settings.value
  }

  const resolveReplaceKey = (patch: SettingsPatch): string | undefined => {
    if (patch.autoCheckUpdate !== undefined) return AUTO_CHECK_UPDATE_REPLACE_KEY
    if (patch.theme !== undefined) return THEME_REPLACE_KEY
    return undefined
  }

  const saveSettings = (patch: SettingsPatch): Promise<boolean> => {
    if (patch.theme === undefined && patch.autoCheckUpdate === undefined) {
      return Promise.resolve(true)
    }
    mutationVersion += 1
    const replaceKey = resolveReplaceKey(patch)
    if (pendingPatches.length === 0) {
      confirmedBaseline = { ...settings.value }
    }
    pendingPatches.push(patch)
    projectPending()

    const run = async (): Promise<boolean> => {
      try {
        const saved = await profileApi.saveSettings(patch)
        confirmedBaseline = saved
        loaded.value = true
        const supersededAuto =
          patch.autoCheckUpdate !== undefined &&
          pendingPatches.some((item) => item !== patch && item.autoCheckUpdate !== undefined)
        if (patch.autoCheckUpdate !== undefined && !supersededAuto) {
          noticeStore.showSuccess(
            saved.autoCheckUpdate ? '已开启自动检查更新' : '已关闭自动检查更新',
            undefined,
            { replaceKey: AUTO_CHECK_UPDATE_REPLACE_KEY },
          )
        }
        return true
      } catch (err) {
        noticeStore.showError(err, '保存设置失败', replaceKey ? { replaceKey } : undefined)
        return false
      } finally {
        const idx = pendingPatches.indexOf(patch)
        if (idx >= 0) pendingPatches.splice(idx, 1)
        projectPending()
        if (pendingPatches.length === 0) {
          confirmedBaseline = null
        }
      }
    }

    saveQueue = saveQueue.then(run, run)
    return saveQueue
  }

  return {
    settings,
    loaded,
    loadSettings,
    saveSettings,
  }
})
