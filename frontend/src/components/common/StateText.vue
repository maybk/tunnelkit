<script setup lang="ts">
withDefaults(defineProps<{ text: string; transitionKey?: string }>(), { transitionKey: undefined })
</script>

<template>
  <span class="inline-grid items-center align-middle">
    <Transition
      enter-active-class="transition-opacity duration-150 ease-out"
      enter-from-class="opacity-65"
      @leave="(_el, done) => done()"
    >
      <span :key="transitionKey ?? text" class="[grid-area:1/1]">{{ text }}</span>
    </Transition>
  </span>
</template>
