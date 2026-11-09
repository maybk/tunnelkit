<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import type { TabsListProps } from 'reka-ui'
import { useForwardProps, useForwardExpose } from 'reka-ui'
import { reactiveOmit } from '@vueuse/core'
import { TabsList } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'

const props = defineProps<TabsListProps & { class?: HTMLAttributes['class'] }>()

const forwarded = useForwardProps(reactiveOmit(props, 'class'))
const { forwardRef } = useForwardExpose()
</script>

<template>
  <TabsList
    :ref="forwardRef"
    v-bind="forwarded"
    :class="
      cn(
        'inline-flex h-9 w-fit items-center justify-center gap-0.5 rounded-xl bg-muted/90 p-1 text-muted-foreground ring-1 ring-black/[0.04] dark:ring-white/[0.06] group-data-horizontal/tabs:h-9',
        props.class,
      )
    "
  >
    <slot />
  </TabsList>
</template>
