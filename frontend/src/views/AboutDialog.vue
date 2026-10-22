<script setup lang="ts">
import { RefreshCw, Shield } from 'lucide-vue-next'
import type { SystemInfo } from '@/api/system'
import StatusDot from '@/components/common/StatusDot.vue'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'

defineProps<{
  systemInfo: SystemInfo | null
  checking: boolean
}>()

const open = defineModel<boolean>('open', { required: true })

const emit = defineEmits<{
  recheck: []
}>()
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent>
      <div class="flex items-center gap-3.5">
        <div
          class="flex size-11 shrink-0 items-center justify-center rounded-xl bg-primary text-primary-foreground shadow-[0_2px_8px_color-mix(in_oklch,var(--primary)_35%,transparent)]"
        >
          <Shield class="size-5" aria-hidden="true" />
        </div>
        <div>
          <div class="flex items-center gap-2">
            <DialogTitle>TunnelKit</DialogTitle>
            <span
              v-if="systemInfo?.appVersion"
              class="rounded-md bg-muted px-2 py-0.5 font-mono text-caption text-muted-foreground tabular-nums"
            >
              v{{ systemInfo.appVersion }}
            </span>
          </div>
          <DialogDescription class="mt-0.5"> 跨平台 Fortinet SSL-VPN 桌面客户端 </DialogDescription>
        </div>
      </div>

      <dl
        class="mt-1 min-w-0 divide-y divide-border overflow-hidden rounded-xl border border-border bg-muted/35 text-caption"
      >
        <div class="flex items-center justify-between gap-4 px-4 py-3">
          <dt class="shrink-0 text-muted-foreground">协议引擎</dt>
          <dd class="min-w-0 truncate font-medium text-foreground">fortivpn 原生引擎</dd>
        </div>
        <div class="flex items-center justify-between gap-4 px-4 py-3">
          <dt class="shrink-0 text-muted-foreground">后台隧道服务</dt>
          <dd
            class="inline-flex min-w-0 items-center gap-1.5 font-medium text-foreground"
            :title="systemInfo?.helperPath || undefined"
          >
            <StatusDot :tone="systemInfo?.helperReady ? 'ok' : 'danger'" />
            <span class="truncate">{{ systemInfo?.helperReady ? '已就绪' : '未就绪' }}</span>
          </dd>
        </div>
        <div class="flex items-center justify-between gap-4 px-4 py-3">
          <dt class="shrink-0 text-muted-foreground">配置与日志目录</dt>
          <dd class="min-w-0 truncate font-mono text-foreground">~/.tunnelkit/</dd>
        </div>
      </dl>

      <div v-if="systemInfo && !systemInfo.helperReady" class="flex justify-end">
        <Button variant="outline" size="sm" :disabled="checking" @click="emit('recheck')">
          <RefreshCw class="size-3.5" :class="checking ? 'animate-spin' : ''" aria-hidden="true" />
          重新检测
        </Button>
      </div>
    </DialogContent>
  </Dialog>
</template>
