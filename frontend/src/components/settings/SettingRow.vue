<script setup lang="ts">
withDefaults(
  defineProps<{
    label: string
    description?: string
    vertical?: boolean
  }>(),
  {
    description: undefined,
    vertical: false,
  },
)
</script>

<template>
  <div
    :class="
      vertical ? 'flex flex-col gap-2.5 py-4' : 'flex items-center justify-between gap-4 py-3.5'
    "
  >
    <div class="min-w-0">
      <div class="text-body font-medium text-foreground">{{ label }}</div>
      <p v-if="description" class="mt-0.5 text-caption text-muted-foreground">
        {{ description }}
      </p>
    </div>

    <div :class="vertical ? 'w-full' : 'shrink-0'">
      <slot />
    </div>
  </div>
</template>
