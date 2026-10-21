<script setup lang="ts">
import { Plus } from 'lucide-vue-next'
import type { Profile } from '@/api/profile'
import type { VpnState } from '@/api/vpn'
import { Button } from '@/components/ui/button'
import ProfileListItem from './ProfileListItem.vue'

defineProps<{
  profiles: Profile[]
  selectedId: string
  creating: boolean
  vpnState: VpnState
}>()

const emit = defineEmits<{
  select: [id: string]
  create: []
}>()
</script>

<template>
  <!-- DesktopChrome 已统一接管窗口顶部安全区与拖拽契约，侧栏直接从 ApplicationBody 顶部开始 -->
  <aside
    class="flex h-full w-64 shrink-0 flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground"
  >
    <div class="flex items-center justify-between px-3.5 pt-3.5 pb-2">
      <span class="text-caption font-medium text-muted-foreground">连接列表</span>
      <span
        v-if="profiles.length > 0"
        class="rounded-full bg-foreground/[0.06] px-2 py-0.5 font-mono text-[11px] font-medium text-muted-foreground tabular-nums"
      >
        {{ profiles.length }}
      </span>
    </div>

    <nav class="flex-1 space-y-2 overflow-y-auto px-3 py-1" aria-label="VPN 连接列表">
      <ProfileListItem
        v-for="item in profiles"
        :key="item.id"
        :profile="item"
        :selected="!creating && item.id === selectedId"
        :vpn-state="vpnState"
        @select="emit('select', $event)"
      />

      <div
        v-if="profiles.length === 0"
        class="my-1 rounded-xl border border-dashed border-sidebar-border bg-card/40 px-3 py-5 text-center"
      >
        <div class="text-caption font-medium text-foreground/75">暂无连接</div>
        <div class="mt-0.5 text-[11px] text-muted-foreground">在右侧填写信息后保存</div>
      </div>
    </nav>

    <div class="border-t border-sidebar-border p-3">
      <Button
        :variant="creating ? 'secondary' : 'outline'"
        size="default"
        class="w-full justify-center font-medium"
        :disabled="creating && profiles.length === 0"
        @click="emit('create')"
      >
        <Plus class="size-4" aria-hidden="true" />
        新建连接
      </Button>
    </div>
  </aside>
</template>
