<script setup lang="ts">
import { computed } from 'vue'
import { Check, ExternalLink, RefreshCw, Sparkles } from '@lucide/vue'
import BrandMark from '@/components/common/BrandMark.vue'
import StateText from '@/components/common/StateText.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import AppButton from '@/components/base/button/AppButton.vue'
import AppDialogContent from '@/components/base/AppDialogContent.vue'
import AppDialogDescription from '@/components/base/AppDialogDescription.vue'
import AppDialogTitle from '@/components/base/AppDialogTitle.vue'
import { Dialog } from '@/components/ui/dialog'
import { GITHUB_REPO_LABEL, GITHUB_REPO_URL } from '@/constants/releases'
import type { SystemInfo, UpdateInfo } from '@/types/system'

const open = defineModel<boolean>('open', { required: true })

const props = withDefaults(
  defineProps<{
    systemInfo: SystemInfo | null
    appVersion: string
    updateInfo: UpdateInfo | null
    checkingUpdate?: boolean
    checkingSystem?: boolean
    upToDate?: boolean
  }>(),
  {
    checkingUpdate: false,
    checkingSystem: false,
    upToDate: false,
  },
)

const emit = defineEmits<{
  checkSystem: []
  checkUpdate: []
  showUpdate: []
  openRepo: []
}>()

const showUpToDate = computed(
  () => !props.checkingUpdate && props.upToDate && !props.updateInfo?.hasUpdate,
)

const checkState = computed(() => {
  if (props.checkingUpdate) return 'checking'
  if (showUpToDate.value) return 'up-to-date'
  return 'idle'
})

const checkLabel = computed(() => {
  if (props.checkingUpdate) return '正在检查…'
  if (showUpToDate.value) return '已是最新版本'
  return '检查更新'
})
</script>

<template>
  <Dialog v-model:open="open">
    <AppDialogContent size="md">
      <div class="flex items-center gap-3.5">
        <BrandMark class="size-11 p-1.5" />
        <div>
          <div class="flex items-center gap-2">
            <AppDialogTitle>TunnelKit</AppDialogTitle>
            <span
              v-if="appVersion || systemInfo?.appVersion"
              class="rounded-md bg-muted px-2 py-0.5 font-mono text-caption text-muted-foreground tabular-nums"
            >
              v{{ appVersion || systemInfo?.appVersion }}
            </span>
          </div>
          <AppDialogDescription class="mt-0.5">
            跨平台 Fortinet SSL-VPN 桌面客户端
          </AppDialogDescription>
        </div>
      </div>

      <dl
        class="min-w-0 divide-y divide-border overflow-hidden rounded-xl border border-border bg-muted/35 text-caption"
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
            <StatusDot :tone="systemInfo?.helperReady ? 'success' : 'danger'" />
            <span class="truncate">{{ systemInfo?.helperReady ? '已就绪' : '未就绪' }}</span>
          </dd>
        </div>
        <div class="flex items-center justify-between gap-4 px-4 py-3">
          <dt class="shrink-0 text-muted-foreground">配置与日志目录</dt>
          <dd class="min-w-0 truncate font-mono text-foreground">~/.tunnelkit/</dd>
        </div>
        <div class="flex items-center justify-between gap-4 px-4 py-3">
          <dt class="shrink-0 text-muted-foreground">开源仓库</dt>
          <dd class="min-w-0 truncate">
            <button
              type="button"
              data-testid="about-open-repo"
              :title="GITHUB_REPO_URL"
              class="inline-flex max-w-full cursor-pointer items-center gap-1 font-mono text-foreground transition-colors hover:text-primary focus-visible:underline focus-visible:outline-none"
              @click="emit('openRepo')"
            >
              <span class="truncate">{{ GITHUB_REPO_LABEL }}</span>
              <ExternalLink class="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
            </button>
          </dd>
        </div>
      </dl>

      <div class="flex flex-wrap items-center justify-end gap-2">
        <AppButton
          v-if="systemInfo && !systemInfo.helperReady"
          type="button"
          variant="outline"
          size="sm"
          :disabled="checkingSystem"
          @click="emit('checkSystem')"
        >
          <RefreshCw
            class="size-3.5"
            :class="checkingSystem ? 'animate-spin' : ''"
            aria-hidden="true"
          />
          重新检测
        </AppButton>

        <AppButton
          v-if="updateInfo?.hasUpdate"
          type="button"
          size="sm"
          data-testid="about-show-update"
          @click="emit('showUpdate')"
        >
          <Sparkles class="size-3.5" aria-hidden="true" />
          新版本 v{{ updateInfo.latestVersion }}
        </AppButton>
        <AppButton
          v-else
          type="button"
          variant="outline"
          size="sm"
          data-testid="about-check-update"
          :data-state="checkState"
          aria-live="polite"
          :disabled="checkingUpdate || showUpToDate"
          :class="
            showUpToDate
              ? 'border-tone-success-border bg-tone-success-soft text-tone-success-fg disabled:opacity-100 dark:border-tone-success-border dark:bg-tone-success-soft'
              : ''
          "
          @click="emit('checkUpdate')"
        >
          <span class="inline-grid size-3.5 place-items-center" aria-hidden="true">
            <RefreshCw
              class="[grid-area:1/1] size-3.5 transition-[opacity,transform,filter] duration-200 ease-[cubic-bezier(0.2,0,0,1)]"
              :class="[
                checkingUpdate ? 'animate-spin' : '',
                showUpToDate
                  ? 'scale-[0.25] opacity-0 blur-[4px]'
                  : 'scale-100 opacity-100 blur-none',
              ]"
            />
            <Check
              class="[grid-area:1/1] size-3.5 transition-[opacity,transform,filter] duration-200 ease-[cubic-bezier(0.2,0,0,1)]"
              :class="
                showUpToDate
                  ? 'scale-100 opacity-100 blur-none'
                  : 'scale-[0.25] opacity-0 blur-[4px]'
              "
            />
          </span>
          <StateText :text="checkLabel" :transition-key="checkState" />
        </AppButton>
      </div>
    </AppDialogContent>
  </Dialog>
</template>
