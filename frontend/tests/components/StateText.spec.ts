import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vite-plus/test'
import StateText from '@/components/common/StateText.vue'

describe('state text transitions', () => {
  it('removes previous labels immediately and does not replay animation for clock ticks', async () => {
    const wrapper = mount(StateText, {
      props: { text: '已连接 · 1 秒', transitionKey: 'connected' },
      global: { stubs: { transition: false } },
    })
    const initial = wrapper.get('span > span').element
    await wrapper.setProps({ text: '已连接 · 2 秒' })
    expect(wrapper.get('span > span').element).toBe(initial)
    await wrapper.setProps({ text: '正在断开…', transitionKey: 'disconnecting' })
    await wrapper.setProps({ text: '未连接', transitionKey: 'disconnected' })
    expect(wrapper.text()).toBe('未连接')
    wrapper.unmount()
  })
})
