<script setup lang="ts">
import type { SwitchRootEmits, SwitchRootProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { SwitchRoot, SwitchThumb, useForwardExpose, useForwardPropsEmits } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<
    SwitchRootProps & {
      class?: HTMLAttributes['class']
      size?: 'sm' | 'default'
    }
  >(),
  {
    size: 'default',
  },
)

const emits = defineEmits<SwitchRootEmits>()
const delegatedProps = reactiveOmit(props, 'class', 'size')
const forwarded = useForwardPropsEmits(delegatedProps, emits)
const { forwardRef } = useForwardExpose()
</script>

<template>
  <SwitchRoot
    :ref="forwardRef"
    v-slot="slotProps"
    data-slot="switch"
    :data-size="size"
    v-bind="forwarded"
    :class="
      cn(
        'peer inline-flex shrink-0 cursor-pointer items-center rounded-full border border-transparent transition-colors duration-150 outline-none focus-visible:ring-2 focus-visible:ring-ring/40 disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:bg-primary data-[state=unchecked]:bg-muted-foreground/25 dark:data-[state=unchecked]:bg-muted-foreground/35',
        size === 'sm' ? 'h-4.5 w-8 p-0.5' : 'h-5.5 w-10 p-0.5',
        props.class,
      )
    "
    @click.stop
  >
    <SwitchThumb
      data-slot="switch-thumb"
      :class="
        cn(
          'pointer-events-none block rounded-full bg-card shadow-xs transition-transform duration-150 transform-gpu backface-hidden dark:data-[state=checked]:bg-primary-foreground dark:data-[state=unchecked]:bg-foreground',
          size === 'sm'
            ? 'size-3.5 data-[state=checked]:translate-x-3.5 data-[state=unchecked]:translate-x-0'
            : 'size-4.5 data-[state=checked]:translate-x-4.5 data-[state=unchecked]:translate-x-0',
        )
      "
    >
      <slot name="thumb" v-bind="slotProps" />
    </SwitchThumb>
  </SwitchRoot>
</template>
