<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = defineProps<{
  modelValue?: string | number
  placeholder?: string
  rows?: number
  class?: HTMLAttributes['class']
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', payload: string): void
}>()
</script>

<template>
  <textarea
    data-slot="textarea"
    :value="props.modelValue"
    :placeholder="props.placeholder"
    :rows="props.rows"
    spellcheck="false"
    :class="
      cn(
        'w-full min-w-0 rounded-lg border border-input bg-card px-3 py-2 text-body leading-relaxed text-foreground shadow-[var(--shadow-control)] transition-[border-color,box-shadow,background-color] duration-150 outline-none placeholder:text-muted-foreground/70 focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/20 disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 dark:bg-background/55 dark:focus-visible:bg-card',
        props.class,
      )
    "
    @input="emit('update:modelValue', ($event.target as HTMLTextAreaElement).value)"
  />
</template>
