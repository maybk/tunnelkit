<script setup lang="ts">
import { Loader2 } from '@lucide/vue'
import AppButton from '@/components/base/button/AppButton.vue'
defineProps<{ saving?: boolean; canCancel?: boolean }>()
const emit = defineEmits<{ cancel: [] }>()
</script>
<template>
  <div class="flex items-center justify-end gap-2.5 pt-1">
    <AppButton
      v-if="canCancel"
      type="button"
      variant="outline"
      :disabled="saving"
      data-testid="profile-cancel-btn"
      @click="emit('cancel')"
      >取消</AppButton
    >
    <AppButton type="submit" :disabled="saving" data-testid="profile-save-btn">
      <Loader2 v-if="saving" class="size-3.5 animate-spin" />
      创建连接
    </AppButton>
  </div>
</template>
