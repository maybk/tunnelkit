<script setup lang="ts">
import { Check, ExternalLink, RefreshCw, Sparkles } from 'lucide-vue-next'
import { GITHUB_REPO_LABEL, GITHUB_REPO_URL, type SystemInfo, type UpdateInfo } from '@/api/system'
import StatusDot from '@/components/common/StatusDot.vue'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'

withDefaults(
  defineProps<{
    systemInfo: SystemInfo | null
    checking: boolean
    updateInfo?: UpdateInfo | null
    checkingUpdate?: boolean
    updateFeedback?: string
  }>(),
  {
    updateInfo: null,
    checkingUpdate: false,
    updateFeedback: '',
  },
)

const open = defineModel<boolean>('open', { required: true })

const emit = defineEmits<{
  recheck: []
  checkUpdate: []
  showUpdate: []
  openRepo: []
}>()
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

      <div class="flex items-center justify-between gap-2 pt-0.5">
        <div class="min-w-0">
          <span
            v-if="updateFeedback && !updateInfo?.hasUpdate"
            data-testid="about-update-feedback"
            class="inline-flex items-center gap-1 text-caption text-status-ok"
          >
            <Check class="size-3.5 shrink-0" aria-hidden="true" />
            <span class="truncate">{{ updateFeedback }}</span>
          </span>
        </div>

        <div class="flex shrink-0 items-center gap-2">
          <Button
            v-if="systemInfo && !systemInfo.helperReady"
            variant="outline"
            size="sm"
            :disabled="checking"
            @click="emit('recheck')"
          >
            <RefreshCw
              class="size-3.5"
              :class="checking ? 'animate-spin' : ''"
              aria-hidden="true"
            />
            重新检测
          </Button>

          <Button
            v-if="updateInfo?.hasUpdate"
            size="sm"
            data-testid="about-show-update"
            @click="emit('showUpdate')"
          >
            <Sparkles class="size-3.5" aria-hidden="true" />
            新版本 v{{ updateInfo.latestVersion }}
          </Button>
          <Button
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
          </Button>
        </div>
      </div>
    </DialogContent>
  </Dialog>
</template>
