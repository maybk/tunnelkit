<script setup lang="ts">
import { getToneClasses, type Tone } from '@/constants/tone'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    tone?: Tone
    size?: 'sm' | 'md'
    pulse?: boolean
  }>(),
  { tone: 'neutral', size: 'md', pulse: false },
)
</script>

<template>
  <span
    aria-hidden="true"
    :class="
      cn(
        'inline-block shrink-0 rounded-full transition-[background-color,box-shadow] duration-200',
        size === 'sm' ? 'size-1.5' : 'size-2',
        getToneClasses(tone).dot,
        props.pulse && 'motion-safe:animate-pulse',
      )
    "
  />
</template>
