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
</script>

<template>
  <section class="space-y-1.5">
    <div v-if="draft.auth === 'password'" class="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
      <FieldGroup label="用户名" :error="errors.username">
        <template #default="{ fieldId }">
          <AppInput
            :id="fieldId"
            v-model="draft.username"
            :disabled="disabled"
            autocomplete="username"
            placeholder="输入 VPN 用户名"
            data-testid="profile-username-input"
          />
        </template>
      </FieldGroup>

      <FieldGroup label="密码" :error="errors.password">
        <template #actions>
          <span
            v-if="draft.hasPassword"
            class="rounded bg-tone-success-solid/14 px-1.5 py-0.5 text-[10.5px] font-medium text-tone-success-solid"
            >已加密保存</span
          >
        </template>
        <template #default="{ fieldId }">
          <AppInput
            :id="fieldId"
            v-model="draft.password"
            type="password"
            :disabled="disabled"
            autocomplete="current-password"
            :placeholder="draft.hasPassword ? '留空则保持原密码' : '输入登录密码'"
            data-testid="profile-password-input"
          />
        </template>
      </FieldGroup>
    </div>
  </section>
</template>
