<script setup lang="ts">
import { computed } from 'vue'
import { Copy, Minus, Square, X } from '@lucide/vue'
import PageTitle from '@/components/common/PageTitle.vue'
import {
  windowCaptionRegionStyle,
  windowControlRegionStyle,
  windowDragRegionStyle,
} from '@/layouts/windowRegions'
import type { DesktopPlatform } from '@/types/desktop'

const props = withDefaults(
  defineProps<{
    platform: DesktopPlatform
    brandTitle?: string
    title?: string
    parentTitle?: string
    narrowRail?: boolean
    isFocused?: boolean
    isMaximised?: boolean
    isFullscreen?: boolean
  }>(),
  {
    brandTitle: 'TunnelKit',
    title: '',
    parentTitle: undefined,
    narrowRail: false,
    isFocused: true,
    isMaximised: false,
    isFullscreen: false,
  },
)

const emit = defineEmits<{
  minimise: []
  toggleMaximise: []
  close: []
}>()

const WIN_CAPTION_BUTTON_CLASS =
  'flex h-full w-12 items-center justify-center transition-colors duration-100 outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset [&>svg]:size-4 [&>svg]:stroke-[1.5]'

const hasMacosInset = computed(() => props.platform === 'macos' && !props.isFullscreen)
const hasTitle = computed(() => Boolean(props.title.trim()))
</script>

<template>
  <header
    :data-active="isFocused"
    :data-platform="platform"
    :data-fullscreen="isFullscreen"
    class="relative z-40 flex h-(--desktop-chrome-height) shrink-0 items-center justify-between bg-background text-foreground transition-colors data-[active=false]:text-muted-foreground"
  >
    <div
      data-testid="desktop-chrome-sidebar-rail"
      :style="windowDragRegionStyle"
      :class="[
        'flex shrink-0 items-center self-stretch overflow-hidden border-r border-sidebar-border bg-sidebar pr-3 select-none',
        narrowRail ? 'w-48' : 'w-64',
      ]"
    >
      <span
        v-if="hasMacosInset"
        data-testid="desktop-chrome-leading-inset"
        class="w-(--desktop-chrome-macos-leading-inset) shrink-0"
        aria-hidden="true"
      />
      <span
        data-testid="desktop-chrome-brand"
        class="truncate text-body font-semibold tracking-tight text-foreground/90"
        :class="hasMacosInset ? 'pl-1' : 'pl-4'"
      >
        {{ brandTitle }}
      </span>
    </div>

    <div
      data-testid="desktop-chrome-main-bar"
      class="flex min-w-0 flex-1 items-center justify-between self-stretch border-b border-border/60"
    >
      <div
        data-testid="desktop-chrome-drag-region"
        class="flex min-w-0 flex-1 items-center self-stretch select-none"
        :style="windowDragRegionStyle"
      >
        <div class="flex min-w-0 flex-1 items-center gap-3 self-stretch pl-6">
          <PageTitle v-if="hasTitle" :title="title" :parent="parentTitle" />

          <!-- 独立空白拖拽保底区：在 Windows Composition Hosting 下声明为原生 caption 区域 -->
          <div
            data-testid="desktop-chrome-caption-spacer"
            class="min-w-(--desktop-chrome-min-drag-width) flex-1 self-stretch"
            :style="platform === 'windows' ? windowCaptionRegionStyle : windowDragRegionStyle"
            aria-hidden="true"
          />
        </div>
      </div>

      <fieldset
        v-if="platform === 'windows' && !isFullscreen"
        class="m-0 flex min-w-0 shrink-0 self-stretch border-0 p-0"
        aria-label="窗口控制"
      >
        <button
          type="button"
          aria-label="最小化"
          title="最小化"
          :class="[WIN_CAPTION_BUTTON_CLASS, 'hover:bg-foreground/8 active:bg-foreground/12']"
          :style="windowControlRegionStyle('minimize')"
          @click="emit('minimise')"
        >
          <Minus aria-hidden="true" />
        </button>
        <button
          type="button"
          :aria-label="isMaximised ? '向下还原' : '最大化'"
          :title="isMaximised ? '向下还原' : '最大化'"
          :class="[WIN_CAPTION_BUTTON_CLASS, 'hover:bg-foreground/8 active:bg-foreground/12']"
          :style="windowControlRegionStyle('maximize')"
          @click="emit('toggleMaximise')"
        >
          <Copy v-if="isMaximised" aria-hidden="true" />
          <Square v-else aria-hidden="true" />
        </button>
        <button
          type="button"
          aria-label="关闭窗口"
          title="关闭窗口"
          :class="[
            WIN_CAPTION_BUTTON_CLASS,
            'hover:bg-tone-danger-solid hover:text-destructive-foreground active:opacity-90',
          ]"
          :style="windowControlRegionStyle('close')"
          @click="emit('close')"
        >
          <X aria-hidden="true" />
        </button>
      </fieldset>
    </div>
  </header>
</template>
