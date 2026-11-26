<script setup lang="ts">
import { onMounted } from 'vue'
import SettingsSidebar from '@/components/settings/SettingsSidebar.vue'
import { useSystemStore } from '@/stores/useSystemStore'

const systemStore = useSystemStore()

onMounted(() => {
  if (!systemStore.systemInfo && !systemStore.checkingSystem) {
    void systemStore.loadSystemInfo()
  }
})
</script>

<template>
  <div class="flex h-full w-full overflow-hidden bg-background">
    <SettingsSidebar :has-update="systemStore.hasUpdate" />
    <div
      data-testid="settings-content"
      class="h-full min-w-0 flex-1 overflow-y-auto px-6 pt-5 pb-6"
    >
      <div class="max-w-2xl">
        <RouterView />
      </div>
    </div>
  </div>
</template>
