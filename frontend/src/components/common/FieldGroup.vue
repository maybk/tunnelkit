<script setup lang="ts">
import { useId, type HTMLAttributes } from 'vue'
import AppLabel from '@/components/base/AppLabel.vue'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    label: string
    forId?: string
    description?: string
    error?: string
    optional?: boolean
    required?: boolean
    class?: HTMLAttributes['class']
  }>(),
  {
    forId: undefined,
    description: undefined,
    error: undefined,
    optional: false,
    required: false,
    class: undefined,
  },
)

const fieldId = useId()
</script>

<template>
  <div :class="cn('min-w-0 space-y-1.5', props.class)">
    <div class="flex items-center justify-between gap-2">
      <div class="flex items-center gap-1.5">
        <AppLabel :for="forId ?? fieldId" class="text-body font-medium text-foreground">
          {{ label }}
        </AppLabel>
        <span v-if="required" class="text-micro font-semibold text-tone-danger-fg">*</span>
        <span v-else-if="optional" class="text-body font-medium text-foreground">（可选）</span>
      </div>
      <slot name="actions" />
    </div>
    <slot :field-id="forId ?? fieldId" />
    <p v-if="error" role="alert" class="text-micro font-medium text-tone-danger-fg">
      {{ error }}
    </p>
    <p v-else-if="description" class="text-micro text-muted-foreground">
      {{ description }}
    </p>
  </div>
</template>
