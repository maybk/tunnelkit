export type DesktopPlatform = 'macos' | 'windows' | 'linux' | 'web'

export interface DesktopWindowState {
  platform: DesktopPlatform
  isFocused: boolean
  isMaximised: boolean
  isFullscreen: boolean
}
