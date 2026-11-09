import type { Plugin } from 'vite-plus'

const clean = (id: string) => id.split('?')[0]!.replaceAll('\\', '/')

export function runtimeBoundaryViolation(
  frontendRoot: string,
  source: string,
  importer?: string,
): string | undefined {
  if (!importer) return undefined
  const file = clean(importer).toLowerCase()
  const root = clean(frontendRoot).replace(/\/$/, '').toLowerCase()
  if (!file.startsWith(root + '/src/')) return undefined
  if (file.startsWith(root + '/src/api/')) return undefined

  const normalizedSource = clean(source).toLowerCase()
  if (
    normalizedSource.startsWith('@bindings/') ||
    normalizedSource.includes('/bindings/') ||
    normalizedSource === '@wailsio/runtime' ||
    normalizedSource.startsWith('@wailsio/runtime/')
  ) {
    return `仅 src/api/* 允许直接导入 Wails 绑定或运行时 (${source}): ${clean(importer)}`
  }
  return undefined
}

export function runtimeBoundary(frontendRoot: string): Plugin {
  return {
    name: 'tunnelkit-runtime-boundary',
    enforce: 'pre',
    resolveId(source, importer) {
      const violation = runtimeBoundaryViolation(frontendRoot, source, importer)
      if (violation) this.error(violation)
      return null
    },
  }
}
