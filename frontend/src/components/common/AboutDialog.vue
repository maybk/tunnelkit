<script setup lang="ts">
import { Check, ExternalLink, RefreshCw, Sparkles } from '@lucide/vue'
import StatusDot from '@/components/common/StatusDot.vue'
import AppButton from '@/components/base/button/AppButton.vue'
import AppDialogContent from '@/components/base/AppDialogContent.vue'
import AppDialogDescription from '@/components/base/AppDialogDescription.vue'
import AppDialogTitle from '@/components/base/AppDialogTitle.vue'
import { Dialog } from '@/components/ui/dialog'
import { GITHUB_REPO_LABEL, GITHUB_REPO_URL } from '@/constants/releases'
import type { SystemInfo, UpdateInfo } from '@/types/system'

const open = defineModel<boolean>('open', { required: true })

withDefaults(
  defineProps<{
    systemInfo: SystemInfo | null
    appVersion: string
    updateInfo: UpdateInfo | null
    checkingUpdate?: boolean
    checkingSystem?: boolean
    updateFeedback?: string
  }>(),
  {
    checkingUpdate: false,
    checkingSystem: false,
    updateFeedback: '',
  },
)

const emit = defineEmits<{
  checkSystem: []
  checkUpdate: []
  showUpdate: []
  openRepo: []
}>()
</script>

<template>
  <Dialog v-model:open="open">
    <AppDialogContent size="md">
      <div class="flex items-center gap-3.5">
        <img
          src="/favicon.svg"
          alt="TunnelKit"
          class="size-11 shrink-0 rounded-xl shadow-xs ring-1 ring-black/6 dark:ring-white/10"
        />
        <div>
          <div class="flex items-center gap-2">
            <AppDialogTitle>TunnelKit</AppDialogTitle>
            <span
              v-if="systemInfo?.appVersion"
              class="rounded-md bg-muted px-2 py-0.5 font-mono text-caption text-muted-foreground tabular-nums"
            >
              v{{ systemInfo.appVersion }}
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

      <div class="flex flex-wrap items-center justify-between gap-2">
        <div class="min-w-0">
          <span
            v-if="updateFeedback && !updateInfo?.hasUpdate"
            data-testid="about-update-feedback"
            class="inline-flex items-center gap-1 text-caption text-tone-success-solid"
          >
            <Check class="size-3.5 shrink-0" aria-hidden="true" />
            <span class="truncate">{{ updateFeedback }}</span>
          </span>
        </div>

        <div class="flex shrink-0 items-center gap-2">
          <AppButton
            v-if="systemInfo && !systemInfo.helperReady"
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
            size="sm"
            data-testid="about-show-update"
            @click="emit('showUpdate')"
          >
            <Sparkles class="size-3.5" aria-hidden="true" />
            新版本 v{{ updateInfo.latestVersion }}
          </AppButton>
          <AppButton
            v-else
            variant="outline"
            size="sm"
            data-testid="about-check-update"
            :disabled="checkingUpdate"
            @click="emit('checkUpdate')"
          >
            <RefreshCw
              class="size-3.5"
              :class="checkingUpdate ? 'animate-spin' : ''"
              aria-hidden="true"
            />
            {{ checkingUpdate ? '正在检查…' : '检查更新' }}
          </AppButton>
        </div>
      </div>
    </AppDialogContent>
  </Dialog>
</template>
