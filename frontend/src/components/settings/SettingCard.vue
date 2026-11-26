<script setup lang="ts">
import AppCard from '@/components/base/card/AppCard.vue'

withDefaults(
  defineProps<{
    title?: string
    padded?: boolean
  }>(),
  {
    title: undefined,
    padded: true,
  },
)
</script>

<template>
  <section class="space-y-2">
    <h2 v-if="title" class="px-1 text-caption font-semibold text-muted-foreground">
      {{ title }}
    </h2>

    <AppCard variant="divided" padding="none" :class="padded ? 'px-5' : ''">
      <slot />
    </AppCard>
  </section>
</template>
