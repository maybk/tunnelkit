import type { SystemInfo, UpdateInfo, UpdateProgress } from '@/types/system'
import { isTrustedReleaseUrl, isTrustedRepositoryUrl } from '@/validate/update'
import * as SystemAPI from '@bindings/tunnelkit/internal/api/systemapi.js'
import * as updateModels from '@bindings/tunnelkit/internal/update/models.js'
import { Browser, Events } from '@wailsio/runtime'
import { mapSystemInfoFromDto, mapUpdateInfoFromDto, mapUpdateProgressFromDto } from './mappers'

export const EVENT_UPDATE_PROGRESS = 'update:progress'

export const systemApi = {
  async getInfo(): Promise<SystemInfo> {
    const dto = await SystemAPI.Info()
    return mapSystemInfoFromDto(dto)
  },

  async checkUpdate(): Promise<UpdateInfo> {
    const dto = await SystemAPI.CheckUpdate()
    return mapUpdateInfoFromDto(dto)
  },

  async applyUpdate(): Promise<void> {
    await SystemAPI.ApplyUpdate()
  },

  async openExternalUrl(url: string): Promise<void> {
    if (!isTrustedReleaseUrl(url) && !isTrustedRepositoryUrl(url)) {
      throw new Error('仅允许打开官方 GitHub 安全链接')
    }
    await Browser.OpenURL(url)
  },

  subscribeUpdateProgress(listener: (progress: UpdateProgress) => void): () => void {
    return Events.On(EVENT_UPDATE_PROGRESS, (event: { data?: unknown }) => {
      const dto = updateModels.Progress.createFrom(event.data ?? {})
      listener(mapUpdateProgressFromDto(dto))
    })
  },
}
