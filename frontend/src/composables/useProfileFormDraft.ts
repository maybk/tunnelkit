import { computed, getCurrentScope, onScopeDispose, ref, watch, type Ref } from 'vue'
import {
  createDefaultProfileDraft,
  isProfileDraftEqual,
  toProfileDraft,
  toProfileInput,
} from '@/domain/profile'
import { formatErrorMessage } from '@/presentation/error'
import { useNoticeStore } from '@/stores/useNoticeStore'
import { useProfilesStore } from '@/stores/useProfilesStore'
import type { ConfigWarning, Profile, ProfileDraft } from '@/types/profile'
import { parseGatewayEndpoint } from '@/utils/network'
import { validateProfileDraft, type ProfileFieldErrors } from '@/validate/profile'

export type ProfileEditorMode = 'visual' | 'text'

export function useProfileFormDraft(sourceProfile: Ref<Profile | null | undefined>) {
  const profilesStore = useProfilesStore()
  const noticeStore = useNoticeStore()

  const baseline = ref<ProfileDraft>(toProfileDraft(sourceProfile.value))
  const draft = ref<ProfileDraft>(toProfileDraft(sourceProfile.value))
  const editorMode = ref<ProfileEditorMode>('visual')
  const configText = ref('')
  const baselineConfigText = ref('')
  const configWarnings = ref<ConfigWarning[]>([])
  const fieldErrors = ref<ProfileFieldErrors>({})
  const formError = ref('')
  const saving = ref(false)

  let textSyncTimer: ReturnType<typeof setTimeout> | null = null
  let draftGeneration = 0
  let parseGeneration = 0

  const clearTextSyncTimer = () => {
    if (textSyncTimer) {
      clearTimeout(textSyncTimer)
      textSyncTimer = null
    }
  }

  const syncFromSource = (profile?: Profile | null) => {
    draftGeneration++
    parseGeneration++
    clearTextSyncTimer()
    const next = profile ? toProfileDraft(profile) : createDefaultProfileDraft()
    baseline.value = { ...next }
    draft.value = { ...next }
    editorMode.value = 'visual'
    configText.value = ''
    baselineConfigText.value = ''
    configWarnings.value = []
    fieldErrors.value = {}
    formError.value = ''
  }

  watch(
    () => sourceProfile.value,
    (next) => syncFromSource(next),
    { immediate: true },
  )

  const isDirty = computed(() => {
    if (editorMode.value === 'text') {
      return (
        configText.value !== baselineConfigText.value ||
        !isProfileDraftEqual(draft.value, baseline.value)
      )
    }
    return !isProfileDraftEqual(draft.value, baseline.value)
  })

  const normalizeHostOnBlur = () => {
    const parsed = parseGatewayEndpoint(draft.value.host)
    if (parsed.host) {
      draft.value.host = parsed.host
    }
    if (parsed.port) {
      draft.value.port = parsed.port
    }
  }

  const applyParsedConfigText = async (rawText: string): Promise<boolean> => {
    const currentDraftGeneration = draftGeneration
    const currentParseGeneration = ++parseGeneration
    const isCurrent = () =>
      currentDraftGeneration === draftGeneration && currentParseGeneration === parseGeneration
    try {
      const parsed = await profilesStore.parseConfigText(toProfileInput(draft.value), rawText)
      if (!isCurrent()) return false
      draft.value = {
        ...toProfileDraft({
          ...parsed.input,
          hasPassword: draft.value.hasPassword,
          updatedAt: '',
        }),
        password: parsed.input.password || draft.value.password,
      }
      configWarnings.value = parsed.warnings
      formError.value = ''
      return true
    } catch (err) {
      if (isCurrent()) formError.value = formatErrorMessage(err, '解析配置文本失败')
      return false
    }
  }

  watch(configText, (nextText) => {
    parseGeneration++
    if (editorMode.value !== 'text') return
    clearTextSyncTimer()
    textSyncTimer = setTimeout(() => {
      textSyncTimer = null
      void applyParsedConfigText(nextText)
    }, 200)
  })

  const switchEditorMode = async (targetMode: ProfileEditorMode): Promise<boolean> => {
    if (targetMode === editorMode.value) return true
    formError.value = ''
    clearTextSyncTimer()

    if (targetMode === 'text') {
      const currentGeneration = draftGeneration
      const sourceDraft = { ...draft.value }
      try {
        const text = await profilesStore.formatConfigText(
          toProfileInput(draft.value),
          draft.value.hasPassword,
        )
        if (currentGeneration !== draftGeneration || !isProfileDraftEqual(sourceDraft, draft.value))
          return false
        if (!baselineConfigText.value && isProfileDraftEqual(draft.value, baseline.value)) {
          baselineConfigText.value = text
        }
        configText.value = text
        configWarnings.value = []
        editorMode.value = 'text'
        return true
      } catch (err) {
        formError.value = formatErrorMessage(err, '生成配置文本失败')
        return false
      }
    }

    const ok = await applyParsedConfigText(configText.value)
    if (!ok) return false
    editorMode.value = 'visual'
    return true
  }

  const resetDraft = () => {
    syncFromSource(sourceProfile.value)
  }

  const submitSave = async (): Promise<Profile | null> => {
    if (saving.value) return null
    saving.value = true
    try {
      formError.value = ''
      fieldErrors.value = {}
      clearTextSyncTimer()

      if (editorMode.value === 'text') {
        const switched = await switchEditorMode('visual')
        if (!switched) return null
      }

      normalizeHostOnBlur()
      const validation = validateProfileDraft(draft.value)
      if (!validation.valid) {
        fieldErrors.value = validation.errors
        formError.value = validation.firstError
        return null
      }

      const saved = await profilesStore.saveProfile(validation.input)
      syncFromSource(saved)
      noticeStore.showSuccess('已保存节点配置', saved.name)
      return saved
    } catch (err) {
      formError.value = formatErrorMessage(err, '保存节点配置失败')
      return null
    } finally {
      saving.value = false
    }
  }

  if (getCurrentScope()) {
    onScopeDispose(() => {
      draftGeneration++
      parseGeneration++
      clearTextSyncTimer()
    })
  }

  return {
    draft,
    editorMode,
    configText,
    configWarnings,
    fieldErrors,
    formError,
    isDirty,
    saving,
    normalizeHostOnBlur,
    switchEditorMode,
    resetDraft,
    submitSave,
  }
}
