<script setup lang="ts">
import type { Component, HTMLAttributes } from 'vue'
import type { Tone } from '@/constants/tone'
import StatusDot from '@/components/common/StatusDot.vue'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    icon?: Component
    label: string
    tooltip?: string
    active?: boolean
    dot?: boolean
    dotTone?: Tone
    dotTestId?: string
    testId?: string
    class?: HTMLAttributes['class']
  }>(),
  {
    icon: undefined,
    tooltip: undefined,
    active: false,
    dot: false,
    dotTone: 'brand',
    dotTestId: undefined,
    testId: undefined,
    class: undefined,
  },
)

const emit = defineEmits<{
  click: []
}>()
</script>

<template>
  <button
    type="button"
    :data-testid="testId"
    :title="tooltip"
    :aria-current="active ? 'page' : undefined"
    :class="
      cn(
        'relative flex w-full items-center justify-between gap-2.5 rounded-lg px-3 py-1.5 text-body transition-[color,background-color,box-shadow,scale] duration-150 ease-out outline-none focus-visible:ring-2 focus-visible:ring-ring/50 active:scale-[0.985]',
        active
          ? 'bg-foreground/7 font-semibold text-foreground shadow-2xs before:absolute before:top-1.5 before:bottom-1.5 before:left-1 before:w-0.5 before:rounded-full before:bg-tone-brand-fg dark:bg-foreground/10'
          : 'font-medium text-sidebar-foreground/85 hover:bg-foreground/6 hover:text-foreground',
        props.class,
      )
    "
    @click="emit('click')"
  >
    <span class="flex min-w-0 items-center gap-2.5 truncate">
      <component
        :is="icon"
        v-if="icon"
        aria-hidden="true"
        :class="cn('size-4 shrink-0 transition-colors', active ? 'text-tone-brand-fg' : '')"
      />
      <span class="truncate">{{ label }}</span>
    </span>

    <StatusDot v-if="dot" :data-testid="dotTestId" :tone="dotTone" size="sm" />
  </button>
</template>
