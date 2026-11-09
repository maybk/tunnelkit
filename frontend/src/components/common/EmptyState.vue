<script setup lang="ts">
import type { Component } from 'vue'

defineProps<{
  icon?: Component
  title: string
  description?: string
}>()
</script>

<template>
  <div class="flex h-full flex-col items-center justify-center p-8 text-center">
    <div
      v-if="icon"
      class="mb-4 flex size-12 items-center justify-center rounded-2xl border border-border/70 bg-card text-muted-foreground shadow-2xs"
    >
      <component :is="icon" class="size-5" />
    </div>
    <h2 class="text-title font-semibold text-foreground">{{ title }}</h2>
    <p v-if="description" class="mt-1.5 max-w-sm text-caption text-muted-foreground">
      {{ description }}
    </p>
    <div v-if="$slots.actions" class="mt-5 flex items-center gap-2.5">
      <slot name="actions" />
    </div>
  </div>
</template>
