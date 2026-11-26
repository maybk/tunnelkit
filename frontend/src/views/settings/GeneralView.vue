<script setup lang="ts">
import AppSwitch from '@/components/base/AppSwitch.vue'
import SettingCard from '@/components/settings/SettingCard.vue'
import SettingRow from '@/components/settings/SettingRow.vue'
import { useSettingsStore } from '@/stores/useSettingsStore'

const settingsStore = useSettingsStore()
</script>

<template>
  <div class="space-y-5">
    <SettingCard title="更新">
      <SettingRow label="自动检查更新" description="启动应用后自动在后台检查新版本">
        <AppSwitch
          data-testid="auto-check-update-switch"
          :model-value="settingsStore.settings.autoCheckUpdate"
          aria-label="自动检查更新"
          @update:model-value="settingsStore.saveSettings({ autoCheckUpdate: $event })"
        />
      </SettingRow>
    </SettingCard>
  </div>
</template>
