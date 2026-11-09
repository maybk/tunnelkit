<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import type { LabelProps } from 'reka-ui'
import { useForwardProps, useForwardExpose } from 'reka-ui'
import { reactiveOmit } from '@vueuse/core'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'

const props = defineProps<LabelProps & { class?: HTMLAttributes['class'] }>()

const forwarded = useForwardProps(reactiveOmit(props, 'class'))
const { forwardRef } = useForwardExpose()
</script>

<template>
  <Label
    :ref="forwardRef"
    v-bind="forwarded"
    :class="
      cn(
        'flex items-center gap-1.5 text-body font-medium text-foreground select-none peer-disabled:cursor-not-allowed peer-disabled:opacity-50 leading-normal',
        props.class,
      )
    "
  >
    <slot />
  </Label>
</template>
