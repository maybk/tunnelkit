<script setup lang="ts">
import { AlertCircle, X } from 'lucide-vue-next'

defineProps<{
  message: string
}>()

const emit = defineEmits<{
  dismiss: []
}>()
</script>

<template>
  <Transition
    enter-active-class="transition-[transform,opacity] duration-200 ease-out"
    enter-from-class="translate-y-2 opacity-0"
    leave-active-class="transition-[transform,opacity] duration-150 ease-out"
    leave-to-class="translate-y-1.5 opacity-0"
  >
    <div
      v-if="message"
      role="alert"
      class="surface-card fixed right-5 bottom-5 z-50 flex max-w-sm items-start gap-2.5 rounded-xl bg-popover px-4 py-3 text-body text-popover-foreground"
    >
      <AlertCircle class="mt-0.5 size-4 shrink-0 text-destructive" aria-hidden="true" />
      <span class="min-w-0 flex-1 leading-snug break-words">{{ message }}</span>
      <button
        type="button"
        class="shrink-0 rounded-md p-0.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        aria-label="关闭提示"
        @click="emit('dismiss')"
      >
        <X class="size-3.5" aria-hidden="true" />
      </button>
    </div>
  </Transition>
</template>
