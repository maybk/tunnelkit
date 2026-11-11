import type { SystemInfo, UpdateInfo, UpdateProgress } from '@/types/system'
import { isTrustedReleaseUrl, isTrustedRepositoryUrl } from '@/validate/update'
import { UpdateProgress as EVENT_UPDATE_PROGRESS } from '@bindings/events/index.js'
import * as SystemAPI from '@bindings/tunnelkit/internal/api/systemapi.js'
import type * as updateModels from '@bindings/tunnelkit/internal/update/models.js'
import { Browser, Events } from '@wailsio/runtime'
import { mapSystemInfoFromDto, mapUpdateInfoFromDto, mapUpdateProgressFromDto } from './mappers'

export { EVENT_UPDATE_PROGRESS }

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
    return Events.On(EVENT_UPDATE_PROGRESS, (event: { data?: updateModels.Progress | null }) => {
      listener(mapUpdateProgressFromDto(event.data))
    })
  },
}
