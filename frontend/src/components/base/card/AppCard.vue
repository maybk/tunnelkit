<script setup lang="ts">
import type { PrimitiveProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import type { CardVariants } from './variants'
import { Primitive } from 'reka-ui'
import { cn } from '@/lib/utils'
import { cardVariants } from './variants'

interface Props extends PrimitiveProps {
  variant?: CardVariants['variant']
  padding?: CardVariants['padding']
  active?: boolean
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), {
  as: 'div',
  variant: 'elevated',
  padding: 'md',
  active: false,
})
</script>

<template>
  <Primitive
    data-slot="card"
    :data-variant="variant"
    :data-active="active ? 'true' : undefined"
    :as="as"
    :as-child="asChild"
    :class="
      cn(
        cardVariants({ variant, padding }),
        active && 'bg-tone-brand-soft/20 ring-1 ring-tone-brand-border',
        props.class,
      )
    "
  >
    <slot />
  </Primitive>
</template>
