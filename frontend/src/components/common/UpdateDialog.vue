<script setup lang="ts">
import { computed } from 'vue'
import { ArrowRight, ExternalLink } from '@lucide/vue'
import type { UpdateInfo, UpdatePhase, UpdateProgress } from '@/types/system'
import { buildUpdateDialogViewModel } from '@/presentation/update'
import AppButton from '@/components/base/button/AppButton.vue'
import AppDialogContent from '@/components/base/AppDialogContent.vue'
import AppDialogDescription from '@/components/base/AppDialogDescription.vue'
import AppDialogTitle from '@/components/base/AppDialogTitle.vue'
import { Dialog } from '@/components/ui/dialog'
import BrandMark from '@/components/common/BrandMark.vue'

const props = withDefaults(
  defineProps<{
    updateInfo: UpdateInfo | null
    phase?: UpdatePhase
    opening?: boolean
    canceling?: boolean
    vpnActive?: boolean
    progress?: UpdateProgress | null
  }>(),
  {
    phase: 'available',
    opening: false,
    canceling: false,
    vpnActive: false,
    progress: null,
  },
)

const open = defineModel<boolean>('open', { required: true })

const emit = defineEmits<{
  downloadUpdate: []
  cancelUpdate: []
  installUpdate: []
  deferInstall: []
  openRelease: [url: string]
  dismissVersion: [version: string]
}>()

const view = computed(() =>
  buildUpdateDialogViewModel({
    phase: props.phase,
    updateInfo: props.updateInfo,
    progress: props.progress,
    vpnActive: props.vpnActive,
  }),
)
const canSelfUpdate = computed(() => Boolean(props.updateInfo?.canSelfUpdate))
const releaseUrl = computed(() => props.updateInfo?.releaseUrl?.trim() ?? '')
const busy = computed(() => Boolean(props.opening || view.value.isLocked))
const showReleaseLink = computed(
  () => props.phase === 'available' && canSelfUpdate.value && Boolean(releaseUrl.value),
)

const handleOpenAutoFocus = (event: Event) => {
  event.preventDefault()
  if (event.target instanceof HTMLElement) {
    event.target.focus()
  }
}

const guardLockedDismiss = (event: Event) => {
  if (view.value.isLocked) event.preventDefault()
}

const handleDialogOpenChange = (next: boolean) => {
  if (!next && view.value.isLocked) return
  open.value = next
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
      class="gap-0 rounded-2xl border-border/80 px-7 pt-7 pb-5 shadow-[0_20px_50px_-12px_rgba(0,0,0,0.22),0_4px_16px_-4px_rgba(0,0,0,0.08)] dark:shadow-[0_24px_60px_-12px_rgba(0,0,0,0.6),0_0_0_1px_rgba(255,255,255,0.06)]"
      :show-close-button="!view.isLocked"
      @open-auto-focus="handleOpenAutoFocus"
      @escape-key-down="guardLockedDismiss"
      @pointer-down-outside="guardLockedDismiss"
      @interact-outside="guardLockedDismiss"
    >
      <template v-if="phase === 'available'">
        <div class="flex items-center gap-4">
          <div
            class="flex size-14 shrink-0 items-center justify-center rounded-xl bg-card p-2.5 shadow-[0_2px_6px_rgba(0,0,0,0.06)] ring-1 ring-black/10 dark:ring-white/10"
          >
            <BrandMark class="size-full" />
          </div>

          <div class="min-w-0 flex-1">
            <AppDialogTitle class="pr-6 text-[16px] leading-snug font-semibold tracking-tight">
              {{ view.title }}
            </AppDialogTitle>

            <div
              v-if="updateInfo"
              class="mt-2 flex flex-wrap items-center justify-between gap-2 text-[13px] text-muted-foreground tabular-nums"
            >
              <div class="inline-flex min-w-0 items-center gap-1.5">
                <span class="truncate">v{{ updateInfo.currentVersion }}</span>
                <ArrowRight class="size-3.5 shrink-0 text-muted-foreground/65" aria-hidden="true" />
                <span
                  data-testid="update-latest-version"
                  class="rounded-md bg-muted px-2 py-0.5 leading-tight font-medium text-foreground"
                >
                  v{{ updateInfo.latestVersion }}
                </span>
              </div>

              <button
                v-if="showReleaseLink"
                type="button"
                data-testid="update-release-link"
                :disabled="busy"
                class="-mr-1.5 inline-flex shrink-0 cursor-pointer items-center gap-1 rounded-md px-1.5 py-0.5 text-caption text-muted-foreground transition-[color,background-color,transform] duration-150 hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/40 focus-visible:outline-none active:scale-[0.96] disabled:pointer-events-none disabled:opacity-45"
                @click="handleOpenRelease"
              >
                <span>更新说明</span>
                <ExternalLink class="size-3 shrink-0" aria-hidden="true" />
              </button>
            </div>
          </div>
        </div>

        <AppDialogDescription
          :class="
            canSelfUpdate ? 'sr-only' : 'mt-4 text-[13px] leading-relaxed text-muted-foreground'
          "
        >
          {{ view.description }}
        </AppDialogDescription>

        <div class="mt-9 flex items-center justify-between gap-3">
          <AppButton
            type="button"
            variant="ghost"
            data-testid="update-dismiss-btn"
            class="-ml-3 px-3 text-[13px] font-normal text-muted-foreground hover:text-foreground"
            :disabled="busy"
            @click="handleDismiss"
          >
            忽略此版本
          </AppButton>

          <AppButton
            v-if="canSelfUpdate"
            type="button"
            data-testid="update-apply-btn"
            class="px-5"
            :disabled="busy"
            @click="!busy && emit('downloadUpdate')"
          >
            立即更新
          </AppButton>
          <AppButton
            v-else
            type="button"
            data-testid="update-download-btn"
            class="px-4"
            :disabled="busy || !releaseUrl"
            @click="handleOpenRelease"
          >
            前往下载
            <ExternalLink class="size-3.5" aria-hidden="true" />
          </AppButton>
        </div>
      </template>

      <div v-else class="flex items-start gap-4">
        <div
          class="mt-0.5 flex size-14 shrink-0 items-center justify-center rounded-xl bg-card p-2.5 shadow-[0_2px_6px_rgba(0,0,0,0.06)] ring-1 ring-black/10 dark:ring-white/10"
        >
          <BrandMark class="size-full" />
        </div>

        <div class="min-w-0 flex-1">
          <AppDialogTitle
            class="truncate text-[16px] leading-snug font-semibold tracking-tight"
            :class="view.isLocked ? '' : 'pr-6'"
          >
            {{ view.title }}
          </AppDialogTitle>

          <AppDialogDescription class="sr-only">
            {{ view.description }}
          </AppDialogDescription>

          <div data-testid="update-progress" class="mt-4.5">
            <div
              role="progressbar"
              aria-label="更新进度"
              :aria-valuenow="view.isIndeterminate ? undefined : view.progressPercent"
              aria-valuemin="0"
              aria-valuemax="100"
              class="h-2 w-full overflow-hidden rounded-full bg-muted"
            >
              <div
                v-if="view.isIndeterminate"
                class="update-progress-indeterminate h-full w-2/5 rounded-full bg-primary"
              />
              <div
                v-else-if="phase === 'ready'"
                class="h-full w-full rounded-full bg-primary/25 dark:bg-primary/35"
              />
              <div
                v-else
                class="h-full rounded-full bg-primary motion-safe:transition-[width] motion-safe:duration-150 motion-safe:ease-out"
                :style="{ width: `${view.progressPercent}%` }"
              />
            </div>
          </div>

          <div class="mt-6 flex min-h-8.5 items-center justify-between gap-3">
            <span
              role="status"
              data-testid="update-progress-meta"
              class="min-w-0 truncate text-[13px] text-muted-foreground tabular-nums"
            >
              {{ view.metaText }}
            </span>

            <div class="flex shrink-0 items-center gap-2">
              <AppButton
                v-if="phase === 'downloading' || phase === 'verifying'"
                type="button"
                variant="secondary"
                data-testid="update-cancel-btn"
                class="px-4"
                :disabled="phase !== 'downloading' || canceling"
                @click="phase === 'downloading' && !canceling && emit('cancelUpdate')"
              >
                {{ canceling ? '取消中…' : '取消' }}
              </AppButton>

              <template v-else-if="phase === 'ready'">
                <AppButton
                  type="button"
                  variant="ghost"
                  data-testid="update-defer-btn"
                  class="px-3 text-[13px] font-normal text-muted-foreground hover:text-foreground"
                  @click="emit('deferInstall')"
                >
                  稍后
                </AppButton>
                <AppButton
                  type="button"
                  data-testid="update-install-btn"
                  class="px-4"
                  @click="emit('installUpdate')"
                >
                  安装并重启
                </AppButton>
              </template>
            </div>
          </div>
        </div>
      </div>
    </AppDialogContent>
  </Dialog>
</template>
