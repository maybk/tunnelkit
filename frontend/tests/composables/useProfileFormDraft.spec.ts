import { nextTick, ref } from 'vue'
import { describe, expect, it, vi } from 'vite-plus/test'
import { profileApi } from '@/api/profile'
import { useProfileFormDraft } from '@/composables/useProfileFormDraft'
import { createTestProfile, createTestProfileInput } from '../fixtures/profiles'
import type { ParsedProfileText } from '@/types/profile'

describe('useProfileFormDraft', () => {
  it('does not apply an old text parse after the draft has been reset', async () => {
    const form = useProfileFormDraft(ref(createTestProfile()))
    vi.spyOn(profileApi, 'formatText').mockResolvedValue('host = vpn.example.com')
    await form.switchEditorMode('text')
    form.configText.value = 'host = stale.example.com'
    await nextTick()
    let resolveParse: ((result: ParsedProfileText) => void) | undefined
    vi.spyOn(profileApi, 'parseText').mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveParse = resolve
        }),
    )
    const parsing = form.switchEditorMode('visual')
    form.resetDraft()
    resolveParse?.({ input: createTestProfileInput({ host: 'stale.example.com' }), warnings: [] })
    await parsing
    expect(form.draft.value.host).toBe('vpn.example.com')
    expect(form.isDirty.value).toBe(false)
  })
  it('keeps an invalid port dirty and blocks toolbar submission', async () => {
    const form = useProfileFormDraft(ref(createTestProfile()))
    const save = vi.spyOn(profileApi, 'save')
    form.draft.value.port = 0
    expect(form.isDirty.value).toBe(true)
    expect(await form.submitSave()).toBeNull()
    expect(form.fieldErrors.value.port).toBeTruthy()
    expect(save).not.toHaveBeenCalled()
  })

  it('tracks dirty state, switches between visual and text modes, and saves profile', async () => {
    const source = ref(createTestProfile())
    const form = useProfileFormDraft(source)

    expect(form.isDirty.value).toBe(false)
    form.draft.value.name = '更新后的节点'
    expect(form.isDirty.value).toBe(true)

    vi.spyOn(profileApi, 'formatText').mockResolvedValue('host = vpn.example.com\nport = 443\n')
    const switchedToText = await form.switchEditorMode('text')
    expect(switchedToText).toBe(true)
    expect(form.editorMode.value).toBe('text')

    vi.spyOn(profileApi, 'parseText').mockResolvedValue({
      input: createTestProfileInput({ name: '从文本解析的节点' }),
      warnings: [],
    })
    vi.spyOn(profileApi, 'save').mockResolvedValue(createTestProfile({ name: '从文本解析的节点' }))
    vi.spyOn(profileApi, 'list').mockResolvedValue([
      createTestProfile({ name: '从文本解析的节点' }),
    ])

    const saved = await form.submitSave()
    expect(saved?.name).toBe('从文本解析的节点')
    expect(form.editorMode.value).toBe('visual')
  })
})
