<script setup lang="ts">
import { ArrowLeft } from '@lucide/vue'
import { useRoute, useRouter } from 'vue-router'
import NavItem from '@/components/common/NavItem.vue'
import { backFromSettings, SETTINGS_NAV_ITEMS, settingsRouteName } from '@/router'

defineProps<{
  hasUpdate?: boolean
}>()

const route = useRoute()
const router = useRouter()
</script>

<template>
  <aside
    data-testid="settings-sidebar"
    class="flex h-full w-48 shrink-0 flex-col overflow-hidden border-r border-sidebar-border bg-sidebar text-sidebar-foreground select-none"
  >
    <div class="px-2.5 pt-2 pb-2">
      <NavItem
        :icon="ArrowLeft"
        label="返回"
        tooltip="返回主界面 (Esc)"
        test-id="settings-back-btn"
        class="text-muted-foreground hover:text-foreground"
        @click="backFromSettings()"
      />
    </div>

    <div class="mx-3 border-b border-sidebar-border/60" />

    <nav class="flex-1 space-y-0.5 overflow-y-auto px-2.5 py-2.5" aria-label="设置导航">
      <div class="px-3 pb-1 text-micro font-medium text-muted-foreground">设置</div>

      <NavItem
        v-for="item in SETTINGS_NAV_ITEMS"
        :key="item.key"
        :icon="item.icon"
        :label="item.label"
        :active="route.name === settingsRouteName(item.key)"
        :dot="item.key === 'about' && Boolean(hasUpdate)"
        dot-test-id="settings-nav-about-dot"
        :test-id="`settings-nav-${item.key}`"
        @click="router.push({ name: settingsRouteName(item.key) })"
      />
    </nav>
  </aside>
</template>
