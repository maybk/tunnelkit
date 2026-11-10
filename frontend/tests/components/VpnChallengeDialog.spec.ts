import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vite-plus/test'
import VpnChallengeDialog from '@/components/vpn/VpnChallengeDialog.vue'
import VpnConnectionHero from '@/components/vpn/VpnConnectionHero.vue'
import { createTestProfile, createTestVpnState } from '../fixtures/profiles'

const stubs = {
  AppDialogContent: { template: '<div><slot /></div>' },
}

describe('VPN challenge dialog', () => {
  it('allows the hero cancel button while connect is still pending', () => {
    const wrapper = mount(VpnConnectionHero, {
      props: {
        profile: createTestProfile(),
        state: createTestVpnState({ status: 'connecting', profileId: 'profile-1' }),
        elapsedText: '',
        busy: true,
      },
    })
    expect(
      wrapper.find<HTMLButtonElement>('[data-testid="vpn-connect-toggle"]').element.disabled,
    ).toBe(false)
    wrapper.unmount()
  })
  it('opens SAML automatically and lets the user reopen the browser', async () => {
    const url = 'https://vpn.example.com:10443/remote/saml/start'
    const wrapper = mount(VpnChallengeDialog, {
      props: { prompt: { id: 'saml-1', kind: 'saml', message: 'SSO', url } },
      global: { stubs },
    })
    expect(wrapper.emitted('openSaml')).toEqual([[url]])
    await wrapper.find('[data-testid="challenge-open-saml-btn"]').trigger('click')
    expect(wrapper.emitted('openSaml')).toEqual([[url], [url]])
    wrapper.unmount()
  })

  it('accepts an empty push approval response or a backup token', async () => {
    const wrapper = mount(VpnChallengeDialog, {
      props: { prompt: { id: 'push-1', kind: 'ftm_push', message: 'Approve push', url: '' } },
      global: { stubs },
    })
    await wrapper.find('form').trigger('submit')
    expect(wrapper.emitted('submit')).toEqual([['', 'push-1']])
    await wrapper.find('[data-testid="challenge-otp-input"]').setValue('123456')
    await wrapper.find('form').trigger('submit')
    expect(wrapper.emitted('submit')).toEqual([
      ['', 'push-1'],
      ['123456', 'push-1'],
    ])
    wrapper.unmount()
  })

  it('keeps cancellation available while an OTP response is pending', async () => {
    const wrapper = mount(VpnChallengeDialog, {
      props: {
        prompt: { id: 'otp-1', kind: 'otp', message: 'OTP', url: '' },
        submitting: true,
        submitError: '重试验证',
      },
      global: { stubs },
    })
    expect(
      wrapper.find<HTMLButtonElement>('[data-testid="challenge-submit-btn"]').element.disabled,
    ).toBe(true)
    expect(wrapper.text()).toContain('重试验证')
    await wrapper.find('[data-testid="challenge-cancel-btn"]').trigger('click')
    expect(wrapper.emitted('cancel')).toHaveLength(1)
    wrapper.unmount()
  })
})
