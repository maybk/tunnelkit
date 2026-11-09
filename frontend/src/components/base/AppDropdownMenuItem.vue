<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import type { DropdownMenuItemProps } from 'reka-ui'
import { useForwardProps, useForwardExpose } from 'reka-ui'
import { reactiveOmit } from '@vueuse/core'
import { DropdownMenuItem } from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

const props = defineProps<
  DropdownMenuItemProps & {
    class?: HTMLAttributes['class']
    variant?: 'default' | 'destructive'
    inset?: boolean
  }
>()

const forwarded = useForwardProps(reactiveOmit(props, 'class'))
const { forwardRef } = useForwardExpose()
</script>

<template>
  <DropdownMenuItem
    :ref="forwardRef"
    v-bind="forwarded"
    :class="
      cn(
        'focus:bg-accent focus:text-accent-foreground data-[variant=destructive]:text-destructive data-[variant=destructive]:focus:bg-destructive/10 data-[variant=destructive]:focus:text-destructive relative flex cursor-pointer items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-body transition-colors outline-none select-none data-[disabled]:pointer-events-none data-[disabled]:opacity-45 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*=size-])]:size-4',
        props.class,
      )
    "
  >
    <slot />
  </DropdownMenuItem>
</template>
