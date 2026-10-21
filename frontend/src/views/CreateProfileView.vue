<script setup lang="ts">
import type { ProfileInput } from '@/api/profile'
import ProfileForm from '@/components/profile/ProfileForm.vue'

defineProps<{
  firstProfile: boolean
  saving: boolean
  formError: string
}>()

const emit = defineEmits<{
  saveProfile: [input: ProfileInput]
  cancel: []
}>()
</script>

<template>
  <div class="h-full overflow-y-auto px-6 pb-6">
    <div class="mx-auto w-full max-w-2xl space-y-4">
      <div class="px-0.5 pt-1">
        <h1 class="text-heading font-semibold tracking-tight text-foreground">
          {{ firstProfile ? '添加第一个 VPN 连接' : '新建 VPN 连接' }}
        </h1>
        <p class="mt-1 text-caption text-muted-foreground">
          填写网关地址与账号信息，或切换到「配置文本」直接粘贴现有配置。
        </p>
      </div>

      <ProfileForm
        :profile="null"
        :saving="saving"
        :error="formError"
        :can-cancel="!firstProfile"
        @save="emit('saveProfile', $event)"
        @cancel="emit('cancel')"
      />
    </div>
  </div>
</template>
