<script setup lang="ts">
import { useRouter } from 'vue-router'
import ProfileFormPanel from '@/components/profile/ProfileFormPanel.vue'
import { ROUTE_NAMES } from '@/router'
import { useProfilesStore } from '@/stores/useProfilesStore'
import type { Profile } from '@/types/profile'

const router = useRouter()
const profilesStore = useProfilesStore()

const handleSaved = (saved: Profile) => {
  profilesStore.selectProfile(saved.id)
  profilesStore.setActiveTab('status')
  void router.push({ name: ROUTE_NAMES.workspace })
}

const handleCancel = () => {
  void router.push({ name: ROUTE_NAMES.workspace })
}
</script>

<template>
  <div class="h-full overflow-y-auto px-6 pb-6">
    <div class="mx-auto w-full max-w-2xl space-y-4">
      <div class="px-0.5 pt-1">
        <h1 class="text-heading font-semibold tracking-tight text-foreground">
          {{ profilesStore.profiles.length ? '新建 VPN 连接' : '添加第一个 VPN 连接' }}
        </h1>
        <p class="mt-1 text-caption text-muted-foreground">
          填写网关地址与账号信息，或切换到「配置文本」直接粘贴现有配置。
        </p>
      </div>
      <ProfileFormPanel
        is-create
        :can-cancel="profilesStore.profiles.length > 0"
        @saved="handleSaved"
        @cancel="handleCancel"
      />
    </div>
  </div>
</template>
