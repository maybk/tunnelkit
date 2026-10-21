<script setup lang="ts">
import { RefreshCw, ShieldAlert } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import CopyButton from './CopyButton.vue'

defineProps<{
  checking: boolean
  helperPath?: string
}>()

const emit = defineEmits<{
  recheck: []
}>()

const buildCommand = 'go build -o bin/tunnelkit-helper ./cmd/tunnelkit-helper'
</script>

<template>
  <div
    class="mx-6 mb-3 flex items-center gap-3 rounded-xl border border-status-busy/35 bg-status-busy/10 px-4 py-2.5 text-body shadow-2xs"
  >
    <div
      class="flex size-7 shrink-0 items-center justify-center rounded-lg bg-status-busy/18 text-status-busy"
    >
      <ShieldAlert class="size-4" aria-hidden="true" />
    </div>
    <div class="min-w-0 flex-1">
      <span class="font-medium text-foreground">未检测到内置隧道服务组件 tunnelkit-helper</span>
      <span class="text-muted-foreground"> · 开发环境下请先编译：</span>
      <code
        class="ml-1 rounded-md bg-card/85 px-2 py-0.5 font-mono text-caption text-foreground ring-1 ring-black/6 dark:ring-white/10"
      >
        {{ buildCommand }}
      </code>
    </div>
    <div class="app-no-drag flex shrink-0 items-center gap-1.5">
      <CopyButton :text="buildCommand" />
      <Button variant="outline" size="xs" :disabled="checking" @click="emit('recheck')">
        <RefreshCw class="size-3" :class="checking ? 'animate-spin' : ''" aria-hidden="true" />
        重新检测
      </Button>
    </div>
  </div>
</template>
