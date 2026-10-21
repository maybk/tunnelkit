<script setup lang="ts">
import { Check, Copy } from 'lucide-vue-next'
import { useClipboard } from '@vueuse/core'
import { Button } from '@/components/ui/button'

const props = defineProps<{
  text: string
  disabled?: boolean
}>()

const { copy, copied, isSupported } = useClipboard({ copiedDuring: 1500, legacy: true })
</script>

<template>
  <Button
    variant="outline"
    size="xs"
    :disabled="disabled || !isSupported || !props.text"
    @click="copy(props.text)"
  >
    <Check v-if="copied" class="size-3 text-status-ok" />
    <Copy v-else class="size-3" />
    {{ copied ? '已复制' : '复制' }}
  </Button>
</template>
