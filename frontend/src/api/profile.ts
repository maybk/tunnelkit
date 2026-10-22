import { DEFAULT_VPN_PORT } from '@/lib/profileForm'
import { ProfileAPI } from '../../bindings/tunnelkit/internal/api'
import type * as api from '../../bindings/tunnelkit/internal/api'
import type * as profile from '../../bindings/tunnelkit/internal/profile'
import type * as fortivpn from '../../bindings/tunnelkit/fortivpn'

export type Profile = profile.Profile
export type ProfileInput = profile.Input
export type ParsedProfileText = api.ParsedProfileText
export type ConfigWarning = fortivpn.Warning

export { DEFAULT_VPN_PORT }

export const listProfiles = (): Promise<Profile[]> => ProfileAPI.List()

export const saveProfile = (input: ProfileInput): Promise<Profile> => ProfileAPI.Save(input)

export const deleteProfile = (id: string): Promise<void> => ProfileAPI.Delete(id)

export const trustProfileCert = (id: string, sha256: string): Promise<Profile> =>
  ProfileAPI.TrustCert(id, sha256)

export const parseProfileText = (base: ProfileInput, text: string): Promise<ParsedProfileText> =>
  ProfileAPI.ParseText(base, text)

export const formatProfileText = (
  input: ProfileInput,
  hasSavedPassword: boolean,
): Promise<string> => ProfileAPI.FormatText(input, hasSavedPassword)
