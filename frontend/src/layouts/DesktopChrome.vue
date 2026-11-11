<script setup lang="ts">
import { computed } from 'vue'
import { Copy, Info, Minus, Monitor, Moon, Square, Sun, X } from '@lucide/vue'
import StatusDot from '@/components/common/StatusDot.vue'
import AppButton from '@/components/base/button/AppButton.vue'
import { THEME_MODE_META } from '@/constants/appearance'
import {
  windowControlRegionStyle,
  windowCaptionRegionStyle,
  windowDragRegionStyle,
  windowInteractiveRegionStyle,
} from '@/layouts/windowRegions'
import type { ThemeMode } from '@/types/appearance'
import type { DesktopPlatform } from '@/types/desktop'
import type { Tone } from '@/constants/tone'

const props = withDefaults(
  defineProps<{
    platform: DesktopPlatform
    title: string
    statusLabel: string
    statusTone: Tone
    statusPulse?: boolean
    isFocused?: boolean
    isMaximised?: boolean
    isFullscreen?: boolean
    themeMode?: ThemeMode
    hasUpdate?: boolean
  }>(),
  {
    isFocused: true,
    statusPulse: false,
    isMaximised: false,
    isFullscreen: false,
    themeMode: 'system',
    hasUpdate: false,
  },
)

const emit = defineEmits<{
  minimise: []
  toggleMaximise: []
  close: []
  cycleTheme: []
  openAbout: []
}>()

const themeLabel = computed(
  () => THEME_MODE_META.find((item) => item.id === props.themeMode)?.label ?? '跟随系统',
)
</script>

<template>
  <header
    class="relative z-40 flex h-(--desktop-chrome-height) shrink-0 items-center justify-between border-b border-sidebar-border bg-sidebar/92 text-foreground backdrop-blur-md transition-colors data-[active=false]:text-muted-foreground"
    :data-active="isFocused"
    :data-platform="platform"
    :data-fullscreen="isFullscreen"
  >
    <div
      class="flex min-w-0 flex-1 items-center self-stretch select-none"
      :style="windowDragRegionStyle"
    >
      <span
        v-if="platform === 'macos' && !isFullscreen"
        data-testid="desktop-chrome-leading-inset"
        class="w-(--desktop-chrome-macos-leading-inset) shrink-0"
        aria-hidden="true"
      />
      <span
        class="truncate text-body font-semibold tracking-tight text-foreground/90"
        :class="platform === 'macos' && !isFullscreen ? 'pl-1' : 'pl-4'"
        >{{ title }}</span
      >
      <span
        class="min-w-(--desktop-chrome-min-drag-width) flex-1 self-stretch"
        :style="platform === 'windows' ? windowCaptionRegionStyle : windowDragRegionStyle"
        aria-hidden="true"
      />
    </div>

    <div class="flex shrink-0 items-center gap-1.5 px-3" :style="windowInteractiveRegionStyle">
      <span
        class="inline-flex max-w-64 items-center gap-1.5 rounded-full bg-foreground/[0.05] px-2.5 py-1 text-caption font-medium text-muted-foreground select-none"
      >
        <StatusDot :tone="statusTone" :pulse="statusPulse" />
        <span class="truncate">{{ statusLabel }}</span>
      </span>
      <AppButton
        variant="ghost"
        size="icon-xs"
        :title="`主题：${themeLabel}`"
        :aria-label="`主题：${themeLabel}`"
        data-testid="chrome-theme-toggle"
        class="text-muted-foreground hover:text-foreground"
        @click="emit('cycleTheme')"
      >
        <Sun v-if="themeMode === 'light'" class="size-3.5" />
        <Moon v-else-if="themeMode === 'dark'" class="size-3.5" />
        <Monitor v-else class="size-3.5" />
      </AppButton>
      <AppButton
        variant="ghost"
        size="icon-xs"
        :title="hasUpdate ? '关于 TunnelKit（有可用更新）' : '关于 TunnelKit'"
        aria-label="关于 TunnelKit"
        data-testid="chrome-about-btn"
        class="relative text-muted-foreground hover:text-foreground"
        @click="emit('openAbout')"
      >
        <Info class="size-4" />
        <span
          v-if="hasUpdate"
          data-testid="chrome-update-dot"
          class="pointer-events-none absolute top-1 right-1 size-1.5 rounded-full bg-primary ring-2 ring-sidebar"
          aria-hidden="true"
        />
      </AppButton>
    </div>

    <fieldset
      v-if="platform === 'windows' && !isFullscreen"
      class="m-0 flex shrink-0 self-stretch border-0 p-0"
      aria-label="窗口控制"
    >
      <button
        type="button"
        aria-label="最小化"
        title="最小化"
        class="flex size-12 items-center justify-center outline-none hover:bg-foreground/8 focus-visible:ring-2 focus-visible:ring-ring"
        :style="windowControlRegionStyle('minimize')"
        @click="emit('minimise')"
      >
        <Minus class="size-4" aria-hidden="true" />
      </button>
      <button
        type="button"
        :aria-label="isMaximised ? '向下还原' : '最大化'"
        :title="isMaximised ? '向下还原' : '最大化'"
        class="flex size-12 items-center justify-center outline-none hover:bg-foreground/8 focus-visible:ring-2 focus-visible:ring-ring"
        :style="windowControlRegionStyle('maximize')"
        @click="emit('toggleMaximise')"
      >
        <Copy v-if="isMaximised" class="size-4" aria-hidden="true" /><Square
          v-else
          class="size-4"
          aria-hidden="true"
        />
      </button>
      <button
        type="button"
        aria-label="关闭窗口"
        title="关闭窗口"
        class="flex size-12 items-center justify-center outline-none hover:bg-tone-danger-solid hover:text-destructive-foreground focus-visible:ring-2 focus-visible:ring-ring"
        :style="windowControlRegionStyle('close')"
        @click="emit('close')"
      >
        <X class="size-4" aria-hidden="true" />
      </button>
    </fieldset>
  </header>
</template>
