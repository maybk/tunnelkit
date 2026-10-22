<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Info } from 'lucide-vue-next'
import { windowCaptionRegionStyle, windowInteractiveRegionStyle } from '@/api/window'
import { useDesktopWindow } from '@/stores/useDesktopWindow'
import { useNotice } from '@/stores/useNotice'
import { useProfiles } from '@/stores/useProfiles'
import { useSystem } from '@/stores/useSystem'
import { useVpn } from '@/stores/useVpn'
import AppSidebar from '@/components/sidebar/AppSidebar.vue'
import HelperMissingBanner from '@/components/common/HelperMissingBanner.vue'
import DesktopChrome from '@/components/common/DesktopChrome.vue'
import NoticeToast from '@/components/common/NoticeToast.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import PromptDialog from '@/components/connection/PromptDialog.vue'
import { Button } from '@/components/ui/button'
import AboutDialog from '@/views/AboutDialog.vue'
import CreateProfileView from '@/views/CreateProfileView.vue'
import ProfileView from '@/views/ProfileView.vue'

const desktopWindow = useDesktopWindow()
const notice = useNotice()
const system = useSystem()
const profiles = useProfiles()
const vpn = useVpn()

const { systemInfo, checking } = system
const { selectedProfile, creating, activeTab } = profiles
const { state: vpnState } = vpn

const ready = ref(false)
const aboutOpen = ref(false)
const helperReady = computed(() => Boolean(systemInfo.value?.helperReady))

const globalStatus = computed(() => {
  const name = vpnState.value.profileName
  if (vpnState.value.status === 'connected') {
    return { label: name ? `已连接 · ${name}` : '已连接', tone: 'ok' as const }
  }
  if (vpnState.value.status === 'connecting') {
    return { label: name ? `正在连接 · ${name}` : '正在连接…', tone: 'busy' as const }
  }
  if (vpnState.value.status === 'disconnecting') {
    return { label: '正在断开…', tone: 'busy' as const }
  }
  return { label: '未连接', tone: 'idle' as const }
})

const recheckSystem = () => system.refreshSystem().catch(notice.showError)

onMounted(async () => {
  desktopWindow.startWatching()
  vpn.startWatching()
  const results = await Promise.allSettled([
    system.refreshSystem(),
    profiles.loadProfiles(),
    vpn.loadState(),
  ])
  results.forEach((r) => r.status === 'rejected' && notice.showError(r.reason))

  const activeId = vpnState.value.profileId
  if (activeId && profiles.profiles.value.some((p) => p.id === activeId)) {
    profiles.selectProfile(activeId)
  }
  ready.value = true
})

onBeforeUnmount(() => {
  desktopWindow.stopWatching()
  vpn.stopWatching()
})

const withSelected = (fn: (id: string) => unknown) => {
  const id = selectedProfile.value?.id
  if (id) fn(id)
}

const trustAndReconnect = (sha256: string) =>
  withSelected(async (id) => {
    if (await profiles.trustCert(id, sha256)) await vpn.connect(id)
  })
</script>

<template>
  <div
    :data-desktop-platform="desktopWindow.platform.value"
    class="flex h-screen w-screen flex-col overflow-hidden bg-background text-foreground"
  >
    <DesktopChrome
      :platform="desktopWindow.platform.value"
      title="TunnelKit"
      :is-focused="desktopWindow.isFocused.value"
      :is-maximised="desktopWindow.isMaximised.value"
      :is-fullscreen="desktopWindow.isFullscreen.value"
      @minimise="desktopWindow.minimise"
      @toggle-maximise="desktopWindow.toggleMaximise"
      @close="desktopWindow.close"
    >
      <div
        class="flex items-center self-stretch"
        :class="
          desktopWindow.platform.value === 'macos' && !desktopWindow.isFullscreen.value
            ? 'pl-1'
            : 'pl-4'
        "
        :style="windowCaptionRegionStyle"
      >
        <span class="text-body font-semibold tracking-tight text-foreground/90">TunnelKit</span>
      </div>

      <template #actions>
        <span
          class="inline-flex items-center gap-1.5 rounded-full bg-foreground/[0.05] px-2.5 py-1 text-caption font-medium text-muted-foreground select-none"
          :style="windowCaptionRegionStyle"
        >
          <StatusDot :tone="globalStatus.tone" />
          <span>{{ globalStatus.label }}</span>
        </span>
        <Button
          variant="ghost"
          size="icon-xs"
          title="关于 TunnelKit"
          aria-label="关于 TunnelKit"
          class="text-muted-foreground hover:text-foreground"
          :style="windowInteractiveRegionStyle"
          @click="aboutOpen = true"
        >
          <Info class="size-4" aria-hidden="true" />
        </Button>
      </template>
    </DesktopChrome>

    <div class="flex min-h-0 flex-1 overflow-hidden">
      <AppSidebar
        :profiles="profiles.profiles.value"
        :selected-id="profiles.selectedId.value"
        :creating="creating"
        :vpn-state="vpnState"
        :pending="vpn.pending.value"
        :helper-ready="helperReady"
        :deleting="profiles.deleting.value"
        :saving="profiles.saving.value"
        @select="profiles.selectProfile"
        @create="profiles.startCreate"
        @connect="vpn.connect"
        @cancel="vpn.cancel"
        @disconnect="vpn.disconnect"
        @delete="profiles.remove"
      />

      <main v-if="ready" class="flex min-w-0 flex-1 flex-col overflow-hidden pt-4">
        <HelperMissingBanner
          v-if="!helperReady"
          :system-info="systemInfo"
          :checking="checking"
          @recheck="recheckSystem"
        />

        <div class="min-h-0 flex-1">
          <ProfileView
            v-if="selectedProfile && !creating"
            v-model:active-tab="activeTab"
            :profile="selectedProfile"
            :vpn-state="vpnState"
            :pending="vpn.pending.value"
            :now-ms="vpn.nowMs.value"
            :helper-ready="helperReady"
            :saving="profiles.saving.value"
            :deleting="profiles.deleting.value"
            :form-error="profiles.formError.value"
            :log-view="vpn.logView.value"
            :log-loading="vpn.logLoading.value"
            @connect="vpn.connect"
            @cancel="vpn.cancel"
            @disconnect="vpn.disconnect"
            @trust-and-reconnect="trustAndReconnect"
            @save-profile="profiles.save"
            @reset-form-error="profiles.clearFormError"
            @refresh-log="withSelected(vpn.loadLog)"
            @clear-log="withSelected(vpn.clearLog)"
          />
          <CreateProfileView
            v-else
            :first-profile="profiles.profiles.value.length === 0"
            :saving="profiles.saving.value"
            :form-error="profiles.formError.value"
            @save-profile="profiles.save"
            @cancel="profiles.cancelCreate"
          />
        </div>
      </main>
    </div>

    <PromptDialog
      :prompt="vpnState.prompt"
      :busy="vpn.answering.value"
      @submit="(id, val) => vpn.answerPrompt(id, val, false)"
      @cancel="(id) => vpn.answerPrompt(id, '', true)"
    />

    <AboutDialog
      v-model:open="aboutOpen"
      :system-info="systemInfo"
      :checking="checking"
      @recheck="recheckSystem"
    />
    <NoticeToast :message="notice.message.value" @dismiss="notice.dismiss" />
  </div>
</template>
