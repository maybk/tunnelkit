<script setup lang="ts">
import { AlertDialogCancel as AlertDialogCancelPrimitive } from 'reka-ui'
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import AppButton from '@/components/base/button/AppButton.vue'

const open = defineModel<boolean>('open', { required: true })

withDefaults(
  defineProps<{
    title: string
    description?: string
    confirmText?: string
    cancelText?: string
    variant?: 'destructive' | 'default'
    confirmTestId?: string
  }>(),
  {
    description: undefined,
    confirmText: '确认',
    cancelText: '取消',
    variant: 'destructive',
    confirmTestId: undefined,
  },
)

const emit = defineEmits<{
  confirm: []
  cancel: []
}>()

const handleConfirm = () => {
  emit('confirm')
  open.value = false
}

const handleCancel = () => {
  emit('cancel')
  open.value = false
}
</script>

<template>
  <AlertDialog v-model:open="open">
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>{{ title }}</AlertDialogTitle>
        <AlertDialogDescription v-if="description || $slots.default">
          <slot>{{ description }}</slot>
        </AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancelPrimitive as-child>
          <AppButton variant="secondary" size="sm" @click="handleCancel">
            {{ cancelText }}
          </AppButton>
        </AlertDialogCancelPrimitive>
        <AppButton :variant="variant" size="sm" :data-testid="confirmTestId" @click="handleConfirm">
          {{ confirmText }}
        </AppButton>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>
</template>
