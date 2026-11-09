<script setup lang="ts">
import type { HTMLAttributes } from 'vue'

import { useForwardPropsEmits, useForwardExpose } from 'reka-ui'
import { reactiveOmit } from '@vueuse/core'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

const props = defineProps<{
  defaultValue?: string | number
  modelValue?: string | number
  class?: HTMLAttributes['class']
}>()
const emit = defineEmits<{ 'update:modelValue': [value: string | number] }>()
const forwarded = useForwardPropsEmits(reactiveOmit(props, 'class'), emit)
const { forwardRef } = useForwardExpose()
</script>

<template>
  <Input
    :ref="forwardRef"
    v-bind="forwarded"
    :class="
      cn(
        'h-9 w-full min-w-0 rounded-lg border border-input bg-card px-3 py-1.5 text-body text-foreground shadow-[var(--shadow-control)] transition-[border-color,box-shadow,background-color] duration-150 outline-none placeholder:text-muted-foreground/70 focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/20 disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 dark:bg-background/55 dark:focus-visible:bg-card md:text-body disabled:bg-card dark:disabled:bg-background/55',
        props.class,
      )
    "
  >
    <slot />
  </Input>
</template>
