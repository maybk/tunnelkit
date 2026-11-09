<script setup lang="ts">
import FieldGroup from '@/components/common/FieldGroup.vue'
import AppInput from '@/components/base/AppInput.vue'
import type { ProfileDraft } from '@/types/profile'
import type { ProfileFieldErrors } from '@/validate/profile'

const draft = defineModel<ProfileDraft>('draft', { required: true })

defineProps<{
  errors: ProfileFieldErrors
  disabled?: boolean
}>()

const emit = defineEmits<{
  blurHost: []
}>()
</script>

<template>
  <section class="space-y-3">
    <div class="grid grid-cols-3 gap-3.5">
      <FieldGroup label="网关地址" class="col-span-2" :error="errors.host">
        <template #default="{ fieldId }">
          <AppInput
            :id="fieldId"
            v-model="draft.host"
            :disabled="disabled"
            placeholder="vpn.example.com"
            data-testid="profile-host-input"
            class="font-mono"
            @blur="emit('blurHost')"
          />
        </template>
      </FieldGroup>

      <FieldGroup label="端口" :error="errors.port">
        <template #default="{ fieldId }">
          <AppInput
            :id="fieldId"
            v-model.number="draft.port"
            type="number"
            min="1"
            max="65535"
            :disabled="disabled"
            data-testid="profile-port-input"
            class="font-mono tabular-nums"
          />
        </template>
      </FieldGroup>
    </div>
  </section>
</template>
