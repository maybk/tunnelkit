import { mount, flushPromises } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vite-plus/test'
import { profileApi } from '@/api/profile'
import ProfileFormPanel from '@/components/profile/ProfileFormPanel.vue'
import { createTestProfile } from '../fixtures/profiles'

describe('profile editor mode transitions', () => {
  it('keeps the shared name field and editable text intact when parsing fails', async () => {
    vi.spyOn(profileApi, 'formatText').mockResolvedValue('host = vpn.example.com')
    vi.spyOn(profileApi, 'parseText').mockRejectedValue(new Error('配置格式错误'))
    const wrapper = mount(ProfileFormPanel, { props: { profile: createTestProfile() } })
    const name = wrapper.get('[data-testid="profile-name-input"]')
    await name.setValue('编辑中的名称')
    await wrapper.get('[role="radio"][aria-label="配置文本"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="profile-name-input"]').element).toBe(name.element)
    const text = wrapper.get('[data-testid="profile-config-textarea"]')
    await text.setValue('invalid config')
    await wrapper.get('[role="radio"][aria-label="表单填写"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('配置格式错误')
    expect(
      (wrapper.get('[data-testid="profile-name-input"]').element as HTMLInputElement).value,
    ).toBe('编辑中的名称')
    expect(
      (wrapper.get('[data-testid="profile-config-textarea"]').element as HTMLTextAreaElement).value,
    ).toBe('invalid config')
    wrapper.unmount()
  })
})
