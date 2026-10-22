<script setup lang="ts">
import { onBeforeUnmount, onMounted, watch } from 'vue'
import { RefreshCw, Terminal, Trash2 } from 'lucide-vue-next'
import type { VpnLogView } from '@/api/vpn'
import { formatClockTime } from '@/lib/format'
import CopyButton from '@/components/common/CopyButton.vue'
import { Button } from '@/components/ui/button'

const POLL_INTERVAL_MS = 2000

const props = defineProps<{
  profileId: string
  logView: VpnLogView | null
  loading: boolean
  autoRefresh: boolean
}>()

const emit = defineEmits<{
  refresh: [silent?: boolean]
  clear: []
}>()

let pollTimer: ReturnType<typeof setInterval> | null = null

const syncPolling = () => {
  if (pollTimer) clearInterval(pollTimer)
  pollTimer = props.autoRefresh ? setInterval(() => emit('refresh', true), POLL_INTERVAL_MS) : null
}

onMounted(() => {
  emit('refresh')
  syncPolling()
})
onBeforeUnmount(() => {
  if (pollTimer) clearInterval(pollTimer)
})
watch(
  () => props.profileId,
  () => emit('refresh'),
)
watch(() => props.autoRefresh, syncPolling)
</script>

<template>
  <div class="surface-card flex h-full flex-col overflow-hidden rounded-2xl">
    <!-- 终端顶部工具栏 -->
    <div
      class="flex items-center justify-between gap-2 border-b border-border bg-muted/45 px-4 py-2.5"
    >
      <div class="flex min-w-0 items-center gap-2">
        <Terminal class="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
        <span class="text-caption font-medium text-foreground">连接日志</span>
        <span class="text-caption text-muted-foreground tabular-nums">
          ·
          {{
            logView?.exists && logView.updatedAt
              ? `更新于 ${formatClockTime(logView.updatedAt)}`
              : '暂无记录'
          }}
        </span>
        <span
          v-if="autoRefresh"
          class="inline-flex items-center gap-1 rounded-full bg-status-ok/14 px-2 py-0.5 text-[10.5px] font-medium text-status-ok"
        >
          <span class="size-1.5 animate-pulse rounded-full bg-status-ok" aria-hidden="true" />
          实时更新
        </span>
      </div>

      <div class="flex shrink-0 items-center gap-1.5">
        <Button variant="outline" size="xs" :disabled="loading" @click="emit('refresh')">
          <RefreshCw class="size-3" :class="loading ? 'animate-spin' : ''" aria-hidden="true" />
          刷新
        </Button>
        <CopyButton :text="logView?.content ?? ''" />
        <Button
          variant="outline"
          size="xs"
          :disabled="loading || !logView?.content"
          @click="emit('clear')"
        >
          <Trash2 class="size-3" aria-hidden="true" />
          清空
        </Button>
      </div>
    </div>

    <!-- 沉浸式深色终端输出视口 -->
    <div class="relative flex min-h-[240px] flex-1 bg-[#0C101A] text-slate-200">
      <pre
        v-if="logView?.content"
        class="h-full w-full overflow-auto p-4 font-mono text-caption leading-relaxed break-all whitespace-pre-wrap text-slate-200 tabular-nums"
        >{{ logView.content }}</pre>
      <div
        v-else
        class="flex flex-1 flex-col items-center justify-center gap-2 p-6 text-center text-slate-400"
      >
        <Terminal class="size-6 opacity-40" aria-hidden="true" />
        <p class="text-caption">暂无日志，发起连接后将在此实时显示连接输出。</p>
      </div>
    </div>
  </div>
</template>
