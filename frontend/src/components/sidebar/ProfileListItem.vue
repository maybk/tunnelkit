<script setup lang="ts">
import { computed } from 'vue'
import type { Profile } from '@/api/profile'
import type { VpnState } from '@/api/vpn'
import { profileStatus } from '@/lib/status'
import StatusDot from '@/components/common/StatusDot.vue'

const props = defineProps<{
  profile: Profile
  selected: boolean
  vpnState: VpnState
}>()

const emit = defineEmits<{
  select: [id: string]
}>()

const view = computed(() => profileStatus(props.profile.id, props.vpnState))
</script>

<template>
  <button
    type="button"
    class="group/item relative flex w-full items-center justify-between gap-2.5 rounded-xl border px-3.5 py-2.5 text-left transition-[background-color,border-color,box-shadow,color] duration-150 ease-out"
    :class="
      selected
        ? 'border-primary/40 bg-card text-foreground shadow-[var(--shadow-card)] ring-1 ring-primary/15'
        : 'border-sidebar-border/85 bg-card/65 text-sidebar-foreground/90 shadow-[0_1px_1px_rgba(15,23,42,0.02)] hover:border-border hover:bg-card hover:text-foreground dark:bg-card/45 dark:hover:bg-card/80'
    "
    :aria-current="selected ? 'true' : undefined"
    @click="emit('select', profile.id)"
  >
    <div class="min-w-0 flex-1">
      <div
        class="truncate text-body font-medium leading-snug"
        :class="selected ? 'text-primary dark:text-foreground' : 'text-foreground'"
      >
        {{ profile.name }}
      </div>
      <div class="mt-0.5 truncate font-mono text-caption text-muted-foreground tabular-nums">
        {{ profile.host }}:{{ profile.port }}
      </div>
    </div>

    <div class="flex shrink-0 items-center gap-1.5">
      <span v-if="view.status === 'connected'" class="text-[11px] font-medium text-status-ok">
        已连接
      </span>
      <StatusDot :tone="view.tone" />
    </div>
  </button>
</template>
