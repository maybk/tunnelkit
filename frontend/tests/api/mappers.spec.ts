import { describe, expect, it } from 'vite-plus/test'
import {
  mapLogViewFromDto,
  mapProfileFromDto,
  mapProfileInputToDto,
  mapSystemInfoFromDto,
  mapUpdateInfoFromDto,
  mapUpdateProgressFromDto,
  mapVpnStateFromDto,
} from '@/api/mappers'
import * as fortivpnModels from '@bindings/tunnelkit/fortivpn/models.js'
import * as vpnModels from '@bindings/tunnelkit/internal/vpn/models.js'
import { createTestProfileInput } from '../fixtures/profiles'

describe('api mappers', () => {
  it('normalizes empty errors while keeping genuine errors and their details', () => {
    for (const error of [
      undefined,
      null,
      new vpnModels.Error(),
      new vpnModels.Error({ message: '  ' }),
    ]) {
      expect(mapVpnStateFromDto({ error }).error).toBeNull()
    }
    const error = new vpnModels.Error({
      message: '网络恢复失败',
      hint: '请查看日志',
      trustedCert: 'synthetic-fingerprint',
    })
    expect(mapVpnStateFromDto({ error }).error).toEqual(error)
    expect(
      mapVpnStateFromDto({ error: new vpnModels.Error({ hint: '请查看日志' }) }).error?.hint,
    ).toBe('请查看日志')
    expect(
      mapVpnStateFromDto({ error: new vpnModels.Error({ hint: '请查看日志' }) }).error?.message,
    ).toBeTruthy()
  })

  it('only exposes a supported challenge in the waiting-input state', () => {
    const prompt = new fortivpnModels.Prompt({
      id: 'synthetic',
      kind: fortivpnModels.PromptKind.PromptOTP,
    })
    expect(
      mapVpnStateFromDto({
        status: vpnModels.Status.StatusConnected,
        phase: fortivpnModels.Phase.PhaseConnected,
        prompt,
      }).prompt,
    ).toBeNull()
    expect(
      mapVpnStateFromDto({
        status: vpnModels.Status.StatusConnecting,
        phase: fortivpnModels.Phase.PhaseWaitingInput,
        prompt,
      }).prompt?.id,
    ).toBe('synthetic')
    expect(
      mapVpnStateFromDto({
        status: vpnModels.Status.StatusConnecting,
        phase: fortivpnModels.Phase.PhaseWaitingInput,
        prompt: new fortivpnModels.Prompt(),
      }).prompt,
    ).toBeNull()
  })

  it('normalizes empty or non-connected tunnel info and stats to null', () => {
    expect(
      mapVpnStateFromDto({
        status: vpnModels.Status.StatusConnected,
        phase: fortivpnModels.Phase.PhaseConnected,
        info: new fortivpnModels.TunnelInfo(),
        stats: new fortivpnModels.Stats(),
      }),
    ).toMatchObject({
      info: null,
      stats: { rxBytes: 0, txBytes: 0, rxPackets: 0, txPackets: 0, rttMillis: 0 },
    })
    expect(
      mapVpnStateFromDto({
        status: vpnModels.Status.StatusConnecting,
        phase: fortivpnModels.Phase.PhaseReconnecting,
        connectedAt: '2026-11-11T00:00:00Z',
        info: new fortivpnModels.TunnelInfo({ localIp: '10.212.0.10' }),
        stats: new fortivpnModels.Stats({ rxBytes: 512 }),
      }),
    ).toMatchObject({
      connectedAt: '',
      info: null,
      stats: null,
    })
  })
  it('maps binding profile and profile input models cleanly', () => {
    const profile = mapProfileFromDto({
      id: 'p1',
      name: '节点 1',
      host: 'vpn.example.com',
      port: 8443,
      realm: '',
      username: 'bob',
      trustedCert: '',
      auth: 'password',
      routes: 'gateway',
      extraRoutes: ['10.0.0.0/8'],
      dns: 'split',
      dnsDomains: ['corp.local'],
      reconnect: true,
      hasPassword: true,
      updatedAt: '2026-04-01T00:00:00Z',
    })
    expect(profile.port).toBe(8443)
    expect(profile.extraRoutes).toEqual(['10.0.0.0/8'])

    const bindingInput = mapProfileInputToDto(createTestProfileInput({ port: 10443 }))
    expect(bindingInput.port).toBe(10443)
  })

  it('maps binding VPN state, log view, system info, update info, and progress safely', () => {
    const state = mapVpnStateFromDto({
      status: vpnModels.Status.StatusConnected,
      phase: fortivpnModels.Phase.PhaseConnected,
      profileId: 'p1',
      profileName: '节点 1',
      connectedAt: '2026-04-01T00:00:00Z',
    })
    expect(state.status).toBe('connected')
    expect(state.profileId).toBe('p1')

    const logView = mapLogViewFromDto({
      profileId: 'p1',
      content: '2026-04-01T00:00:00Z [INFO] tunnel up',
      exists: true,
      updatedAt: '2026-04-01T00:00:00Z',
    })
    expect(logView.exists).toBe(true)

    const sys = mapSystemInfoFromDto({
      appVersion: '0.3.2',
      platform: 'darwin/arm64',
      helperReady: true,
      helperPath: '/Library/PrivilegedHelperTools/tunnelkit-helper',
    })
    expect(sys.appVersion).toBe('0.3.2')

    const update = mapUpdateInfoFromDto({
      currentVersion: '0.3.2',
      latestVersion: '0.4.0',
      hasUpdate: true,
      releaseUrl: 'https://github.com/maybk/tunnelkit/releases/tag/v0.4.0',
      canSelfUpdate: true,
      assetName: 'tunnelkit-darwin-arm64.tar.gz',
      assetSize: 1024,
    })
    expect(update.hasUpdate).toBe(true)

    const progress = mapUpdateProgressFromDto({
      stage: 'downloading',
      percent: 45,
      downloadedBytes: 450,
      totalBytes: 1000,
      message: '下载中',
    })
    expect(progress.stage).toBe('downloading')
    expect(progress.percent).toBe(45)
  })
})
