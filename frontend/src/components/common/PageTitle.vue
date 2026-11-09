<script setup lang="ts">
defineProps<{
  title: string
  parent?: string
}>()
</script>

<template>
  <h1
    data-testid="page-title"
    class="flex min-w-0 items-center gap-1.5 truncate text-title font-bold tracking-tight text-foreground"
  >
    <template v-if="parent">
      <span class="truncate font-semibold text-muted-foreground">{{ parent }}</span>
      <span class="text-muted-foreground/50" aria-hidden="true">/</span>
    </template>
    <span class="truncate">{{ title }}</span>
  </h1>
</template>
