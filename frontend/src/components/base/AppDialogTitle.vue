<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import type { DialogTitleProps } from 'reka-ui'
import { useForwardProps, useForwardExpose } from 'reka-ui'
import { reactiveOmit } from '@vueuse/core'
import { DialogTitle } from '@/components/ui/dialog'
import { cn } from '@/lib/utils'

const props = defineProps<DialogTitleProps & { class?: HTMLAttributes['class'] }>()

const forwarded = useForwardProps(reactiveOmit(props, 'class'))
const { forwardRef } = useForwardExpose()
</script>

<template>
  <DialogTitle
    :ref="forwardRef"
    v-bind="forwarded"
    :class="cn('text-title font-semibold text-foreground leading-normal', props.class)"
  >
    <slot />
  </DialogTitle>
</template>
