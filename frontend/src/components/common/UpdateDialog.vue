<script setup lang="ts">
import { computed } from 'vue'
import { ArrowRight, ExternalLink } from '@lucide/vue'
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
const releaseUrl = computed(() => props.updateInfo?.releaseUrl?.trim() ?? '')
const busy = computed(() => Boolean(props.opening || props.applying))
const showProgress = computed(() => Boolean(props.applying || props.progress))
const showReleaseLink = computed(
  () => canSelfUpdate.value && Boolean(releaseUrl.value) && !props.applying,
)

const dialogTitle = computed(() => (props.applying ? '正在更新' : '发现新版本'))
const dialogDescription = computed(() => {
  if (props.applying) return '更新完成后将自动重启，请稍候'
  return canSelfUpdate.value ? '安装完成后将自动重启应用' : '请前往发布页下载安装包完成更新'
})

const progressPercent = computed(() => {
  const pct = props.progress?.percent ?? 0
  if (!Number.isFinite(pct)) return 0
  return Math.max(0, Math.min(100, Math.round(pct)))
})

const handleOpenAutoFocus = (event: Event) => {
  event.preventDefault()
  if (event.target instanceof HTMLElement) {
    event.target.focus()
  }
}

const handleDialogOpenChange = (next: boolean) => {
  if (!next && props.applying) return
  open.value = next
}

const handleApplyUpdate = () => {
  if (!canSelfUpdate.value || busy.value) return
  emit('applyUpdate')
}

const handleOpenRelease = () => {
  if (!releaseUrl.value || busy.value) return
  emit('openRelease', releaseUrl.value)
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
    <AppDialogContent
      size="md"
      :show-close-button="!applying"
      @open-auto-focus="handleOpenAutoFocus"
    >
      <div class="flex items-center gap-3.5 pr-6">
        <img
          src="/favicon.svg"
          alt="TunnelKit"
          class="size-11 shrink-0 rounded-xl shadow-xs ring-1 ring-black/6 dark:ring-white/10"
        />
        <div class="min-w-0">
          <AppDialogTitle>{{ dialogTitle }}</AppDialogTitle>
          <AppDialogDescription class="mt-0.5">{{ dialogDescription }}</AppDialogDescription>
        </div>
      </div>

      <div
        v-if="updateInfo || showProgress"
        class="min-w-0 divide-y divide-border overflow-hidden rounded-xl border border-border bg-muted/35 text-caption"
      >
        <div v-if="updateInfo" class="flex items-center justify-between gap-3 px-4 py-3">
          <div class="inline-flex min-w-0 items-center gap-2 font-mono tabular-nums">
            <span class="truncate text-muted-foreground">v{{ updateInfo.currentVersion }}</span>
            <ArrowRight class="size-3.5 shrink-0 text-muted-foreground/70" aria-hidden="true" />
            <span
              class="shrink-0 rounded-md bg-primary/12 px-2 py-0.5 font-semibold text-primary"
              data-testid="update-latest-version"
            >
              v{{ updateInfo.latestVersion }}
            </span>
          </div>

          <button
            v-if="showReleaseLink"
            type="button"
            data-testid="update-release-link"
            :disabled="busy"
            class="-mr-1.5 inline-flex shrink-0 cursor-pointer items-center gap-1 rounded-md px-1.5 py-0.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/40 focus-visible:outline-none disabled:pointer-events-none disabled:opacity-45"
            @click="handleOpenRelease"
          >
            <span>查看发布页</span>
            <ExternalLink class="size-3.5 shrink-0" aria-hidden="true" />
          </button>
        </div>

        <div v-if="showProgress" data-testid="update-progress" class="space-y-2 px-4 py-3">
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
      </div>

      <div v-if="!applying" class="flex items-center justify-between gap-3">
        <AppButton
          type="button"
          variant="ghost"
          size="sm"
          data-testid="update-dismiss-btn"
          class="-ml-2.5 px-2.5 text-caption font-normal text-muted-foreground hover:text-foreground"
          :disabled="busy"
          @click="handleDismiss"
        >
          忽略此版本
        </AppButton>

        <AppButton
          v-if="canSelfUpdate"
          type="button"
          size="sm"
          data-testid="update-apply-btn"
          class="px-4"
          :disabled="busy"
          @click="handleApplyUpdate"
        >
          立即更新
        </AppButton>
        <AppButton
          v-else
          type="button"
          size="sm"
          data-testid="update-download-btn"
          class="px-3.5"
          :disabled="busy || !releaseUrl"
          @click="handleOpenRelease"
        >
          前往下载
          <ExternalLink class="size-3.5" aria-hidden="true" />
        </AppButton>
      </div>
    </AppDialogContent>
  </Dialog>
</template>
