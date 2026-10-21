import { ref } from 'vue'
import { createGlobalState } from '@vueuse/core'
import { getSystemInfo, type SystemInfo } from '@/api/system'

export const useSystem = createGlobalState(() => {
  const systemInfo = ref<SystemInfo | null>(null)
  const checking = ref(false)

  const refreshSystem = async () => {
    checking.value = true
    try {
      systemInfo.value = await getSystemInfo()
    } finally {
      checking.value = false
    }
  }

  return {
    systemInfo,
    checking,
    refreshSystem,
  }
})
