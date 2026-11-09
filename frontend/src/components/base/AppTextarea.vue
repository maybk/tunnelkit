<script setup lang="ts">
import type { HTMLAttributes } from 'vue'

import { useForwardPropsEmits, useForwardExpose } from 'reka-ui'
import { reactiveOmit } from '@vueuse/core'
import { Textarea } from '@/components/ui/textarea'
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
  <Textarea
    :ref="forwardRef"
    v-bind="forwarded"
    spellcheck="false"
    :class="
      cn(
        'w-full min-w-0 rounded-lg border border-input bg-card px-3 py-2 text-body leading-relaxed text-foreground shadow-[var(--shadow-control)] transition-[border-color,box-shadow,background-color] duration-150 outline-none placeholder:text-muted-foreground/70 focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/20 disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 dark:bg-background/55 dark:focus-visible:bg-card md:text-body block field-sizing-fixed min-h-0 disabled:bg-card dark:disabled:bg-background/55',
        props.class,
      )
    "
  >
    <slot />
  </Textarea>
</template>
