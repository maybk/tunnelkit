<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Activity, Check, Loader2, RotateCcw, ScrollText, SlidersHorizontal } from 'lucide-vue-next'
import type { Profile, ProfileInput } from '@/api/profile'
import type { VpnLogView, VpnState } from '@/api/vpn'
import type { DetailTab } from '@/stores/useProfiles'
import { profileStatus } from '@/lib/status'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import ConnectionHeader from '@/components/connection/ConnectionHeader.vue'
import StatusPanel from '@/components/connection/StatusPanel.vue'
import LogPanel from '@/components/connection/LogPanel.vue'
import ProfileForm from '@/components/profile/ProfileForm.vue'

const props = defineProps<{
  profile: Profile
  vpnState: VpnState
  pending: boolean
  nowMs: number
  helperReady: boolean
  saving: boolean
  deleting: boolean
  formError: string
  logView: VpnLogView | null
  logLoading: boolean
}>()

const activeTab = defineModel<DetailTab>('activeTab', { required: true })

const emit = defineEmits<{
  connect: [id: string]
  cancel: []
  disconnect: []
  trustAndReconnect: [sha256: string]
  saveProfile: [input: ProfileInput]
  resetFormError: []
  refreshLog: [silent?: boolean]
  clearLog: []
}>()

const profileFormRef = ref<InstanceType<typeof ProfileForm> | null>(null)
const configDirty = ref(false)
const configJustSaved = ref(false)

watch(
  () => props.profile.id,
  () => {
    configDirty.value = false
    configJustSaved.value = false
  },
)

const view = computed(() => profileStatus(props.profile.id, props.vpnState))
const status = computed(() => view.value.status)
const logLive = computed(() => status.value === 'connecting' || status.value === 'connected')
</script>

<template>
  <div class="flex h-full flex-col overflow-hidden">
    <ConnectionHeader
      :profile="profile"
      :vpn-state="vpnState"
      :pending="pending"
      :now-ms="nowMs"
      :helper-ready="helperReady"
      @connect="emit('connect', $event)"
      @cancel="emit('cancel')"
      @disconnect="emit('disconnect')"
    />

    <Tabs
      :model-value="activeTab"
      class="flex min-h-0 flex-1 flex-col pt-3.5 pb-5"
      @update:model-value="activeTab = $event as DetailTab"
    >
      <div class="flex min-h-9 items-center justify-between gap-3 px-6">
        <TabsList>
          <TabsTrigger value="status">
            <Activity class="size-3.5" aria-hidden="true" />
            状态
            <span
              v-if="view.error"
              class="size-1.5 rounded-full bg-status-danger"
              aria-hidden="true"
            />
          </TabsTrigger>
          <TabsTrigger value="config">
            <SlidersHorizontal class="size-3.5" aria-hidden="true" />
            配置
            <span
              v-if="configDirty"
              class="size-1.5 rounded-full bg-primary"
              aria-hidden="true"
              title="有未保存的更改"
            />
          </TabsTrigger>
          <TabsTrigger value="logs">
            <ScrollText class="size-3.5" aria-hidden="true" />
            日志
            <span
              v-if="logLive"
              class="size-1.5 animate-pulse rounded-full bg-status-ok"
              aria-hidden="true"
            />
          </TabsTrigger>
        </TabsList>

        <div class="flex items-center pr-5">
          <Transition
            enter-active-class="transition-[opacity,transform] duration-150 ease-out"
            enter-from-class="opacity-0 translate-y-0.5"
            enter-to-class="opacity-100 translate-y-0"
            leave-active-class="transition-[opacity,transform] duration-150 ease-out"
            leave-from-class="opacity-100 translate-y-0"
            leave-to-class="opacity-0 translate-y-0.5"
            mode="out-in"
          >
            <div
              v-if="activeTab === 'config' && configDirty"
              key="dirty-actions"
              class="flex items-center gap-2"
            >
              <Button
                type="button"
                variant="outline"
                size="sm"
                data-testid="tab-reset-profile"
                :disabled="saving || deleting"
                @click="profileFormRef?.reset()"
              >
                <RotateCcw class="size-3.5" aria-hidden="true" />
                还原
              </Button>
              <Button
                type="button"
                size="sm"
                data-testid="tab-save-profile"
                class="min-w-[88px]"
                :disabled="saving || deleting"
                title="保存配置 (⌘S / Ctrl+S)"
                @click="profileFormRef?.submit()"
              >
                <Loader2 v-if="saving" class="size-3.5 animate-spin" aria-hidden="true" />
                <Check v-else class="size-3.5" aria-hidden="true" />
                保存
              </Button>
            </div>
            <span
              v-else-if="activeTab === 'config' && configJustSaved"
              key="saved-badge"
              class="inline-flex items-center gap-1 rounded-full bg-status-ok/14 px-2.5 py-1 text-caption font-medium text-status-ok"
            >
              <Check class="size-3.5" aria-hidden="true" />
              已保存
            </span>
          </Transition>
        </div>
      </div>

      <TabsContent value="status" class="min-h-0 overflow-y-auto px-6 pt-3 pb-1">
        <StatusPanel
          :profile="profile"
          :vpn-state="vpnState"
          :pending="pending"
          :now-ms="nowMs"
          @trust-and-reconnect="emit('trustAndReconnect', $event)"
        />
      </TabsContent>

      <TabsContent
        value="config"
        :unmount-on-hide="false"
        class="min-h-0 overflow-y-auto px-6 pt-3 pb-1"
      >
        <ProfileForm
          ref="profileFormRef"
          v-model:dirty="configDirty"
          v-model:just-saved="configJustSaved"
          :profile="profile"
          :saving="saving"
          :deleting="deleting"
          :error="formError"
          hide-actions
          @save="emit('saveProfile', $event)"
          @reset="emit('resetFormError')"
        />
      </TabsContent>

      <TabsContent value="logs" class="flex min-h-0 flex-1 flex-col px-6 pt-3">
        <LogPanel
          :profile-id="profile.id"
          :log-view="logView"
          :loading="logLoading"
          :auto-refresh="logLive"
          @refresh="emit('refreshLog', $event)"
          @clear="emit('clearLog')"
        />
      </TabsContent>
    </Tabs>
  </div>
</template>
