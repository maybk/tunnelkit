<script setup lang="ts">
import { computed } from 'vue'
import { TONE_CLASSES, type Tone } from '@/constants/tone'
import { cn } from '@/lib/utils'
import StatusDot from './StatusDot.vue'

const props = withDefaults(
  defineProps<{
    tone?: Tone
    dot?: boolean
    pulse?: boolean
    mono?: boolean
    size?: 'xs' | 'sm'
    class?: string
  }>(),
  {
    tone: 'neutral',
    dot: false,
    pulse: false,
    mono: false,
    size: 'xs',
    class: '',
  },
)

const toneSet = computed(() => TONE_CLASSES[props.tone] ?? TONE_CLASSES.neutral)
</script>

<template>
  <span
    :class="
      cn(
        'inline-flex shrink-0 items-center gap-1.5 rounded-md border font-medium whitespace-nowrap transition-colors',
        props.size === 'sm' ? 'px-2 py-0.5 text-caption' : 'px-1.5 py-0.5 text-micro',
        props.mono && 'font-mono tabular-nums',
        toneSet.badge,
        props.class,
      )
    "
  >
    <StatusDot v-if="props.dot" :tone="tone" :pulse="pulse" size="sm" />
    <slot />
  </span>
</template>
