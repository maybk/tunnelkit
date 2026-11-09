<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import { Check, Copy } from '@lucide/vue'
import AppButton from '@/components/base/button/AppButton.vue'
import { useClipboard } from '@vueuse/core'

const props = withDefaults(
  defineProps<{
    text: string
    label?: string
    title?: string
    class?: string
  }>(),
  {
    label: '复制',
    title: '复制到剪贴板',
    class: '',
  },
)

const copied = ref(false)
const { copy, isSupported } = useClipboard({ legacy: true })
let copyTimer: ReturnType<typeof setTimeout> | null = null

const handleCopy = async () => {
  if (!props.text) return
  try {
    await copy(props.text)
    copied.value = true
    if (copyTimer) clearTimeout(copyTimer)
    copyTimer = setTimeout(() => {
      copied.value = false
      copyTimer = null
    }, 1500)
  } catch {
    copied.value = false
  }
}

onBeforeUnmount(() => {
  if (copyTimer) {
    clearTimeout(copyTimer)
    copyTimer = null
  }
})
</script>

<template>
  <AppButton
    type="button"
    :title="copied ? '已复制' : props.title"
    :aria-label="copied ? '已复制' : props.title"
    :disabled="!props.text || !isSupported"
    data-testid="copy-button"
    variant="outline"
    size="xs"
    :class="props.class"
    @click.stop="handleCopy"
  >
    <Check v-if="copied" class="size-3 shrink-0" />
    <Copy v-else class="size-3 shrink-0" />
    <span v-if="props.label || copied">{{ copied ? '已复制' : props.label }}</span>
  </AppButton>
</template>
