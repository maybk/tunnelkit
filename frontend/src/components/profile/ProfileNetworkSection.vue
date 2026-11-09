<script setup lang="ts">
import FieldGroup from '@/components/common/FieldGroup.vue'
import AppInput from '@/components/base/AppInput.vue'
import { DNS_MODE_OPTIONS, ROUTE_MODE_OPTIONS } from '@/constants/profile'
import type { ProfileDraft } from '@/types/profile'
import type { ProfileFieldErrors } from '@/validate/profile'

const draft = defineModel<ProfileDraft>('draft', { required: true })

defineProps<{
  errors: ProfileFieldErrors
  disabled?: boolean
}>()
</script>

<template>
  <section class="space-y-3.5">
    <div class="grid grid-cols-1 gap-3.5 sm:grid-cols-2 border-t border-border/60 pt-3.5">
      <FieldGroup label="路由策略">
        <template #default="{ fieldId }">
          <select
            :id="fieldId"
            v-model="draft.routes"
            :disabled="disabled"
            data-testid="profile-routes-select"
            class="flex h-9 w-full rounded-lg border border-input bg-card px-3 py-1 text-body text-foreground shadow-2xs transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
          >
            <option v-for="option in ROUTE_MODE_OPTIONS" :key="option.value" :value="option.value">
              {{ option.label }}
            </option>
          </select>
        </template>
      </FieldGroup>

      <FieldGroup label="DNS 策略">
        <template #default="{ fieldId }">
          <select
            :id="fieldId"
            v-model="draft.dns"
            :disabled="disabled"
            data-testid="profile-dns-select"
            class="flex h-9 w-full rounded-lg border border-input bg-card px-3 py-1 text-body text-foreground shadow-2xs transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
          >
            <option v-for="option in DNS_MODE_OPTIONS" :key="option.value" :value="option.value">
              {{ option.label }}
            </option>
          </select>
        </template>
      </FieldGroup>
    </div>

    <div class="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
      <FieldGroup label="附加内网网段" :error="errors.extraRoutes" optional>
        <template #default="{ fieldId }">
          <AppInput
            :id="fieldId"
            v-model="draft.extraRoutesText"
            :disabled="disabled"
            placeholder="10.10.0.0/16, 172.16.10.0/24"
            class="font-mono text-caption"
          />
        </template>
      </FieldGroup>

      <FieldGroup label="内网域名后缀" optional>
        <template #default="{ fieldId }">
          <AppInput
            :id="fieldId"
            v-model="draft.dnsDomainsText"
            :disabled="disabled"
            placeholder="corp.internal, svc.local"
            class="font-mono text-caption"
          />
        </template>
      </FieldGroup>
    </div>

    <label class="flex cursor-pointer items-center gap-2 pt-0.5 text-body text-foreground">
      <input
        v-model="draft.reconnect"
        :disabled="disabled"
        type="checkbox"
        class="size-4 rounded border-border accent-primary"
      />
      网络波动或休眠唤醒后自动重连
    </label>
  </section>
</template>
