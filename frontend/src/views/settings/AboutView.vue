<script setup lang="ts">
import { computed, onUnmounted } from 'vue'
import { Check, Copy, ExternalLink, RefreshCw, Sparkles } from '@lucide/vue'
import AppButton from '@/components/base/button/AppButton.vue'
import BrandMark from '@/components/common/BrandMark.vue'
import StateText from '@/components/common/StateText.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import SettingCard from '@/components/settings/SettingCard.vue'
import { useCopyFeedback } from '@/composables/useCopyFeedback'
import { GITHUB_REPO_LABEL, GITHUB_REPO_URL } from '@/constants/releases'
import { useSystemStore } from '@/stores/useSystemStore'

const systemStore = useSystemStore()
const { copiedKey, copy } = useCopyFeedback()

const showUpToDate = computed(
  () =>
    !systemStore.isCheckingUpdate &&
    systemStore.updateUpToDate &&
    !systemStore.updateInfo?.hasUpdate,
)

const checkState = computed(() => {
  if (systemStore.isCheckingUpdate) return 'checking'
  if (showUpToDate.value) return 'up-to-date'
  return 'idle'
})

const checkLabel = computed(() => {
  if (systemStore.isCheckingUpdate) return '正在检查…'
  if (showUpToDate.value) return '已是最新版本'
  return '检查更新'
})

const helperStatus = computed(() => {
  if (!systemStore.systemInfo) {
    return { tone: 'neutral' as const, label: '检测中…' }
  }
  return systemStore.systemInfo.helperReady
    ? { tone: 'success' as const, label: '已就绪' }
    : { tone: 'danger' as const, label: '未就绪' }
})

onUnmounted(() => {
  systemStore.clearUpToDateFeedback()
})
</script>

<template>
  <div class="space-y-5">
    <SettingCard :padded="false">
      <div class="flex flex-wrap items-center justify-between gap-4 p-5">
        <div class="flex min-w-0 items-center gap-3.5">
          <BrandMark class="size-11 p-1.5" />
          <div class="min-w-0 space-y-0.5">
            <div class="flex items-center gap-2">
              <span class="text-title font-bold text-foreground">TunnelKit</span>
              <span
                v-if="systemStore.appVersion"
                data-testid="about-version-badge"
                class="rounded-md bg-foreground/6 px-2 py-0.5 font-mono text-caption font-medium text-muted-foreground tabular-nums select-text"
              >
                v{{ systemStore.appVersion }}
              </span>
              <StatusDot
                v-if="systemStore.hasUpdate"
                data-testid="settings-about-update-dot"
                tone="brand"
                size="sm"
              />
            </div>
            <div class="truncate text-caption text-muted-foreground">
              跨平台 Fortinet SSL-VPN 桌面客户端
            </div>
          </div>
        </div>

        <div class="flex shrink-0 flex-wrap items-center gap-2">
          <AppButton
            v-if="systemStore.systemInfo && !systemStore.systemInfo.helperReady"
            type="button"
            variant="outline"
            size="sm"
            data-testid="about-recheck-system"
            :disabled="systemStore.checkingSystem"
            @click="systemStore.loadSystemInfo()"
          >
            <RefreshCw
              class="size-3.5"
              :class="systemStore.checkingSystem ? 'motion-safe:animate-spin' : ''"
              aria-hidden="true"
            />
            重新检测
          </AppButton>

          <AppButton
            v-if="systemStore.hasUpdate && systemStore.updateInfo"
            type="button"
            size="sm"
            data-testid="about-show-update"
            @click="systemStore.showUpdateDialog()"
          >
            <Sparkles class="size-3.5" aria-hidden="true" />
            {{
              systemStore.isUpdateReady
                ? `安装 v${systemStore.updateInfo.latestVersion}`
                : `新版本 v${systemStore.updateInfo.latestVersion}`
            }}
          </AppButton>

          <AppButton
            v-else
            type="button"
            variant="outline"
            size="sm"
            data-testid="about-check-update"
            :data-state="checkState"
            aria-live="polite"
            :disabled="systemStore.isCheckingUpdate || showUpToDate"
            :class="
              showUpToDate
                ? 'border-tone-success-border bg-tone-success-soft text-tone-success-fg disabled:opacity-100'
                : ''
            "
            @click="systemStore.checkUpdate({ silent: false, inline: true })"
          >
            <span class="inline-grid size-3.5 place-items-center" aria-hidden="true">
              <RefreshCw
                class="[grid-area:1/1] size-3.5 transition-[opacity,transform,filter] duration-200 ease-[cubic-bezier(0.2,0,0,1)]"
                :class="[
                  systemStore.isCheckingUpdate ? 'motion-safe:animate-spin' : '',
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
      </div>

      <dl data-testid="about-info-list" class="min-w-0 divide-y divide-border/50 text-caption">
        <div class="flex items-center justify-between gap-4 px-5 py-3.5">
          <dt class="shrink-0 text-muted-foreground">后台隧道服务</dt>
          <dd
            data-testid="about-helper-status"
            class="inline-flex min-w-0 items-center gap-1.5 font-medium text-foreground"
            :title="systemStore.systemInfo?.helperPath || undefined"
          >
            <StatusDot :tone="helperStatus.tone" />
            <span class="truncate">{{ helperStatus.label }}</span>
          </dd>
        </div>

        <div class="flex items-center justify-between gap-4 px-5 py-3.5">
          <dt class="shrink-0 text-muted-foreground">配置与日志目录</dt>
          <dd class="flex min-w-0 items-center gap-1.5">
            <span
              data-testid="about-data-dir"
              class="min-w-0 truncate font-mono text-foreground select-text"
              :title="systemStore.systemInfo?.dataDir || undefined"
            >
              ~/.tunnelkit/
            </span>
            <button
              v-if="systemStore.systemInfo?.dataDir"
              type="button"
              data-testid="about-copy-data-dir"
              :aria-label="copiedKey === 'dataDir' ? '已复制目录路径' : '复制目录路径'"
              :title="copiedKey === 'dataDir' ? '已复制目录路径' : '复制目录路径'"
              class="inline-flex size-6 shrink-0 cursor-pointer items-center justify-center rounded-md text-muted-foreground transition-colors outline-none hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/40 active:scale-[0.96]"
              @click="copy(systemStore.systemInfo.dataDir, 'dataDir')"
            >
              <Check
                v-if="copiedKey === 'dataDir'"
                class="size-3.5 text-tone-success-fg"
                aria-hidden="true"
              />
              <Copy v-else class="size-3.5" aria-hidden="true" />
            </button>
          </dd>
        </div>

        <div class="flex items-center justify-between gap-4 px-5 py-3.5">
          <dt class="shrink-0 text-muted-foreground">开源仓库</dt>
          <dd class="min-w-0 truncate">
            <button
              type="button"
              data-testid="about-open-repo"
              :title="GITHUB_REPO_URL"
              class="inline-flex max-w-full cursor-pointer items-center gap-1 font-mono text-foreground transition-colors hover:text-primary focus-visible:underline focus-visible:outline-none"
              @click="systemStore.openRepository()"
            >
              <span class="truncate">{{ GITHUB_REPO_LABEL }}</span>
              <ExternalLink class="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
            </button>
          </dd>
        </div>
      </dl>
    </SettingCard>
  </div>
</template>
