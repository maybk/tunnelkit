<script setup lang="ts">
import { computed } from 'vue'
import { ArrowRight, Download, ExternalLink } from '@lucide/vue'
import type { UpdateInfo, UpdateProgress } from '@/types/system'
import AppButton from '@/components/base/button/AppButton.vue'
import AppDialogContent from '@/components/base/AppDialogContent.vue'
import AppDialogDescription from '@/components/base/AppDialogDescription.vue'
import AppDialogTitle from '@/components/base/AppDialogTitle.vue'
import { Dialog } from '@/components/ui/dialog'

const props = defineProps<{
  updateInfo: UpdateInfo | null
  opening?: boolean
  applying?: boolean
  progress?: UpdateProgress | null
}>()

const open = defineModel<boolean>('open', { required: true })

const emit = defineEmits<{
  applyUpdate: []
  openRelease: [url: string]
  dismissVersion: [version: string]
}>()

const canSelfUpdate = computed(() => Boolean(props.updateInfo?.canSelfUpdate))
const busy = computed(() => Boolean(props.opening || props.applying))
const progressPercent = computed(() => {
  const pct = props.progress?.percent ?? 0
  return Math.max(0, Math.min(100, Math.round(pct)))
})

const handleDialogOpenChange = (next: boolean) => {
  if (!next && props.applying) return
  open.value = next
}

const handleApplyUpdate = () => {
  if (!canSelfUpdate.value || busy.value) return
  emit('applyUpdate')
}

const handleOpenRelease = () => {
  if (!props.updateInfo?.releaseUrl || busy.value) return
  emit('openRelease', props.updateInfo.releaseUrl)
}

const handleDismiss = () => {
  if (busy.value) return
  if (!props.updateInfo?.latestVersion) {
    open.value = false
    return
  }
  emit('dismissVersion', props.updateInfo.latestVersion)
}
</script>

<template>
  <Dialog :open="open" @update:open="handleDialogOpenChange">
    <AppDialogContent size="md" :show-close-button="!applying">
      <div class="flex items-center gap-3.5">
        <img
          src="/favicon.svg"
          alt="TunnelKit"
          class="size-11 shrink-0 rounded-xl shadow-xs ring-1 ring-black/6 dark:ring-white/10"
        />
        <div>
          <AppDialogTitle>{{ applying ? '正在更新' : '发现新版本' }}</AppDialogTitle>
          <AppDialogDescription class="mt-0.5">
            <template v-if="applying">更新完成后将自动重启，请稍候</template>
            <template v-else>检测到新版本 v{{ updateInfo?.latestVersion }} 可供下载</template>
          </AppDialogDescription>
        </div>
      </div>

      <div v-if="updateInfo" class="flex flex-wrap items-center justify-between gap-2 text-caption">
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

      <div
        v-if="applying || progress"
        data-testid="update-progress"
        class="space-y-1.5 rounded-xl border border-border bg-muted/25 px-3.5 py-2.5 text-caption"
      >
        <div class="flex items-center justify-between gap-2 text-muted-foreground">
          <span role="status" class="min-w-0 break-words">{{
            progress?.message || '正在准备更新…'
          }}</span>
          <span class="shrink-0 font-mono tabular-nums">{{ progressPercent }}%</span>
        </div>
        <div
          role="progressbar"
          aria-label="更新进度"
          :aria-valuenow="progressPercent"
          aria-valuemin="0"
          aria-valuemax="100"
          class="h-1.5 w-full overflow-hidden rounded-full bg-muted"
        >
          <div
            class="h-full rounded-full bg-primary transition-[width] duration-150"
            :style="{ width: `${progressPercent}%` }"
          />
        </div>
      </div>

      <div v-if="!applying" class="flex flex-wrap items-center justify-end gap-2">
        <AppButton
          type="button"
          variant="outline"
          size="sm"
          data-testid="update-dismiss-btn"
          :disabled="busy"
          @click="handleDismiss"
        >
          忽略此版本
        </AppButton>
        <AppButton
          type="button"
          :variant="canSelfUpdate ? 'outline' : 'default'"
          size="sm"
          data-testid="update-download-btn"
          :disabled="busy || !updateInfo?.releaseUrl"
          @click="handleOpenRelease"
        >
          <ExternalLink class="size-3.5" aria-hidden="true" />
          前往下载
        </AppButton>
        <AppButton
          v-if="canSelfUpdate"
          type="button"
          size="sm"
          data-testid="update-apply-btn"
          :disabled="busy"
          @click="handleApplyUpdate"
        >
          <Download class="size-3.5" aria-hidden="true" />
          立即更新并重启
        </AppButton>
      </div>
    </AppDialogContent>
  </Dialog>
</template>
