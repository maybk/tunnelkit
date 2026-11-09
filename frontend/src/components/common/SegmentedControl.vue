<script setup lang="ts" generic="T extends string">
import type { HTMLAttributes, VNodeChild } from 'vue'
import { RadioGroupItem, RadioGroupRoot } from 'reka-ui'
import { cn } from '@/lib/utils'

export interface SegmentedOption<V extends string = string> {
  value: V
  label: string
  testId?: string
  disabled?: boolean
}

const modelValue = defineModel<T>({ required: true })
const props = withDefaults(
  defineProps<{
    options: ReadonlyArray<SegmentedOption<T>>
    variant?: 'card' | 'primary'
    size?: 'xs' | 'sm'
    disabled?: boolean
    ariaLabel?: string
    class?: HTMLAttributes['class']
  }>(),
  { variant: 'card', size: 'sm', disabled: false, ariaLabel: undefined, class: undefined },
)

defineSlots<{
  option?: (props: { option: SegmentedOption<T>; selected: boolean }) => VNodeChild
}>()

const selectedStyles = {
  card: 'data-[state=checked]:bg-card data-[state=checked]:text-foreground',
  primary: 'data-[state=checked]:bg-primary data-[state=checked]:text-primary-foreground',
} as const

const selectOption = (value: unknown) => {
  if (props.disabled) return
  const option = props.options.find((item) => item.value === value && !item.disabled)
  if (option) modelValue.value = option.value
}
</script>

<template>
  <RadioGroupRoot
    :model-value="modelValue"
    orientation="horizontal"
    :disabled="disabled"
    :aria-disabled="disabled || undefined"
    :aria-label="ariaLabel"
    :class="
      cn(
        'inline-flex h-8 shrink-0 items-center gap-0.5 rounded-lg bg-muted/90 p-0.5 ring-1 ring-black/[0.04] dark:ring-white/[0.06]',
        size === 'xs' ? 'text-micro' : 'text-caption',
        props.class,
      )
    "
    @update:model-value="selectOption"
  >
    <RadioGroupItem
      v-for="opt in options"
      :key="opt.value"
      :value="opt.value"
      :data-testid="opt.testId"
      :aria-label="opt.label"
      :disabled="opt.disabled"
      :class="
        cn(
          'inline-flex items-center justify-center gap-1 rounded-md text-muted-foreground transition-colors outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:shadow-[var(--shadow-card)]',
          size === 'xs' ? 'px-2 py-0.5 font-medium' : 'h-7 px-2.5 font-medium',
          selectedStyles[variant],
        )
      "
    >
      <slot name="option" :option="opt" :selected="modelValue === opt.value">{{ opt.label }}</slot>
    </RadioGroupItem>
  </RadioGroupRoot>
</template>
