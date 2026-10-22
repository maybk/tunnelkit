import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import type { Profile, ProfileInput } from '@/api/profile'
import ProfileForm from '@/components/profile/ProfileForm.vue'

vi.mock('@/api/profile', () => {
  return {
    DEFAULT_VPN_PORT: 443,
    formatProfileText: vi.fn(async (input: ProfileInput) => {
      const lines = [`host = ${input.host}`, `port = ${input.port}`]
      if (input.username) lines.push(`username = ${input.username}`)
      if (input.password) lines.push(`password = ${input.password}`)
      if (input.trustedCert) lines.push(`trusted-cert = ${input.trustedCert}`)
      lines.push(`routes = ${input.routes || 'gateway'}`)
      lines.push(`dns = ${input.dns || 'split'}`)
      return lines.join('\n')
    }),
    parseProfileText: vi.fn(async (base: ProfileInput, raw: string) => {
      const next: ProfileInput = { ...base }
      for (const line of raw.split('\n')) {
        const trimmed = line.trim()
        if (!trimmed || trimmed.startsWith('#')) continue
        const eq = trimmed.indexOf('=')
        if (eq === -1) continue
        const k = trimmed.slice(0, eq).trim()
        const v = trimmed.slice(eq + 1).trim()
        if (k === 'host') {
          const [h, p] = v.split(':')
          next.host = h ?? ''
          if (p) next.port = Number(p)
        } else if (k === 'port') {
          next.port = Number(v)
        } else if (k === 'username') {
          next.username = v
        } else if (k === 'password') {
          next.password = v
        } else if (k === 'trusted-cert') {
          next.trustedCert = v
        } else if (k === 'set-dns' && v === '0') {
          next.dns = 'off'
        }
      }
      return { input: next, warnings: [] }
    }),
  }
})

const saved: Profile = {
  id: 'p1',
  name: '办公网',
  host: 'vpn.corp.local',
  port: 443,
  username: 'bob',
  hasPassword: true,
  trustedCert: '',
  auth: 'password',
  routes: 'gateway',
  dns: 'off',
  reconnect: true,
  updatedAt: '2026-10-19T12:00:00Z',
}

describe('ProfileForm', () => {
  it('emits a trimmed payload when creating', async () => {
    const wrapper = mount(ProfileForm, { props: { profile: null } })

    await wrapper.find('#profile-name').setValue('  公司 VPN ')
    await wrapper.find('#profile-host').setValue('vpn.company.com ')
    await wrapper.find('#profile-port').setValue('8443')
    await wrapper.find('#profile-username').setValue(' alice')
    await wrapper.find('#profile-password').setValue('s3cret')
    await wrapper.find('form').trigger('submit')

    expect(wrapper.emitted('save')?.[0]?.[0]).toEqual({
      id: '',
      name: '公司 VPN',
      host: 'vpn.company.com',
      port: 8443,
      username: 'alice',
      password: 's3cret',
      realm: '',
      trustedCert: '',
      auth: 'password',
      routes: 'gateway',
      extraRoutes: [],
      dns: 'split',
      dnsDomains: [],
      reconnect: true,
      userAgent: '',
      legacyTls: false,
      clientCert: '',
      clientKey: '',
    })
    expect(wrapper.find('#profile-password').attributes('required')).toBeDefined()
  })

  it('keeps the saved password when left empty while editing', async () => {
    const wrapper = mount(ProfileForm, { props: { profile: saved } })
    const password = wrapper.find('#profile-password')
    expect(password.attributes('required')).toBeUndefined()
    expect(password.attributes('placeholder')).toBe('留空则保持原密码')

    await wrapper.find('form').trigger('submit')
    expect(wrapper.emitted('save')?.[0]?.[0]).toMatchObject({
      id: 'p1',
      password: '',
      dns: 'off',
    })
  })

  it('shows backend errors and a saved confirmation after a successful save', async () => {
    const wrapper = mount(ProfileForm, {
      props: { profile: saved, error: '端口必须在 1 到 65535 之间' },
    })
    expect(wrapper.get('[role="alert"]').text()).toBe('端口必须在 1 到 65535 之间')

    await wrapper.setProps({ error: '', saving: true })
    await wrapper.setProps({ saving: false })
    expect(wrapper.text()).toContain('已保存')
  })

  it('tracks dirty state, supports resetting changes, and handles Cmd/Ctrl+S', async () => {
    const wrapper = mount(ProfileForm, { props: { profile: saved } })
    const saveBtn = wrapper.get('[data-testid="save-profile-button"]')

    expect(saveBtn.attributes('disabled')).toBeDefined()
    expect(wrapper.find('[data-testid="reset-profile-button"]').exists()).toBe(false)

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 's', metaKey: true }))
    expect(wrapper.emitted('save')).toBeUndefined()

    await wrapper.get('#profile-port').setValue('8443')
    expect(saveBtn.attributes('disabled')).toBeUndefined()
    const resetBtn = wrapper.get('[data-testid="reset-profile-button"]')

    await resetBtn.trigger('click')
    expect((wrapper.get('#profile-port').element as HTMLInputElement).value).toBe('443')
    expect(saveBtn.attributes('disabled')).toBeDefined()
    expect(wrapper.emitted('reset')).toHaveLength(1)

    await wrapper.get('#profile-port').setValue('9443')
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 's', metaKey: true }))
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('save')?.[0]?.[0]).toMatchObject({
      id: 'p1',
      port: 9443,
    })

    wrapper.unmount()
  })

  it('hides bottom action bar when hideActions is true', () => {
    const wrapper = mount(ProfileForm, { props: { profile: saved, hideActions: true } })
    expect(wrapper.find('[data-testid="save-profile-button"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('syncs pasted openfortivpn config text to form fields and back', async () => {
    const wrapper = mount(ProfileForm, { props: { profile: null } })

    await wrapper.find('#profile-name').setValue('研发办公 VPN')
    await wrapper.get('[data-testid="mode-text"]').trigger('click')

    const textArea = wrapper.get('#profile-config-text')
    await textArea.setValue(
      [
        '# 现有配置粘贴',
        'host = 198.51.100.10:10443',
        'username = alice',
        'password = my-pass',
        'trusted-cert = aabbcc',
        'set-dns = 0',
      ].join('\n'),
    )

    await wrapper.get('[data-testid="mode-form"]').trigger('click')
    expect((wrapper.get('#profile-host').element as HTMLInputElement).value).toBe('198.51.100.10')
    expect((wrapper.get('#profile-port').element as HTMLInputElement).value).toBe('10443')
    expect((wrapper.get('#profile-username').element as HTMLInputElement).value).toBe('alice')
    expect((wrapper.get('#profile-password').element as HTMLInputElement).value).toBe('my-pass')
    expect((wrapper.get('#profile-cert').element as HTMLInputElement).value).toBe('aabbcc')
    expect((wrapper.get('#profile-dns').element as HTMLSelectElement).value).toBe('off')

    await wrapper.get('#profile-port').setValue('8443')
    await wrapper.get('[data-testid="mode-text"]').trigger('click')
    expect((wrapper.get('#profile-config-text').element as HTMLTextAreaElement).value).toContain(
      'port = 8443',
    )
  })
})
