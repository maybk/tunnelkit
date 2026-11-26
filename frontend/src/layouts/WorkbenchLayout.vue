<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import NoticeToast from '@/components/common/NoticeToast.vue'
import UpdateDialog from '@/components/common/UpdateDialog.vue'
import VpnChallengeDialog from '@/components/vpn/VpnChallengeDialog.vue'
import { useGlobalShortcuts } from '@/composables/useGlobalShortcuts'
import AppSidebar from '@/layouts/AppSidebar.vue'
import DesktopChrome from '@/layouts/DesktopChrome.vue'
import { ROUTE_NAMES } from '@/router'
import { useDesktopWindowStore } from '@/stores/useDesktopWindowStore'
import { useProfilesStore } from '@/stores/useProfilesStore'
import { useSystemStore } from '@/stores/useSystemStore'
import { useVpnStore } from '@/stores/useVpnStore'

const route = useRoute()
const router = useRouter()
const desktopWin = useDesktopWindowStore()
const profilesStore = useProfilesStore()
const vpnStore = useVpnStore()
const systemStore = useSystemStore()

useGlobalShortcuts()

const isSettings = computed(() => Boolean(route.meta.settings))
const isCreating = computed(() => route.name === ROUTE_NAMES.profileCreate)

const chromeTitle = computed(() => {
  if (isSettings.value) return route.meta.title ?? '通用'
  if (isCreating.value) return route.meta.title ?? '新建连接'
  if (profilesStore.selectedProfile) return profilesStore.selectedProfile.name
  if (profilesStore.profiles.length === 0) return '新建连接'
  return ''
})

const chromeParentTitle = computed(() => (isSettings.value ? '设置' : undefined))

const handleSelectProfile = (id: string) => {
  profilesStore.selectProfile(id)
  if (isCreating.value) {
    void router.push({ name: ROUTE_NAMES.workspace })
  }
}

const handleCreateProfile = () => {
  void router.push({ name: ROUTE_NAMES.profileCreate })
}

const handleOpenSettings = () => {
  void router.push({ name: ROUTE_NAMES.settings })
}
</script>

<template>
  <div class="flex h-screen w-screen flex-col overflow-hidden bg-background text-foreground">
    <DesktopChrome
      :platform="desktopWin.platform"
      brand-title="TunnelKit"
      :title="chromeTitle"
      :parent-title="chromeParentTitle"
      :narrow-rail="isSettings"
      :is-focused="desktopWin.isFocused"
      :is-maximised="desktopWin.isMaximised"
      :is-fullscreen="desktopWin.isFullscreen"
      @minimise="desktopWin.minimise()"
      @toggle-maximise="desktopWin.toggleMaximise()"
      @close="desktopWin.close()"
    />

    <div class="flex min-h-0 flex-1 overflow-hidden">
      <AppSidebar
        v-if="!isSettings"
        :profiles="profilesStore.profiles"
        :selected-id="profilesStore.selectedId"
        :vpn-state="vpnStore.state"
        :platform="desktopWin.platform"
        :is-creating="isCreating"
        :has-update="systemStore.hasUpdate"
        :busy="vpnStore.busyAction"
        :pending-action="vpnStore.pendingAction"
        :canceling="vpnStore.cancelingConnect"
        :deleting-profile-id="profilesStore.deletingProfileId"
        @select-profile="handleSelectProfile"
        @create-profile="handleCreateProfile"
        @open-settings="handleOpenSettings"
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
      :phase="systemStore.updatePhase"
      :progress="systemStore.updateProgress"
      :opening="systemStore.isOpeningRelease"
      :canceling="systemStore.isCancelingUpdate"
      :vpn-active="vpnStore.state.status !== 'disconnected'"
      @download-update="systemStore.downloadUpdate()"
      @cancel-update="systemStore.cancelUpdate()"
      @install-update="systemStore.installUpdate()"
      @defer-install="systemStore.deferInstall()"
      @open-release="(url) => systemStore.openReleasePage(url)"
      @dismiss-version="(ver) => systemStore.dismissUpdate(ver)"
    />

    <NoticeToast />
  </div>
</template>
