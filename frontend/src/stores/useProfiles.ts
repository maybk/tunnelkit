import { computed, ref } from 'vue'
import { createGlobalState } from '@vueuse/core'
import {
  deleteProfile,
  listProfiles,
  saveProfile,
  trustProfileCert,
  type Profile,
  type ProfileInput,
} from '@/api/profile'
import { errorMessage } from '@/lib/format'
import { useNotice } from '@/stores/useNotice'

export type DetailTab = 'status' | 'config' | 'logs'

export const useProfiles = createGlobalState(() => {
  const { showError } = useNotice()

  const profiles = ref<Profile[]>([])
  const selectedId = ref('')
  const creating = ref(false)
  const activeTab = ref<DetailTab>('status')
  const saving = ref(false)
  const deleting = ref(false)
  /** 仅用于配置表单内联展示的保存 / 删除错误。 */
  const formError = ref('')

  const selectedProfile = computed<Profile | null>(() =>
    creating.value ? null : (profiles.value.find((p) => p.id === selectedId.value) ?? null),
  )

  const loadProfiles = async (preferredId?: string) => {
    const list = await listProfiles()
    profiles.value = list
    if (preferredId && list.some((p) => p.id === preferredId)) {
      selectedId.value = preferredId
      creating.value = false
    } else if (!list.some((p) => p.id === selectedId.value)) {
      selectedId.value = list[0]?.id ?? ''
    }
    if (list.length === 0) creating.value = true
  }

  const selectProfile = (id: string) => {
    if (creating.value || selectedId.value !== id) activeTab.value = 'status'
    creating.value = false
    selectedId.value = id
    formError.value = ''
  }

  const startCreate = () => {
    creating.value = true
    formError.value = ''
  }

  const cancelCreate = () => {
    if (profiles.value.length > 0) creating.value = false
    formError.value = ''
  }

  const save = async (input: ProfileInput): Promise<boolean> => {
    if (saving.value) return false
    saving.value = true
    formError.value = ''
    try {
      const saved = await saveProfile(input)
      await loadProfiles(saved.id)
      return true
    } catch (err) {
      formError.value = errorMessage(err)
      return false
    } finally {
      saving.value = false
    }
  }

  const remove = async (id: string): Promise<boolean> => {
    if (deleting.value) return false
    deleting.value = true
    formError.value = ''
    try {
      await deleteProfile(id)
      await loadProfiles()
      activeTab.value = 'status'
      return true
    } catch (err) {
      formError.value = errorMessage(err)
      return false
    } finally {
      deleting.value = false
    }
  }

  const trustCert = async (id: string, sha256: string): Promise<boolean> => {
    try {
      await trustProfileCert(id, sha256)
      await loadProfiles(id)
      return true
    } catch (err) {
      showError(err)
      return false
    }
  }

  return {
    profiles,
    selectedId,
    creating,
    activeTab,
    saving,
    deleting,
    formError,
    selectedProfile,
    loadProfiles,
    selectProfile,
    startCreate,
    cancelCreate,
    save,
    remove,
    trustCert,
  }
})
