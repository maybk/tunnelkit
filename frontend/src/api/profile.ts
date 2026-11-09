import type { ParsedProfileText, Profile, ProfileInput } from '@/types/profile'
import * as ProfileAPI from '@bindings/tunnelkit/internal/api/profileapi.js'
import { mapParsedProfileTextFromDto, mapProfileFromDto, mapProfileInputToDto } from './mappers'

export const profileApi = {
  async list(): Promise<Profile[]> {
    const items = await ProfileAPI.List()
    return (items ?? []).map((item) => mapProfileFromDto(item))
  },

  async save(input: ProfileInput): Promise<Profile> {
    const saved = await ProfileAPI.Save(mapProfileInputToDto(input))
    return mapProfileFromDto(saved)
  },

  async remove(id: string): Promise<void> {
    await ProfileAPI.Delete(id)
  },

  async trustCert(id: string, sha256: string): Promise<Profile> {
    const updated = await ProfileAPI.TrustCert(id, sha256)
    return mapProfileFromDto(updated)
  },

  async formatText(input: ProfileInput, hasSavedPassword: boolean): Promise<string> {
    return ProfileAPI.FormatText(mapProfileInputToDto(input), hasSavedPassword)
  },

  async parseText(base: ProfileInput, text: string): Promise<ParsedProfileText> {
    const parsed = await ProfileAPI.ParseText(mapProfileInputToDto(base), text)
    return mapParsedProfileTextFromDto(parsed)
  },
}
