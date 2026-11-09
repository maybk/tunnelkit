import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { profileApi } from '@/api/profile'
import { SELECTED_PROFILE_STORAGE_KEY } from '@/constants/storage'
import { useNoticeStore } from '@/stores/useNoticeStore'
import type { ParsedProfileText, Profile, ProfileInput, WorkspaceTab } from '@/types/profile'

function readStoredSelectedId(): string {
  try {
    return globalThis.localStorage?.getItem(SELECTED_PROFILE_STORAGE_KEY) ?? ''
  } catch {
    return ''
  }
}

function writeStoredSelectedId(id: string) {
  try {
    if (id) {
      globalThis.localStorage?.setItem(SELECTED_PROFILE_STORAGE_KEY, id)
    } else {
      globalThis.localStorage?.removeItem(SELECTED_PROFILE_STORAGE_KEY)
    }
  } catch {
    // 忽略受限环境下的 localStorage 异常
  }
}

export const useProfilesStore = defineStore('profiles', () => {
  const noticeStore = useNoticeStore()
  const profiles = ref<Profile[]>([])
  const selectedId = ref<string>(readStoredSelectedId())
  const activeTab = ref<WorkspaceTab>('status')
  const loading = ref(false)
  const deletingProfileId = ref('')

  const selectedProfile = computed<Profile | null>(
    () => profiles.value.find((p) => p.id === selectedId.value) ?? null,
  )

  const selectProfile = (id: string) => {
    selectedId.value = id
    writeStoredSelectedId(id)
  }

  const setActiveTab = (tab: WorkspaceTab) => {
    activeTab.value = tab
  }

  const reconcileSelection = (preferredId?: string) => {
    if (preferredId && profiles.value.some((p) => p.id === preferredId)) {
      selectProfile(preferredId)
      return
    }
    if (selectedId.value && profiles.value.some((p) => p.id === selectedId.value)) {
      return
    }
    const first = profiles.value[0]?.id ?? ''
    selectProfile(first)
  }

  const loadProfiles = async (activeVpnProfileId?: string): Promise<Profile[]> => {
    loading.value = true
    try {
      const list = await profileApi.list()
      profiles.value = list
      reconcileSelection(activeVpnProfileId)
      return list
    } finally {
      loading.value = false
    }
  }

  const saveProfile = async (input: ProfileInput): Promise<Profile> => {
    const saved = await profileApi.save(input)
    await loadProfiles(saved.id)
    selectProfile(saved.id)
    return saved
  }

  const deleteProfile = async (id: string): Promise<void> => {
    if (!id || deletingProfileId.value) return
    deletingProfileId.value = id
    const deletedProfile = profiles.value.find((profile) => profile.id === id)
    try {
      await profileApi.remove(id)
      profiles.value = profiles.value.filter((profile) => profile.id !== id)
      reconcileSelection()
      await loadProfiles()
      noticeStore.showSuccess('已删除节点', deletedProfile?.name)
    } catch (err) {
      noticeStore.showError(err, '删除节点失败')
    } finally {
      deletingProfileId.value = ''
    }
  }

  const trustCert = async (id: string, sha256: string): Promise<Profile> => {
    const updated = await profileApi.trustCert(id, sha256)
    await loadProfiles(updated.id)
    return updated
  }

  const formatConfigText = async (
    input: ProfileInput,
    hasSavedPassword: boolean,
  ): Promise<string> => {
    return profileApi.formatText(input, hasSavedPassword)
  }

  const parseConfigText = async (base: ProfileInput, text: string): Promise<ParsedProfileText> => {
    return profileApi.parseText(base, text)
  }

  return {
    profiles,
    selectedId,
    selectedProfile,
    activeTab,
    loading,
    deletingProfileId,
    selectProfile,
    setActiveTab,
    loadProfiles,
    saveProfile,
    deleteProfile,
    trustCert,
    formatConfigText,
    parseConfigText,
  }
})
