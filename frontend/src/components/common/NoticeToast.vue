<script setup lang="ts">
import { AlertCircle, AlertTriangle, CheckCircle2, Info, X } from '@lucide/vue'
import { storeToRefs } from 'pinia'
import { computed } from 'vue'
import { useNoticeStore } from '@/stores/useNoticeStore'

const noticeStore = useNoticeStore()
const { items } = storeToRefs(noticeStore)
const visibleItem = computed(() => items.value.at(-1))
</script>

<template>
  <div
    aria-live="polite"
    class="pointer-events-none fixed right-5 bottom-5 z-100 flex max-w-sm flex-col gap-2"
  >
    <Transition
      enter-active-class="transition-[translate,opacity] duration-200 ease-out"
      enter-from-class="translate-y-2 opacity-0"
      leave-active-class="fixed right-5 bottom-5 transition-[translate,opacity] duration-150 ease-out"
      leave-to-class="translate-y-1.5 opacity-0"
    >
      <div
        v-if="visibleItem"
        :key="visibleItem.id"
        data-testid="notice-toast-item"
        class="surface-card pointer-events-auto flex items-start gap-2.5 max-w-[min(24rem,calc(100vw-2.5rem))] rounded-xl bg-popover px-4 py-3 text-body text-popover-foreground"
      >
        <CheckCircle2
          v-if="visibleItem.tone === 'success'"
          class="mt-0.5 size-4 shrink-0 text-tone-success-fg"
        />
        <AlertCircle
          v-else-if="visibleItem.tone === 'danger'"
          class="mt-0.5 size-4 shrink-0 text-tone-danger-fg"
        />
        <AlertTriangle
          v-else-if="visibleItem.tone === 'warning'"
          class="mt-0.5 size-4 shrink-0 text-tone-warning-fg"
        />
        <Info v-else class="mt-0.5 size-4 shrink-0 text-tone-info-fg" />

        <div class="min-w-0 flex-1">
          <p class="text-body leading-snug text-foreground">{{ visibleItem.title }}</p>
          <p v-if="visibleItem.message" class="mt-0.5 text-body leading-snug text-muted-foreground">
            {{ visibleItem.message }}
          </p>
        </div>

        <button
          type="button"
          aria-label="关闭通知"
          class="rounded-md p-0.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          @click="noticeStore.dismiss(visibleItem.id)"
        >
          <X class="size-3.5" />
        </button>
      </div>
    </Transition>
  </div>
</template>
