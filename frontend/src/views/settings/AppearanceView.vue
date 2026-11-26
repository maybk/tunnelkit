<script setup lang="ts">
import { computed } from 'vue'
import { Monitor, Moon, Sun } from '@lucide/vue'
import SegmentedControl from '@/components/common/SegmentedControl.vue'
import SettingCard from '@/components/settings/SettingCard.vue'
import SettingRow from '@/components/settings/SettingRow.vue'
import { THEME_MODE_META } from '@/constants/appearance'
import { useSettingsStore } from '@/stores/useSettingsStore'

const settingsStore = useSettingsStore()

const themeIcons = { system: Monitor, light: Sun, dark: Moon }
const themeOptions = computed(() =>
  THEME_MODE_META.map((item) => ({
    value: item.id,
    label: item.label,
    testId: `settings-theme-${item.id}`,
  })),
)
</script>

<template>
  <div class="space-y-5">
    <SettingCard>
      <SettingRow label="主题模式" description="选择跟随系统、浅色或深色外观">
        <SegmentedControl
          :model-value="settingsStore.settings.theme"
          :options="themeOptions"
          aria-label="主题模式"
          @update:model-value="settingsStore.saveSettings({ theme: $event })"
        >
          <template #option="{ option }">
            <component :is="themeIcons[option.value]" class="size-3.5" aria-hidden="true" />
            <span>{{ option.label }}</span>
          </template>
        </SegmentedControl>
      </SettingRow>
    </SettingCard>
  </div>
</template>
