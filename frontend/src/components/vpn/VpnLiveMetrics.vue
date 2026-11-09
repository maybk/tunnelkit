<script setup lang="ts">
import { Activity, ArrowDownUp, Network } from '@lucide/vue'
import CopyButton from '@/components/common/CopyButton.vue'
import type { VpnStats, VpnTunnelInfo } from '@/types/vpn'
import { formatBytes } from '@/utils/format'

defineProps<{ info: VpnTunnelInfo | null; stats: VpnStats | null }>()
</script>
<template>
  <Transition
    enter-active-class="transition-[grid-template-rows,opacity] duration-200 ease-out"
    enter-from-class="grid-rows-[0fr]! opacity-0"
    leave-active-class="transition-[grid-template-rows,opacity] duration-150 ease-out"
    leave-to-class="grid-rows-[0fr]! opacity-0"
    @before-enter="(el) => el.removeAttribute('aria-hidden')"
    @leave-cancelled="(el) => el.removeAttribute('aria-hidden')"
    @before-leave="(el) => el.setAttribute('aria-hidden', 'true')"
  >
    <div v-if="info" class="grid grid-rows-[1fr]">
      <div class="min-h-0 overflow-hidden">
        <div class="grid grid-cols-2 gap-3">
          <div class="surface-card flex flex-col justify-between rounded-2xl p-4">
            <div class="flex items-center justify-between gap-2">
              <span
                class="inline-flex items-center gap-1.5 text-caption font-medium text-muted-foreground"
              >
                <Network class="size-3.5 text-tone-success-solid" aria-hidden="true" />
                内网 IP ({{ info.interface || 'tun' }})
              </span>
              <CopyButton v-if="info.localIp" :text="info.localIp" />
            </div>
            <div
              class="mt-2 truncate font-mono text-title font-semibold text-foreground tabular-nums"
            >
              {{ info.localIp || '—' }}
            </div>
            <div class="mt-1 truncate text-caption text-muted-foreground tabular-nums">
              对端 {{ info.peerIp || '—' }} · MTU {{ info.mtu || 1354 }}
            </div>
          </div>

          <div class="surface-card flex flex-col justify-between rounded-2xl p-4">
            <div class="flex items-center justify-between gap-2">
              <span
                class="inline-flex items-center gap-1.5 text-caption font-medium text-muted-foreground"
              >
                <ArrowDownUp class="size-3.5 text-tone-success-solid" aria-hidden="true" />
                实时流量
              </span>
              <Activity class="size-3.5 text-muted-foreground/70" aria-hidden="true" />
            </div>
            <div
              class="mt-2 truncate font-mono text-title font-semibold text-foreground tabular-nums"
            >
              ↓ {{ formatBytes(stats?.rxBytes ?? 0) }} · ↑
              {{ formatBytes(stats?.txBytes ?? 0) }}
            </div>
            <div class="mt-1 truncate text-caption text-muted-foreground tabular-nums">
              路由 {{ info.routeCount || info.routes?.length || 0 }} 条 · DNS
              {{ info.dnsServers?.join(', ') || '默认' }}
            </div>
          </div>
        </div>
      </div>
    </div>
  </Transition>
</template>
