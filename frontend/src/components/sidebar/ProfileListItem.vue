<script setup lang="ts">
import { computed, ref } from 'vue'
import { ArrowLeftRight, EllipsisVertical, Loader2, Play, Trash2, Unplug, X } from 'lucide-vue-next'
import type { Profile } from '@/api/profile'
import type { VpnState } from '@/api/vpn'
import { profileStatus } from '@/lib/status'
import StatusDot from '@/components/common/StatusDot.vue'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

const props = withDefaults(
  defineProps<{
    profile: Profile
    selected: boolean
    vpnState: VpnState
    pending?: boolean
    helperReady?: boolean
    deleting?: boolean
    saving?: boolean
  }>(),
  {
    pending: false,
    helperReady: true,
    deleting: false,
    saving: false,
  },
)

const emit = defineEmits<{
  select: [id: string]
  connect: [id: string]
  cancel: []
  disconnect: []
  delete: [id: string]
}>()

const view = computed(() => profileStatus(props.profile.id, props.vpnState))
const menuOpen = ref(false)
const confirmDeleteOpen = ref(false)

const isBusyOnThisProfile = computed(
  () =>
    props.pending && (!props.vpnState.profileId || props.vpnState.profileId === props.profile.id),
)

const otherDisconnecting = computed(
  () => view.value.otherActive && props.vpnState.status === 'disconnecting',
)

const connectBlockedReason = computed(() => {
  if (!props.helperReady) return '特权辅助组件 tunnelkit-helper 尚未就绪'
  if (otherDisconnecting.value) return '正在断开当前活动的连接，请稍候'
  return ''
})

const inUse = computed(() => view.value.status !== 'disconnected')
const deleteDisabled = computed(() => inUse.value || props.deleting || props.saving)

const openContextMenu = () => {
  menuOpen.value = true
}

const requestDelete = () => {
  if (deleteDisabled.value) return
  menuOpen.value = false
  confirmDeleteOpen.value = true
}

const confirmDelete = () => {
  emit('delete', props.profile.id)
}
</script>

<template>
  <div
    :data-menu-open="menuOpen ? 'true' : undefined"
    class="group/item relative overflow-hidden rounded-xl border transition-[background-color,border-color,box-shadow,color] duration-150 ease-out"
    :class="
      selected
        ? 'border-primary/40 bg-card text-foreground shadow-[var(--shadow-card)] ring-1 ring-primary/15'
        : 'border-sidebar-border/85 bg-card/65 text-sidebar-foreground/90 shadow-[0_1px_1px_rgba(15,23,42,0.02)] hover:border-border hover:bg-card hover:text-foreground data-[menu-open=true]:border-border data-[menu-open=true]:bg-card data-[menu-open=true]:text-foreground dark:bg-card/45 dark:hover:bg-card dark:data-[menu-open=true]:bg-card'
    "
    @contextmenu.prevent="openContextMenu"
  >
    <button
      type="button"
      data-testid="profile-select-button"
      class="flex w-full items-center justify-between gap-2.5 rounded-[11px] px-3.5 py-2.5 text-left outline-none focus-visible:ring-2 focus-visible:ring-ring/40"
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

    <div
      :data-menu-open="menuOpen ? 'true' : undefined"
      class="pointer-events-none absolute inset-y-0 right-0 flex items-center justify-end rounded-r-[11px] bg-linear-to-r from-transparent via-card/90 via-40% to-card pr-2 pl-8 opacity-0 transition-opacity duration-150 ease-out group-hover/item:pointer-events-auto group-hover/item:opacity-100 group-focus-within/item:pointer-events-auto group-focus-within/item:opacity-100 data-[menu-open=true]:pointer-events-auto data-[menu-open=true]:opacity-100"
    >
      <DropdownMenu v-model:open="menuOpen">
        <DropdownMenuTrigger as-child>
          <button
            type="button"
            data-testid="profile-menu-trigger"
            :aria-label="`${profile.name} 更多操作`"
            class="flex size-7 items-center justify-center rounded-lg text-muted-foreground transition-colors duration-150 hover:bg-foreground/[0.08] hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/40 focus-visible:outline-none data-[state=open]:bg-foreground/[0.08] data-[state=open]:text-foreground"
            @click.stop
          >
            <EllipsisVertical class="size-4" aria-hidden="true" />
          </button>
        </DropdownMenuTrigger>

        <DropdownMenuContent align="end" class="w-40">
          <DropdownMenuItem
            v-if="view.status === 'connected'"
            data-testid="menu-disconnect"
            :disabled="pending"
            @select="emit('disconnect')"
          >
            <Unplug class="size-4 text-muted-foreground" aria-hidden="true" />
            <span>断开连接</span>
          </DropdownMenuItem>

          <DropdownMenuItem
            v-else-if="view.status === 'connecting'"
            data-testid="menu-cancel"
            @select="emit('cancel')"
          >
            <X class="size-4 text-muted-foreground" aria-hidden="true" />
            <span>取消连接</span>
          </DropdownMenuItem>

          <DropdownMenuItem
            v-else-if="view.status === 'disconnecting'"
            data-testid="menu-disconnecting"
            disabled
          >
            <Loader2 class="size-4 animate-spin text-muted-foreground" aria-hidden="true" />
            <span>正在断开…</span>
          </DropdownMenuItem>

          <DropdownMenuItem
            v-else
            data-testid="menu-connect"
            :disabled="isBusyOnThisProfile || Boolean(connectBlockedReason)"
            :title="connectBlockedReason || undefined"
            @select="emit('connect', profile.id)"
          >
            <Loader2
              v-if="isBusyOnThisProfile"
              class="size-4 animate-spin text-muted-foreground"
              aria-hidden="true"
            />
            <ArrowLeftRight
              v-else-if="view.otherActive"
              class="size-4 text-muted-foreground"
              aria-hidden="true"
            />
            <Play v-else class="size-4 text-muted-foreground" aria-hidden="true" />
            <span>{{ view.otherActive ? '切换连接' : '连接' }}</span>
          </DropdownMenuItem>

          <DropdownMenuSeparator />

          <span :title="inUse ? '请先断开该连接' : undefined" class="block">
            <DropdownMenuItem
              variant="destructive"
              data-testid="menu-delete"
              :disabled="deleteDisabled"
              @select="requestDelete"
            >
              <Trash2 class="size-4" aria-hidden="true" />
              <span>删除连接</span>
            </DropdownMenuItem>
          </span>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>

    <AlertDialog v-model:open="confirmDeleteOpen">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>删除「{{ profile.name }}」？</AlertDialogTitle>
          <AlertDialogDescription>
            将同时删除已保存的密码和连接日志，此操作无法撤销。
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>取消</AlertDialogCancel>
          <AlertDialogAction data-testid="confirm-delete" @click="confirmDelete">
            删除
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>
