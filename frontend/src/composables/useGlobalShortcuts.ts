import { onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { backFromSettings, ROUTE_NAMES } from '@/router'
import { useProfilesStore } from '@/stores/useProfilesStore'
import { hasOpenOverlay, isEditableElement } from '@/utils/dom'

export function useGlobalShortcuts() {
  const router = useRouter()
  const route = useRoute()
  const profilesStore = useProfilesStore()

  const handleKeydown = (event: KeyboardEvent) => {
    if (event.defaultPrevented || hasOpenOverlay(document)) return

    if (
      event.key === 'Escape' &&
      !event.metaKey &&
      !event.ctrlKey &&
      !event.altKey &&
      !event.shiftKey
    ) {
      if (route.meta.settings && !isEditableElement(event.target)) {
        event.preventDefault()
        void backFromSettings()
        return
      }
      if (route.name === ROUTE_NAMES.profileCreate) {
        event.preventDefault()
        void router.push({ name: ROUTE_NAMES.workspace })
        return
      }
    }

    if (isEditableElement(event.target)) return

    const isMod = event.metaKey || event.ctrlKey
    if (!isMod || event.altKey || event.shiftKey) return

    const key = event.key.toLowerCase()
    if (key === ',') {
      event.preventDefault()
      if (!route.meta.settings) {
        void router.push({ name: ROUTE_NAMES.settings })
      }
      return
    }

    if (key === 'n') {
      event.preventDefault()
      void router.push({ name: ROUTE_NAMES.profileCreate })
      return
    }

    if (key === '1') {
      event.preventDefault()
      profilesStore.setActiveTab('status')
      if (route.name !== ROUTE_NAMES.workspace) {
        void router.push({ name: ROUTE_NAMES.workspace })
      }
    } else if (key === '2') {
      event.preventDefault()
      profilesStore.setActiveTab('config')
      if (route.name !== ROUTE_NAMES.workspace) {
        void router.push({ name: ROUTE_NAMES.workspace })
      }
    } else if (key === '3') {
      event.preventDefault()
      profilesStore.setActiveTab('logs')
      if (route.name !== ROUTE_NAMES.workspace) {
        void router.push({ name: ROUTE_NAMES.workspace })
      }
    }
  }

  onMounted(() => {
    window.addEventListener('keydown', handleKeydown)
  })

  onUnmounted(() => {
    window.removeEventListener('keydown', handleKeydown)
  })
}
