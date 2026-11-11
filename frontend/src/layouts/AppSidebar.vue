<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  ArrowLeftRight,
  EllipsisVertical,
  Loader2,
  Play,
  Plus,
  Trash2,
  Unplug,
  X,
} from '@lucide/vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import AppButton from '@/components/base/button/AppButton.vue'
import AppDropdownMenuContent from '@/components/base/AppDropdownMenuContent.vue'
import AppDropdownMenuItem from '@/components/base/AppDropdownMenuItem.vue'
import {
  DropdownMenu,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { getViewingStatusMeta } from '@/presentation/vpn'
import type { Profile } from '@/types/profile'
import type { VpnPendingAction, VpnState } from '@/types/vpn'
import { formatEndpoint } from '@/utils/network'

const props = withDefaults(
  defineProps<{
    profiles: readonly Profile[]
    selectedId: string
    vpnState: VpnState
    isCreating?: boolean
    busy?: boolean
    pendingAction?: VpnPendingAction | null
    canceling?: boolean
    deletingProfileId?: string
  }>(),
  { isCreating: false, busy: false, canceling: false, deletingProfileId: '' },
)

const emit = defineEmits<{
  selectProfile: [id: string]
  createProfile: []
  connect: [id: string]
  cancel: []
  disconnect: []
  delete: [id: string]
}>()

const deleteTarget = ref<Profile | null>(null)
const openMenuId = ref<string | null>(null)
const activeProfileId = computed(() =>
  props.vpnState.status === 'disconnected' ? '' : props.vpnState.profileId,
)

const itemStatus = (id: string) => getViewingStatusMeta(props.vpnState, id)

const handleMenuOpenChange = (id: string, open: boolean) => {
  if (open) {
    openMenuId.value = id
  } else if (openMenuId.value === id) {
    openMenuId.value = null
  }
}

const confirmDelete = (id: string) => {
  emit('delete', id)
  deleteTarget.value = null
}
</script>

<template>
  <aside
    class="flex w-64 shrink-0 flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground select-none"
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

    <div class="flex-1 space-y-2 overflow-y-auto px-3 py-1">
      <div
        v-if="profiles.length === 0"
        class="px-3 py-6 text-center text-caption text-muted-foreground"
      >
        暂无连接
      </div>

      <DropdownMenu
        v-for="profile in profiles"
        :key="profile.id"
        :open="openMenuId === profile.id"
        @update:open="(open) => handleMenuOpenChange(profile.id, open)"
      >
        <div
          :data-menu-open="openMenuId === profile.id ? 'true' : undefined"
          class="group/item relative overflow-hidden rounded-xl border transition-[background-color,border-color,box-shadow] duration-150"
          :class="
            selectedId === profile.id && !isCreating
              ? 'border-primary/40 bg-card text-foreground shadow-[var(--shadow-card)] ring-1 ring-primary/15'
              : 'border-sidebar-border/85 bg-card/65 text-sidebar-foreground/90 hover:border-border hover:bg-card hover:text-foreground data-[menu-open=true]:border-border data-[menu-open=true]:bg-card data-[menu-open=true]:text-foreground'
          "
        >
          <button
            type="button"
            class="flex w-full items-center justify-between gap-2.5 rounded-[11px] px-3.5 py-2.5 text-left outline-none focus-visible:ring-2 focus-visible:ring-ring/40"
            :aria-current="selectedId === profile.id ? 'true' : undefined"
            :data-testid="`sidebar-profile-${profile.id}`"
            @click="emit('selectProfile', profile.id)"
            @contextmenu.prevent="openMenuId = profile.id"
          >
            <div class="min-w-0 flex-1">
              <div
                class="truncate text-body font-medium leading-snug"
                :class="
                  selectedId === profile.id && !isCreating
                    ? 'text-primary dark:text-foreground'
                    : 'text-foreground'
                "
              >
                {{ profile.name }}
              </div>
              <div
                class="mt-0.5 truncate font-mono text-caption text-muted-foreground tabular-nums"
              >
                {{ formatEndpoint(profile.host, profile.port) }}
              </div>
            </div>
            <div class="flex shrink-0 items-center gap-1.5">
              <span
                v-if="vpnState.status === 'connected' && activeProfileId === profile.id"
                class="text-micro font-medium text-tone-success-fg"
              >
                {{ itemStatus(profile.id).label }}
              </span>
              <StatusDot
                :tone="itemStatus(profile.id).tone"
                :pulse="itemStatus(profile.id).pulse"
              />
            </div>
          </button>

          <div
            :data-menu-open="openMenuId === profile.id ? 'true' : undefined"
            class="pointer-events-none absolute inset-y-0 right-0 flex items-center rounded-r-[11px] bg-linear-to-r from-transparent via-card/90 via-40% to-card pr-2 pl-8 opacity-0 transition-opacity group-hover/item:pointer-events-auto group-hover/item:opacity-100 group-focus-within/item:pointer-events-auto group-focus-within/item:opacity-100 data-[menu-open=true]:pointer-events-auto data-[menu-open=true]:opacity-100"
          >
            <DropdownMenuTrigger as-child>
              <button
                type="button"
                data-testid="sidebar-profile-menu"
                :aria-label="`${profile.name} 更多操作`"
                class="flex size-7 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-foreground/[0.08] hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/40 focus-visible:outline-none data-[state=open]:bg-foreground/[0.08] data-[state=open]:text-foreground"
                @click.stop
              >
                <EllipsisVertical class="size-4" aria-hidden="true" />
              </button>
            </DropdownMenuTrigger>
          </div>
        </div>

        <AppDropdownMenuContent align="end" class="w-36 min-w-36">
          <AppDropdownMenuItem
            v-if="activeProfileId === profile.id && vpnState.status === 'connected'"
            :disabled="busy"
            @select="emit('disconnect')"
          >
            <Loader2
              v-if="pendingAction?.kind === 'disconnect' && pendingAction.profileId === profile.id"
              class="size-4 animate-spin text-muted-foreground"
            />
            <Unplug v-else class="size-4 text-muted-foreground" />
            {{
              pendingAction?.kind === 'disconnect' && pendingAction.profileId === profile.id
                ? '正在断开…'
                : '断开连接'
            }}
          </AppDropdownMenuItem>
          <AppDropdownMenuItem
            v-else-if="activeProfileId === profile.id && vpnState.status === 'connecting'"
            :disabled="canceling"
            @select="emit('cancel')"
          >
            <Loader2 v-if="canceling" class="size-4 animate-spin text-muted-foreground" />
            <X v-else class="size-4 text-muted-foreground" />
            {{ canceling ? '正在取消…' : '取消连接' }}
          </AppDropdownMenuItem>
          <AppDropdownMenuItem
            v-else-if="activeProfileId === profile.id && vpnState.status === 'disconnecting'"
            disabled
          >
            <Loader2 class="size-4 animate-spin text-muted-foreground" />正在断开…
          </AppDropdownMenuItem>
          <AppDropdownMenuItem
            v-else
            :disabled="busy || vpnState.status === 'disconnecting'"
            @select="emit('connect', profile.id)"
          >
            <Loader2
              v-if="pendingAction?.kind === 'connect' && pendingAction.profileId === profile.id"
              class="size-4 animate-spin text-muted-foreground"
            />
            <ArrowLeftRight v-else-if="activeProfileId" class="size-4 text-muted-foreground" />
            <Play v-else class="size-4 text-muted-foreground" />
            {{ activeProfileId ? '切换连接' : '连接' }}
          </AppDropdownMenuItem>
          <DropdownMenuSeparator />
          <AppDropdownMenuItem
            variant="destructive"
            :disabled="activeProfileId === profile.id || Boolean(deletingProfileId)"
            @select="deleteTarget = profile"
          >
            <Trash2 class="size-4" />
            删除
          </AppDropdownMenuItem>
        </AppDropdownMenuContent>

        <ConfirmDialog
          :open="deleteTarget?.id === profile.id"
          title="删除连接"
          :description="`删除后将永久移除连接「${profile.name}」及其已保存的密码与连接日志，此操作无法撤销。`"
          :confirm-test-id="`sidebar-delete-${profile.id}`"
          @update:open="
            (open) => {
              if (!open) deleteTarget = null
            }
          "
          @confirm="confirmDelete(profile.id)"
        />
      </DropdownMenu>
    </div>

    <div class="border-t border-sidebar-border p-3">
      <AppButton
        :variant="isCreating ? 'secondary' : 'outline'"
        size="default"
        class="w-full justify-center font-medium"
        data-testid="sidebar-new-profile-btn"
        @click="emit('createProfile')"
      >
        <Plus class="size-4" />
        新建连接
      </AppButton>
    </div>
  </aside>
</template>
