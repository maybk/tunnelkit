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
  <div class="h-full overflow-y-auto px-6 pt-4 pb-6">
    <div class="mx-auto w-full max-w-2xl space-y-4">
      <div class="px-0.5">
        <h2
          v-if="!profilesStore.profiles.length"
          class="text-heading font-semibold tracking-tight text-foreground"
        >
          添加第一个 VPN 连接
        </h2>
        <p
          class="text-caption text-muted-foreground"
          :class="!profilesStore.profiles.length ? 'mt-1' : ''"
        >
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
