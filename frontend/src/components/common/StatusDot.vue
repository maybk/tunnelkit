<script setup lang="ts">
import type { StatusTone } from '@/lib/status'

defineProps<{
  tone: StatusTone
}>()

const dotClass: Record<StatusTone, string> = {
  idle: 'bg-status-idle/75',
  busy: 'bg-status-busy shadow-[0_0_0_3px_color-mix(in_oklch,var(--status-busy)_22%,transparent)] animate-pulse',
  ok: 'bg-status-ok shadow-[0_0_0_3px_color-mix(in_oklch,var(--status-ok)_22%,transparent)]',
  danger:
    'bg-status-danger shadow-[0_0_0_3px_color-mix(in_oklch,var(--status-danger)_22%,transparent)]',
}
</script>

<template>
  <span
    aria-hidden="true"
    class="inline-block size-2 shrink-0 rounded-full transition-[background-color,box-shadow] duration-200"
    :class="dotClass[tone]"
  />
</template>
