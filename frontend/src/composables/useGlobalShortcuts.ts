import { onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ROUTE_NAMES } from '@/router'
import { useProfilesStore } from '@/stores/useProfilesStore'
import { hasOpenOverlay, isEditableElement } from '@/utils/dom'

export function useGlobalShortcuts() {
  const router = useRouter()
  const route = useRoute()
  const profilesStore = useProfilesStore()

  const handleKeydown = (event: KeyboardEvent) => {
    if (hasOpenOverlay(document)) return

    if (event.key === 'Escape' && route.name === ROUTE_NAMES.profileCreate) {
      event.preventDefault()
      void router.push({ name: ROUTE_NAMES.workspace })
      return
    }

    if (isEditableElement(event.target)) return

    const isMod = event.metaKey || event.ctrlKey
    if (!isMod || event.altKey || event.shiftKey) return

    const key = event.key.toLowerCase()
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
