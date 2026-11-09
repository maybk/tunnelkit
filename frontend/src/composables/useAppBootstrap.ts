import { onMounted, onUnmounted } from 'vue'
import { useAppearanceStore } from '@/stores/useAppearanceStore'
import { useDesktopWindowStore } from '@/stores/useDesktopWindowStore'
import { useNoticeStore } from '@/stores/useNoticeStore'
import { useProfilesStore } from '@/stores/useProfilesStore'
import { useSystemStore } from '@/stores/useSystemStore'
import { useVpnStore } from '@/stores/useVpnStore'

export function useAppBootstrap() {
  useAppearanceStore()
  const desktopWin = useDesktopWindowStore()
  const profilesStore = useProfilesStore()
  const vpnStore = useVpnStore()
  const systemStore = useSystemStore()
  const noticeStore = useNoticeStore()

  let updateCheckTimer: ReturnType<typeof setTimeout> | null = null
  let disposed = false

  onMounted(async () => {
    desktopWin.startWatching()
    vpnStore.startWatching()
    systemStore.startWatching()

    const results = await Promise.allSettled([
      vpnStore.loadState(),
      systemStore.loadSystemInfo(),
      profilesStore.loadProfiles(),
    ])
    const stateResult = results[0]
    const profilesResult = results[2]
    if (stateResult?.status === 'fulfilled' && profilesResult?.status === 'fulfilled') {
      const activeProfileId = stateResult.value.profileId
      if (profilesResult.value.some((profile) => profile.id === activeProfileId)) {
        profilesStore.selectProfile(activeProfileId)
      }
    }
    for (const result of results) {
      if (result.status === 'rejected') noticeStore.showError(result.reason, '初始化工作区失败')
    }

    if (disposed) return
    updateCheckTimer = setTimeout(() => {
      void systemStore.checkUpdate({ silent: true })
    }, 1500)
  })

  onUnmounted(() => {
    disposed = true
    if (updateCheckTimer) {
      clearTimeout(updateCheckTimer)
      updateCheckTimer = null
    }
    desktopWin.stopWatching()
    vpnStore.stopWatching()
    systemStore.stopWatching()
  })
}
