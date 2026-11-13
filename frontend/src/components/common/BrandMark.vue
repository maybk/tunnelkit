<script setup lang="ts">
import brandMarkDark from '@/assets/brand/brand-mark-dark.svg'
import brandMarkLight from '@/assets/brand/brand-mark-light.svg'

withDefaults(defineProps<{ label?: string }>(), { label: 'TunnelKit' })
</script>

<template>
  <!-- 透明品牌标识：随 html.dark 切换配色，深色版提亮低对比度色阶 -->
  <span class="inline-flex shrink-0" role="img" :aria-label="label">
    <img :src="brandMarkLight" alt="" draggable="false" class="size-full dark:hidden" />
    <img :src="brandMarkDark" alt="" draggable="false" class="hidden size-full dark:block" />
  </span>
</template>
