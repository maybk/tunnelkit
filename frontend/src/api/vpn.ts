import type { LogViewSnapshot } from '@/types/log'
import type { VpnState } from '@/types/vpn'
import { VpnState as EVENT_VPN_STATE } from '@bindings/events/index.js'
import * as VPNAPI from '@bindings/tunnelkit/internal/api/vpnapi.js'
import type * as vpnModels from '@bindings/tunnelkit/internal/vpn/models.js'
import { Browser, Events } from '@wailsio/runtime'
import { mapLogViewFromDto, mapVpnStateFromDto } from './mappers'

export { EVENT_VPN_STATE }

export const vpnApi = {
  async getState(): Promise<VpnState> {
    const dto = await VPNAPI.State()
    return mapVpnStateFromDto(dto)
  },

  async connect(profileId: string): Promise<void> {
    await VPNAPI.Connect(profileId)
  },

  async disconnect(): Promise<void> {
    await VPNAPI.Disconnect()
  },

  async cancel(): Promise<void> {
    await VPNAPI.Cancel()
  },

  async answerPrompt(promptId: string, value: string, cancel: boolean): Promise<void> {
    await VPNAPI.Answer(promptId, value, cancel)
  },

  async openAuthenticationUrl(rawUrl: string): Promise<void> {
    const url = new URL(rawUrl)
    if (url.protocol !== 'https:' || url.username || url.password) {
      throw new Error('网关登录地址无效')
    }
    await Browser.OpenURL(url.toString())
  },

  async getLog(profileId: string): Promise<LogViewSnapshot> {
    const dto = await VPNAPI.Log(profileId)
    return mapLogViewFromDto(dto)
  },

  async clearLog(profileId: string): Promise<void> {
    await VPNAPI.ClearLog(profileId)
  },

  subscribeState(listener: (state: VpnState) => void): () => void {
    return Events.On(EVENT_VPN_STATE, (event: { data?: vpnModels.State | null }) => {
      // Wails 已按注册的事件类型创建模型；再次转换会将 undefined 字段创建为空对象。
      listener(mapVpnStateFromDto(event.data))
    })
  },
}
