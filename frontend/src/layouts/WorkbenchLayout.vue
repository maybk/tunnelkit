<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AboutDialog from '@/components/common/AboutDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import NoticeToast from '@/components/common/NoticeToast.vue'
import UpdateDialog from '@/components/common/UpdateDialog.vue'
import VpnChallengeDialog from '@/components/vpn/VpnChallengeDialog.vue'
import { useGlobalShortcuts } from '@/composables/useGlobalShortcuts'
import AppSidebar from '@/layouts/AppSidebar.vue'
import DesktopChrome from '@/layouts/DesktopChrome.vue'
import { ROUTE_NAMES } from '@/router'
import { useAppearanceStore } from '@/stores/useAppearanceStore'
import { useDesktopWindowStore } from '@/stores/useDesktopWindowStore'
import { useProfilesStore } from '@/stores/useProfilesStore'
import { useSystemStore } from '@/stores/useSystemStore'
import { useVpnStore } from '@/stores/useVpnStore'

const route = useRoute()
const router = useRouter()
const themeStore = useAppearanceStore()
const desktopWin = useDesktopWindowStore()
const profilesStore = useProfilesStore()
const vpnStore = useVpnStore()
const systemStore = useSystemStore()

useGlobalShortcuts()

const isCreating = computed(() => route.name === ROUTE_NAMES.profileCreate)

const globalStatus = computed(() => {
  const state = vpnStore.state
  if (state.status === 'connected')
    return {
      label: state.profileName ? `已连接 · ${state.profileName}` : '已连接',
      tone: 'success' as const,
    }
  if (state.status === 'connecting')
    return {
      label: state.profileName ? `正在连接 · ${state.profileName}` : '正在连接…',
      tone: 'warning' as const,
    }
  if (state.status === 'disconnecting') return { label: '正在断开…', tone: 'warning' as const }
  return { label: '未连接', tone: 'neutral' as const }
})

const handleSelectProfile = (id: string) => {
  profilesStore.selectProfile(id)
  if (isCreating.value) {
    void router.push({ name: ROUTE_NAMES.workspace })
  }
}

const handleCreateProfile = () => {
  void router.push({ name: ROUTE_NAMES.profileCreate })
}

const handleOpenAbout = () => {
  systemStore.openAboutDialog()
}
</script>

<template>
  <div class="flex h-screen w-screen flex-col overflow-hidden bg-background text-foreground">
    <DesktopChrome
      :platform="desktopWin.platform"
      title="TunnelKit"
      :status-label="globalStatus.label"
      :status-tone="globalStatus.tone"
      :status-pulse="
        vpnStore.state.status === 'connecting' || vpnStore.state.status === 'disconnecting'
      "
      :is-focused="desktopWin.isFocused"
      :is-maximised="desktopWin.isMaximised"
      :is-fullscreen="desktopWin.isFullscreen"
      :theme-mode="themeStore.theme"
      :has-update="systemStore.hasUpdate"
      @minimise="desktopWin.minimise()"
      @toggle-maximise="desktopWin.toggleMaximise()"
      @close="desktopWin.close()"
      @cycle-theme="themeStore.cycleTheme()"
      @open-about="handleOpenAbout"
    />

    <div class="flex min-h-0 flex-1 overflow-hidden">
      <AppSidebar
        :profiles="profilesStore.profiles"
        :selected-id="profilesStore.selectedId"
        :vpn-state="vpnStore.state"
        :is-creating="isCreating"
        :busy="vpnStore.busyAction"
        :pending-action="vpnStore.pendingAction"
        :canceling="vpnStore.cancelingConnect"
        :deleting-profile-id="profilesStore.deletingProfileId"
        @select-profile="handleSelectProfile"
        @create-profile="handleCreateProfile"
        @connect="vpnStore.requestConnect"
        @cancel="vpnStore.cancel"
        @disconnect="vpnStore.disconnect"
        @delete="profilesStore.deleteProfile"
      />

      <main class="min-h-0 min-w-0 flex-1 overflow-hidden bg-background">
        <RouterView />
      </main>
    </div>

    <VpnChallengeDialog
      :prompt="vpnStore.state.prompt"
      :submitting="vpnStore.answeringPrompt"
      :canceling="vpnStore.cancelingPrompt"
      :submit-error="vpnStore.promptError"
      @submit="(code, id) => vpnStore.answerChallenge(code, false, id)"
      @cancel="(id) => vpnStore.answerChallenge('', true, id)"
      @open-saml="vpnStore.openSamlPrompt"
    />

    <ConfirmDialog
      v-model:open="vpnStore.switchConfirmOpen"
      title="切换连接"
      :description="`当前已连接到「${vpnStore.state.profileName || '其他连接'}」，确认后将先断开当前连接，再连接到「${vpnStore.pendingSwitchProfileName}」。`"
      variant="default"
      confirm-test-id="vpn-confirm-switch-btn"
      @confirm="vpnStore.confirmSwitchConnect()"
      @cancel="vpnStore.cancelSwitchConnect()"
    />

    <UpdateDialog
      v-model:open="systemStore.updateDialogOpen"
      :update-info="systemStore.updateInfo"
      :progress="systemStore.updateProgress"
      :opening="systemStore.isOpeningRelease"
      :applying="systemStore.isApplyingUpdate"
      @apply-update="systemStore.applyUpdate()"
      @open-release="(url) => systemStore.openReleasePage(url)"
      @dismiss-version="(ver) => systemStore.dismissUpdate(ver)"
    />

    <AboutDialog
      v-model:open="systemStore.aboutDialogOpen"
      :system-info="systemStore.systemInfo"
      :app-version="systemStore.appVersion"
      :update-info="systemStore.updateInfo"
      :checking-update="systemStore.isCheckingUpdate"
      :checking-system="systemStore.checkingSystem"
      :up-to-date="systemStore.updateUpToDate"
      @check-system="systemStore.loadSystemInfo()"
      @check-update="systemStore.checkUpdate({ silent: false })"
      @show-update="systemStore.openUpdateFromAbout()"
      @open-repo="systemStore.openRepository()"
    />

    <NoticeToast />
  </div>
</template>
