<script setup lang="ts">
import FieldGroup from '@/components/common/FieldGroup.vue'
import AppInput from '@/components/base/AppInput.vue'
import { AUTH_TYPE_OPTIONS } from '@/constants/profile'
import type { ProfileDraft } from '@/types/profile'
import type { ProfileFieldErrors } from '@/validate/profile'
const draft = defineModel<ProfileDraft>('draft', { required: true })
defineProps<{ errors: ProfileFieldErrors; disabled?: boolean }>()
</script>
<template>
  <section class="space-y-4">
    <div class="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
      <FieldGroup label="认证方式">
        <template #default="{ fieldId }">
          <select
            :id="fieldId"
            v-model="draft.auth"
            :disabled="disabled"
            class="flex h-9 w-full rounded-lg border border-input bg-card px-3 py-1 text-body text-foreground shadow-2xs focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
          >
            <option v-for="option in AUTH_TYPE_OPTIONS" :key="option.value" :value="option.value">
              {{ option.label }}
            </option>
          </select>
        </template>
      </FieldGroup>
      <FieldGroup label="Realm 认证域" optional>
        <template #default="{ fieldId }">
          <AppInput
            :id="fieldId"
            v-model="draft.realm"
            :disabled="disabled"
            placeholder="默认留空"
            class="font-mono"
          />
        </template>
      </FieldGroup>
    </div>
    <div class="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
      <FieldGroup label="服务器证书指纹" :error="errors.trustedCert" optional>
        <template #default="{ fieldId }">
          <AppInput
            :id="fieldId"
            v-model="draft.trustedCert"
            :disabled="disabled"
            placeholder="留空则使用系统根证书库验证"
            class="font-mono text-caption"
          />
        </template>
      </FieldGroup>

      <FieldGroup label="自定义 User-Agent" optional>
        <template #default="{ fieldId }">
          <AppInput
            :id="fieldId"
            v-model="draft.userAgent"
            :disabled="disabled"
            placeholder="Mozilla/5.0 SV1"
            class="font-mono text-caption"
          />
        </template>
      </FieldGroup>
    </div>

    <div class="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
      <FieldGroup label="客户端证书路径" :error="errors.clientCert" optional>
        <template #default="{ fieldId }">
          <AppInput
            :id="fieldId"
            v-model="draft.clientCert"
            :disabled="disabled"
            placeholder="/path/to/client.crt"
            class="font-mono"
          />
        </template>
      </FieldGroup>

      <FieldGroup label="客户端私钥路径" optional>
        <template #default="{ fieldId }">
          <AppInput
            :id="fieldId"
            v-model="draft.clientKey"
            :disabled="disabled"
            placeholder="/path/to/client.key"
            class="font-mono"
          />
        </template>
      </FieldGroup>
    </div>
    <label class="flex cursor-pointer items-start gap-2.5 pt-0.5 text-body text-foreground">
      <input
        v-model="draft.legacyTls"
        :disabled="disabled"
        type="checkbox"
        class="mt-0.5 size-4 rounded border-border accent-primary"
      />
      <span class="space-y-0.5"
        ><span class="block leading-tight font-medium">兼容旧版 VPN 网关</span
        ><span class="block text-caption text-muted-foreground"
          >连接老旧设备提示协议或握手错误时开启（允许 TLS 1.0/1.1 与传统加密算法）</span
        ></span
      >
    </label>
  </section>
</template>
