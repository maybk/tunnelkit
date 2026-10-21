import { SystemAPI } from '../../bindings/tunnelkit/internal/api'
import type * as api from '../../bindings/tunnelkit/internal/api'

export type SystemInfo = api.SystemInfo

export const getSystemInfo = (): Promise<SystemInfo> => SystemAPI.Info()
