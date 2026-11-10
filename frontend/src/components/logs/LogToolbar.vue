<script setup lang="ts">
import { Copy, RefreshCw, Terminal, Trash2 } from '@lucide/vue'
import AppButton from '@/components/base/button/AppButton.vue'

withDefaults(
  defineProps<{
    autoRefresh?: boolean
    totalLines: number
    updatedAt: string
    loading?: boolean
  }>(),
  {
    loading: false,
  },
)

const emit = defineEmits<{
  refresh: []
  clear: []
  'copy-all': []
}>()
</script>

<template>
  <div
    class="flex flex-wrap items-center justify-between gap-3 border-b border-border bg-muted/45 px-4 py-2.5"
  >
    <div class="flex min-w-0 items-center gap-2 text-caption">
      <Terminal class="size-3.5 shrink-0 text-muted-foreground" />
      <span class="font-medium text-foreground">连接日志</span>
      <span class="truncate text-caption text-muted-foreground tabular-nums">
        · {{ updatedAt ? `更新于 ${updatedAt}` : '暂无记录' }}
      </span>
      <span
        v-if="autoRefresh"
        class="inline-flex items-center gap-1 rounded-full bg-tone-success-solid/14 px-2 py-0.5 text-[10.5px] font-medium text-tone-success-solid"
        ><span
          class="size-1.5 animate-pulse rounded-full bg-tone-success-solid"
          aria-hidden="true"
        />实时更新</span
      >
    </div>

    <div class="flex items-center gap-1.5">
      <AppButton variant="outline" size="xs" :disabled="loading" @click="emit('refresh')">
        <RefreshCw class="size-3.5" :class="{ 'animate-spin': loading }" />
        刷新
      </AppButton>

      <AppButton
        variant="outline"
        size="xs"
        :disabled="totalLines === 0"
        data-testid="log-copy-btn"
        @click="emit('copy-all')"
      >
        <Copy class="size-3.5" />
        复制
      </AppButton>

      <AppButton
        variant="outline"
        size="xs"
        :disabled="totalLines === 0"
        data-testid="log-clear-btn"
        @click="emit('clear')"
      >
        <Trash2 class="size-3.5" />
        清空
      </AppButton>
    </div>
  </div>
</template>
