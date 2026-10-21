<script setup lang="ts">
import { Copy, Minus, Square, X } from 'lucide-vue-next'
import {
  windowCaptionRegionStyle,
  windowControlRegionStyle,
  windowDragRegionStyle,
  type DesktopPlatform,
} from '@/api/window'

withDefaults(
  defineProps<{
    platform: DesktopPlatform
    title: string
    isFocused?: boolean
    isMaximised?: boolean
    isFullscreen?: boolean
  }>(),
  {
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
</script>

<template>
  <header
    :title="title"
    :data-active="isFocused"
    :data-platform="platform"
    :data-fullscreen="isFullscreen"
    class="relative z-40 flex h-(--desktop-chrome-height) shrink-0 items-center justify-between border-b border-sidebar-border bg-sidebar/92 text-foreground backdrop-blur-md transition-colors data-[active=false]:text-muted-foreground"
  >
    <div
      data-testid="desktop-chrome-drag-region"
      class="flex min-w-0 flex-1 items-center self-stretch select-none"
      :style="windowDragRegionStyle"
    >
      <span
        v-if="platform === 'macos' && !isFullscreen"
        data-testid="desktop-chrome-leading-inset"
        class="w-(--desktop-chrome-macos-leading-inset) shrink-0"
        aria-hidden="true"
      />

      <div class="flex min-w-0 flex-1 items-center self-stretch">
        <slot />
        <!-- 独立空白拖拽保底区：在 Windows Composition Hosting 下声明为原生 caption 区域 -->
        <div
          data-testid="desktop-chrome-caption-spacer"
          class="min-w-(--desktop-chrome-min-drag-width) flex-1 self-stretch"
          :style="windowCaptionRegionStyle"
          aria-hidden="true"
        />
      </div>
    </div>

    <div v-if="$slots.actions" class="flex shrink-0 items-center gap-1.5 px-3">
      <slot name="actions" />
    </div>

    <fieldset
      v-if="platform === 'windows' && !isFullscreen"
      class="m-0 flex min-w-0 shrink-0 self-stretch border-0 p-0"
      aria-label="窗口控制"
    >
      <button
        type="button"
        aria-label="最小化窗口"
        title="最小化窗口"
        class="flex size-12 items-center justify-center transition-colors duration-100 outline-none hover:bg-foreground/8 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset active:bg-foreground/12 [&>svg]:size-4 [&>svg]:stroke-[1.5]"
        :style="windowControlRegionStyle('minimize')"
        @click="emit('minimise')"
      >
        <Minus aria-hidden="true" />
      </button>

      <button
        type="button"
        :aria-label="isMaximised ? '还原窗口' : '最大化窗口'"
        :title="isMaximised ? '还原窗口' : '最大化窗口'"
        class="flex size-12 items-center justify-center transition-colors duration-100 outline-none hover:bg-foreground/8 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset active:bg-foreground/12 [&>svg]:size-4 [&>svg]:stroke-[1.5]"
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
        class="flex size-12 items-center justify-center transition-colors duration-100 outline-none hover:bg-red-600 hover:text-white focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset active:bg-red-700 [&>svg]:size-4 [&>svg]:stroke-[1.5]"
        :style="windowControlRegionStyle('close')"
        @click="emit('close')"
      >
        <X aria-hidden="true" />
      </button>
    </fieldset>
  </header>
</template>
