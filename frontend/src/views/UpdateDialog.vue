<script setup lang="ts">
import { ArrowRight, ExternalLink } from 'lucide-vue-next'
import type { UpdateInfo } from '@/api/system'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'

const props = defineProps<{
  updateInfo: UpdateInfo | null
  opening?: boolean
}>()

const open = defineModel<boolean>('open', { required: true })

const emit = defineEmits<{
  openRelease: [url: string]
  dismissVersion: [version: string]
}>()

const handleOpenRelease = () => {
  if (!props.updateInfo?.releaseUrl) return
  emit('openRelease', props.updateInfo.releaseUrl)
}

const handleDismiss = () => {
  if (!props.updateInfo?.latestVersion) {
    open.value = false
    return
  }
  emit('dismissVersion', props.updateInfo.latestVersion)
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent>
      <div class="flex items-center gap-3.5">
        <img
          src="/favicon.svg"
          alt="TunnelKit"
          class="size-11 shrink-0 rounded-xl shadow-xs ring-1 ring-black/6 dark:ring-white/10"
        />
        <div>
          <DialogTitle>发现新版本</DialogTitle>
          <DialogDescription class="mt-0.5">
            检测到新版本 v{{ updateInfo?.latestVersion }} 可供下载
          </DialogDescription>
        </div>
      </div>

      <div
        v-if="updateInfo"
        class="mt-1 flex items-center justify-between gap-3 rounded-xl border border-border bg-muted/35 px-4 py-3 text-caption"
      >
        <span class="text-muted-foreground">版本更新</span>
        <div class="inline-flex items-center gap-2 font-mono tabular-nums">
          <span class="text-muted-foreground">v{{ updateInfo.currentVersion }}</span>
          <ArrowRight class="size-3.5 text-muted-foreground/70" aria-hidden="true" />
          <span
            class="rounded-md bg-primary/12 px-2 py-0.5 font-semibold text-primary"
            data-testid="update-latest-version"
          >
            v{{ updateInfo.latestVersion }}
          </span>
        </div>
      </div>

      <div class="flex items-center justify-end gap-2 pt-1">
        <Button
          type="button"
          variant="outline"
          size="sm"
          data-testid="update-dismiss-btn"
          :disabled="opening"
          @click="handleDismiss"
        >
          忽略此版本
        </Button>
        <Button
          type="button"
          size="sm"
          data-testid="update-download-btn"
          :disabled="opening || !updateInfo?.releaseUrl"
          @click="handleOpenRelease"
        >
          <ExternalLink class="size-3.5" aria-hidden="true" />
          前往下载
        </Button>
      </div>
    </DialogContent>
  </Dialog>
</template>
