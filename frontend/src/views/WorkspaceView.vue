<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useTimeoutFn } from '@vueuse/core'
import HelperMissingBanner from '@/components/common/HelperMissingBanner.vue'
import ProfileCreateView from '@/views/ProfileCreateView.vue'
import { Activity, Check, Loader2, ScrollText, RotateCcw, SlidersHorizontal } from '@lucide/vue'
import LogViewerPanel from '@/components/logs/LogViewerPanel.vue'
import ProfileFormPanel from '@/components/profile/ProfileFormPanel.vue'
import AppButton from '@/components/base/button/AppButton.vue'
import AppTabsList from '@/components/base/AppTabsList.vue'
import AppTabsTrigger from '@/components/base/AppTabsTrigger.vue'
import { Tabs, TabsContent } from '@/components/ui/tabs'
import VpnConnectionHero from '@/components/vpn/VpnConnectionHero.vue'
import VpnSessionMetricsGrid from '@/components/vpn/VpnSessionMetricsGrid.vue'
import { useProfilesStore } from '@/stores/useProfilesStore'
import { useSystemStore } from '@/stores/useSystemStore'
import { useVpnStore } from '@/stores/useVpnStore'
import type { WorkspaceTab } from '@/types/profile'

const profilesStore = useProfilesStore()
const vpnStore = useVpnStore()
const systemStore = useSystemStore()
const profileForm = ref<InstanceType<typeof ProfileFormPanel> | null>(null)
const profileDirty = ref(false)
const profileSaved = ref(false)
const { start: startSavedFeedback } = useTimeoutFn(
  () => {
    profileSaved.value = false
  },
  2000,
  { immediate: false },
)
const markProfileSaved = () => {
  profileSaved.value = true
  startSavedFeedback()
}

const handleProfileDirty = (dirty: boolean) => {
  profileDirty.value = dirty
  if (dirty) profileSaved.value = false
}

const resetProfileDraft = () => {
  profileSaved.value = false
  profileForm.value?.resetDraft()
}

const selectedProfile = computed(() => profilesStore.selectedProfile)

watch(
  () => selectedProfile.value?.id,
  () => {
    profileSaved.value = false
  },
)

const activeTabModel = computed<WorkspaceTab>({
  get: () => profilesStore.activeTab,
  set: (tab) => profilesStore.setActiveTab(tab),
})

const isHelperMissing = computed(() =>
  Boolean(systemStore.systemInfo && !systemStore.systemInfo.helperReady),
)
</script>

<template>
  <div
    class="flex h-full min-h-0 flex-col overflow-hidden"
    :class="selectedProfile || isHelperMissing ? 'pt-4' : ''"
  >
    <HelperMissingBanner
      v-if="isHelperMissing"
      :checking="systemStore.checkingSystem"
      data-testid="helper-missing-banner"
      @recheck="systemStore.loadSystemInfo()"
    />

    <ProfileCreateView v-if="!selectedProfile" />

    <div v-else class="flex min-h-0 flex-1 flex-col">
      <VpnConnectionHero
        :profile="selectedProfile"
        :state="vpnStore.state"
        :elapsed-text="vpnStore.elapsedText"
        :busy="vpnStore.busyAction"
        :pending-action="vpnStore.pendingAction"
        :canceling="vpnStore.cancelingConnect"
        @toggle-connect="vpnStore.toggleProfileConnection(selectedProfile.id)"
        @trust-cert="vpnStore.trustErrorCertAndReconnect()"
      />

      <Tabs
        v-model="activeTabModel"
        :unmount-on-hide="false"
        class="flex min-h-0 flex-1 flex-col gap-0 pt-3.5 pb-5"
      >
        <div class="flex min-h-9 items-center justify-between gap-3 px-6">
          <AppTabsList class="w-fit">
            <AppTabsTrigger value="status" data-testid="tab-trigger-status" class="gap-1.5">
              <Activity class="size-3.5" />
              状态
              <span
                v-if="vpnStore.state.profileId === selectedProfile.id && vpnStore.state.error"
                class="size-1.5 rounded-full bg-tone-danger-solid"
                aria-hidden="true"
              />
            </AppTabsTrigger>
            <AppTabsTrigger value="config" data-testid="tab-trigger-config" class="gap-1.5">
              <SlidersHorizontal class="size-3.5" />
              配置
              <span
                v-if="profileDirty"
                class="size-1.5 rounded-full bg-primary"
                aria-hidden="true"
                title="有未保存的更改"
              />
            </AppTabsTrigger>
            <AppTabsTrigger value="logs" data-testid="tab-trigger-logs" class="gap-1.5">
              <ScrollText class="size-3.5" />
              日志
              <span
                v-if="
                  vpnStore.state.profileId === selectedProfile.id &&
                  ['connecting', 'connected'].includes(vpnStore.state.status)
                "
                class="size-1.5 animate-pulse rounded-full bg-tone-success-solid"
                aria-hidden="true"
              />
            </AppTabsTrigger>
          </AppTabsList>

          <div class="relative flex items-center pr-5">
            <Transition
              enter-active-class="transition-[opacity,translate] duration-150 ease-out"
              enter-from-class="opacity-0 translate-y-0.5"
              leave-active-class="absolute right-5 transition-[opacity,translate] duration-150 ease-out"
              leave-to-class="opacity-0 translate-y-0.5"
            >
              <div
                v-if="activeTabModel === 'config' && profileDirty"
                key="dirty"
                class="flex items-center gap-2"
              >
                <AppButton
                  variant="outline"
                  size="sm"
                  :disabled="profileForm?.saving"
                  data-testid="profile-reset-btn"
                  @click="resetProfileDraft"
                >
                  <RotateCcw class="size-3.5" />还原
                </AppButton>
                <AppButton
                  size="sm"
                  :disabled="profileForm?.saving"
                  data-testid="profile-save-btn"
                  class="min-w-[88px]"
                  title="保存配置 (⌘S / Ctrl+S)"
                  @click="profileForm?.submitDraft()"
                >
                  <Loader2 v-if="profileForm?.saving" class="size-3.5 animate-spin" /><Check
                    v-else
                    class="size-3.5"
                  />保存
                </AppButton>
              </div>
              <span
                v-else-if="activeTabModel === 'config' && profileSaved"
                key="saved"
                class="inline-flex items-center gap-1 rounded-full bg-tone-success-solid/14 px-2.5 py-1 text-caption font-medium text-tone-success-solid"
                ><Check class="size-3.5" aria-hidden="true" />已保存</span
              >
            </Transition>
          </div>
        </div>

        <TabsContent
          value="status"
          class="text-inherit min-h-0 overflow-y-auto px-6 pt-3 pb-1 outline-none"
        >
          <VpnSessionMetricsGrid
            :profile="selectedProfile"
            :state="vpnStore.state"
            :elapsed-text="vpnStore.elapsedText"
          />
        </TabsContent>

        <TabsContent
          value="config"
          class="text-inherit min-h-0 overflow-y-auto px-6 pt-3 pb-1 outline-none"
        >
          <ProfileFormPanel
            :key="selectedProfile.id"
            ref="profileForm"
            :profile="selectedProfile"
            @update:dirty="handleProfileDirty"
            @saved="markProfileSaved"
          />
        </TabsContent>

        <TabsContent
          value="logs"
          class="text-inherit flex min-h-0 flex-1 flex-col overflow-hidden px-6 pt-3 outline-none"
        >
          <LogViewerPanel :profile-id="selectedProfile.id" :active="activeTabModel === 'logs'" />
        </TabsContent>
      </Tabs>
    </div>
  </div>
</template>
