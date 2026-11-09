<script setup lang="ts">
import { computed, type HTMLAttributes } from 'vue'
import { TONE_CLASSES, type Tone } from '@/constants/tone'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    tone?: Extract<Tone, 'danger' | 'warning' | 'info' | 'success' | 'neutral'>
    message?: string
    class?: HTMLAttributes['class']
  }>(),
  {
    tone: 'danger',
    message: '',
    class: undefined,
  },
)

const panelClass = computed(() => TONE_CLASSES[props.tone]?.panel ?? TONE_CLASSES.danger.panel)
</script>

<template>
  <div
    v-if="message || $slots.default"
    :role="tone === 'danger' || tone === 'warning' ? 'alert' : 'status'"
    :class="cn('rounded-xl border px-3.5 py-2 text-caption', panelClass, props.class)"
  >
    <slot>{{ message }}</slot>
  </div>
</template>
