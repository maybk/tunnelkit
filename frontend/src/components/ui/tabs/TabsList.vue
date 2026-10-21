<script setup lang="ts">
import type { TabsListProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { TabsList } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = defineProps<TabsListProps & { class?: HTMLAttributes['class'] }>()
const delegatedProps = reactiveOmit(props, 'class')
</script>

<template>
  <!-- 外层 rounded-xl (12px) = 内层 TabsTrigger rounded-lg (8px) + p-1 (4px)，严格同心圆角 -->
  <TabsList
    data-slot="tabs-list"
    v-bind="delegatedProps"
    :class="
      cn(
        'inline-flex h-9 w-fit items-center justify-center gap-0.5 rounded-xl bg-muted/90 p-1 text-muted-foreground ring-1 ring-black/[0.04] dark:ring-white/[0.06]',
        props.class,
      )
    "
  >
    <slot />
  </TabsList>
</template>
