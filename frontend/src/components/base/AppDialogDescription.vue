<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import type { DialogDescriptionProps } from 'reka-ui'
import { useForwardProps, useForwardExpose } from 'reka-ui'
import { reactiveOmit } from '@vueuse/core'
import { DialogDescription } from '@/components/ui/dialog'
import { cn } from '@/lib/utils'

const props = defineProps<DialogDescriptionProps & { class?: HTMLAttributes['class'] }>()

const forwarded = useForwardProps(reactiveOmit(props, 'class'))
const { forwardRef } = useForwardExpose()
</script>

<template>
  <DialogDescription
    :ref="forwardRef"
    v-bind="forwarded"
    :class="cn('text-body text-muted-foreground', props.class)"
  >
    <slot />
  </DialogDescription>
</template>
