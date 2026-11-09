<script setup lang="ts">
import { computed, onMounted, onUnmounted, watch } from 'vue'
import { LOG_POLL_INTERVAL_MS } from '@/constants/log'
import { useVpnStore } from '@/stores/useVpnStore'
import { useLogsStore } from '@/stores/useLogsStore'
import { formatClockTime } from '@/utils/time'
import LogStreamList from './LogStreamList.vue'
import LogToolbar from './LogToolbar.vue'

const props = defineProps<{
  profileId: string
  active?: boolean
}>()

const logsStore = useLogsStore()
const vpnStore = useVpnStore()
const autoRefresh = computed(
  () =>
    vpnStore.state.profileId === props.profileId &&
    ['connecting', 'connected'].includes(vpnStore.state.status),
)
const content = computed(() => logsStore.contentForProfile(props.profileId))
const updatedAt = computed(() => {
  const value = logsStore.updatedAtForProfile(props.profileId)
  return value ? formatClockTime(value) : ''
})
const lines = computed(() => (content.value ? content.value.split('\n').length : 0))

let pollTimer: ReturnType<typeof setInterval> | null = null

const startPolling = () => {
  if (pollTimer) clearInterval(pollTimer)
  pollTimer = null
  if (props.active === false) return
  void logsStore.fetchLog(props.profileId, true)
  if (autoRefresh.value) {
    pollTimer = setInterval(() => {
      void logsStore.fetchLog(props.profileId, true)
    }, LOG_POLL_INTERVAL_MS)
  }
}

watch(
  () => [props.profileId, props.active, autoRefresh.value],
  () => startPolling(),
)

onMounted(() => {
  startPolling()
})

onUnmounted(() => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
})
</script>

<template>
  <div class="surface-card flex h-full min-h-0 flex-col overflow-hidden rounded-2xl">
    <LogToolbar
      :auto-refresh="autoRefresh"
      :total-lines="lines"
      :updated-at="updatedAt"
      :loading="logsStore.loading"
      @refresh="logsStore.fetchLog(profileId)"
      @copy-all="logsStore.copyProfileLogs(profileId)"
      @clear="logsStore.clearLog(profileId)"
    />

    <LogStreamList :content="content" :auto-scroll="autoRefresh" />
  </div>
</template>
