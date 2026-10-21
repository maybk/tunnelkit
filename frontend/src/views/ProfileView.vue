<script setup lang="ts">
import { computed } from 'vue'
import { Activity, ScrollText, SlidersHorizontal } from 'lucide-vue-next'
import type { Profile, ProfileInput } from '@/api/profile'
import type { VpnLogView, VpnState } from '@/api/vpn'
import type { DetailTab } from '@/stores/useProfiles'
import { profileStatus } from '@/lib/status'
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
  deleteProfile: [id: string]
  refreshLog: []
  clearLog: []
}>()

const view = computed(() => profileStatus(props.profile.id, props.vpnState))
const status = computed(() => view.value.status)
const inUse = computed(() => status.value !== 'disconnected')
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
      class="flex min-h-0 flex-1 flex-col px-6 pt-3.5 pb-5"
      @update:model-value="activeTab = $event as DetailTab"
    >
      <TabsList class="self-start">
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

      <TabsContent value="status" class="min-h-0 overflow-y-auto pt-3 pr-0.5">
        <StatusPanel
          :profile="profile"
          :vpn-state="vpnState"
          :pending="pending"
          :now-ms="nowMs"
          @trust-and-reconnect="emit('trustAndReconnect', $event)"
        />
      </TabsContent>

      <TabsContent value="config" class="min-h-0 overflow-y-auto pt-3 pr-0.5">
        <ProfileForm
          :profile="profile"
          :saving="saving"
          :deleting="deleting"
          :error="formError"
          :delete-disabled="inUse"
          @save="emit('saveProfile', $event)"
          @delete="emit('deleteProfile', $event)"
        />
      </TabsContent>

      <TabsContent value="logs" class="flex min-h-0 flex-1 flex-col pt-3">
        <LogPanel
          :profile-id="profile.id"
          :log-view="logView"
          :loading="logLoading"
          :auto-refresh="logLive"
          @refresh="emit('refreshLog')"
          @clear="emit('clearLog')"
        />
      </TabsContent>
    </Tabs>
  </div>
</template>
