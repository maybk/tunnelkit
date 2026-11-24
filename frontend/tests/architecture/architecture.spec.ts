// @vitest-environment node
import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs'
import { basename, dirname, relative, resolve } from 'node:path'
import ts from 'typescript'
import { describe, expect, it } from 'vite-plus/test'

const frontendRoot = resolve(process.cwd())
const sourceRoot = resolve(frontendRoot, 'src')
const configRoot = resolve(frontendRoot, 'config')
const testsRoot = resolve(frontendRoot, 'tests')
const scriptExtensions = ['.ts', '.js', '.mjs', '.cjs']
const disallowedFrontendFilenamePattern =
  /^(?:extra|misc)(?:\.(?:spec|test))?\.(?:ts|js|mjs|cjs|vue)$|[._-](?:part\d+|split\d*|\d+)(?:\.(?:spec|test))?\.(?:ts|js|mjs|cjs|vue)$/i

function listFiles(folder: string, extensions: string[] = ['.ts', '.vue']): string[] {
  if (!existsSync(folder)) return []
  return readdirSync(folder, { withFileTypes: true }).flatMap((entry) => {
    if (entry.name.startsWith('.')) return []
    const file = resolve(folder, entry.name)
    if (entry.isDirectory()) return listFiles(file, extensions)
    return extensions.some((ext) => file.endsWith(ext)) ? [file] : []
  })
}

function countTextLines(raw: string): number {
  if (raw.length === 0) return 0
  const trimmed = raw.endsWith('\r\n')
    ? raw.slice(0, -2)
    : raw.endsWith('\n')
      ? raw.slice(0, -1)
      : raw
  return trimmed.split('\n').length
}

function isDisallowedFrontendFilename(filename: string): boolean {
  return disallowedFrontendFilenamePattern.test(filename)
}

function extractTsSource(file: string): string {
  const raw = readFileSync(file, 'utf8')
  if (!file.endsWith('.vue')) return raw
  const blocks = [...raw.matchAll(/<script\b[^>]*>([\s\S]*?)<\/script>/gi)]
  return blocks.map((match) => match[1] ?? '').join('\n')
}

function parseSource(file: string): ts.SourceFile {
  return ts.createSourceFile(file, extractTsSource(file), ts.ScriptTarget.Latest, true)
}

function canonicalSpecifier(file: string, specifier: string): string {
  if (!specifier.startsWith('.')) return specifier
  const target = relative(sourceRoot, resolve(dirname(file), specifier)).replaceAll('\\', '/')
  if (target.startsWith('../bindings/')) return `@bindings/${target.slice('../bindings/'.length)}`
  return target.startsWith('../') ? specifier : `@/${target}`
}

function collectImports(file: string, runtimeOnly: boolean): string[] {
  const specifiers: string[] = []
  const add = (node: ts.Expression) => {
    if (ts.isStringLiteral(node)) specifiers.push(canonicalSpecifier(file, node.text))
  }
  const visit = (node: ts.Node) => {
    if (ts.isImportDeclaration(node) && (!runtimeOnly || !node.importClause?.isTypeOnly))
      add(node.moduleSpecifier)
    if (ts.isExportDeclaration(node) && node.moduleSpecifier && (!runtimeOnly || !node.isTypeOnly))
      add(node.moduleSpecifier)
    if (
      ts.isCallExpression(node) &&
      node.expression.kind === ts.SyntaxKind.ImportKeyword &&
      node.arguments[0]
    )
      add(node.arguments[0])
    ts.forEachChild(node, visit)
  }
  visit(parseSource(file))
  return specifiers
}

function collectRuntimeImports(file: string): string[] {
  return collectImports(file, true)
}

function collectAllImports(file: string): string[] {
  return collectImports(file, false)
}

function resolveInternalModule(fromFile: string, specifier: string): string | undefined {
  const base = specifier.startsWith('@/')
    ? resolve(sourceRoot, specifier.slice(2))
    : specifier.startsWith('.')
      ? resolve(dirname(fromFile), specifier)
      : undefined
  if (!base) return undefined
  for (const candidate of [base, `${base}.ts`, `${base}.vue`, resolve(base, 'index.ts')]) {
    if (existsSync(candidate) && statSync(candidate).isFile()) return candidate
  }
  return `${base}.ts`
}

function rel(file: string): string {
  return relative(sourceRoot, file).replaceAll('\\', '/')
}

describe('frontend architecture', () => {
  it('Rule 1: views/, components/, layouts/, and composables/ do not directly import @/api, @bindings, or @wailsio/runtime, and composables/ stays decoupled from UI layers', () => {
    const files = [
      ...listFiles(resolve(sourceRoot, 'views')),
      ...listFiles(resolve(sourceRoot, 'components')),
      ...listFiles(resolve(sourceRoot, 'layouts')),
      ...listFiles(resolve(sourceRoot, 'composables')),
    ]
    const violations: string[] = []
    for (const file of files) {
      for (const spec of collectRuntimeImports(file)) {
        if (
          spec === '@/api' ||
          spec.startsWith('@/api/') ||
          spec === '@bindings' ||
          spec.startsWith('@bindings/') ||
          spec === '@wailsio/runtime' ||
          spec.startsWith('@wailsio/runtime/')
        ) {
          violations.push(`${rel(file)} -> ${spec}`)
        }
      }
    }
    const composableForbidden = ['@/views', '@/components', '@/layouts']
    for (const file of listFiles(resolve(sourceRoot, 'composables'), ['.ts'])) {
      for (const spec of collectAllImports(file)) {
        if (
          composableForbidden.some((prefix) => spec === prefix || spec.startsWith(`${prefix}/`))
        ) {
          violations.push(`${rel(file)} -> ${spec}`)
        }
      }
    }
    expect(violations).toEqual([])
  })

  it('Rule 2a: components/ui/, components/base/, and components/common/ respect layered component boundaries', () => {
    const allowedPrimitivePackages = [
      'vue',
      'reka-ui',
      '@vueuse/core',
      '@lucide/vue',
      'class-variance-authority',
      'clsx',
      'tailwind-merge',
    ]
    const isAllowedPrimitivePackage = (spec: string) =>
      allowedPrimitivePackages.some((pkg) => spec === pkg || spec.startsWith(`${pkg}/`))

    const violations: string[] = []
    for (const file of listFiles(resolve(sourceRoot, 'components/ui'))) {
      for (const spec of collectAllImports(file)) {
        if (spec.startsWith('@/')) {
          if (!spec.startsWith('@/lib/') && !spec.startsWith('@/components/ui/')) {
            violations.push(`${rel(file)} -> ${spec}`)
          }
        } else if (!spec.startsWith('.') && !isAllowedPrimitivePackage(spec)) {
          violations.push(`${rel(file)} -> ${spec}`)
        }
      }
    }
    for (const file of listFiles(resolve(sourceRoot, 'components/base'))) {
      for (const spec of collectAllImports(file)) {
        if (spec.startsWith('@/')) {
          if (
            !['@/components/ui/', '@/components/base/', '@/lib/'].some((prefix) =>
              spec.startsWith(prefix),
            )
          ) {
            violations.push(`${rel(file)} -> ${spec}`)
          }
        } else if (!spec.startsWith('.') && !isAllowedPrimitivePackage(spec)) {
          violations.push(`${rel(file)} -> ${spec}`)
        }
      }
    }
    for (const file of listFiles(resolve(sourceRoot, 'components/common'))) {
      for (const spec of collectAllImports(file)) {
        if (
          spec === '@/views' ||
          spec.startsWith('@/views/') ||
          spec === '@/layouts' ||
          spec.startsWith('@/layouts/') ||
          (spec.startsWith('@/components/') &&
            !spec.startsWith('@/components/ui/') &&
            !spec.startsWith('@/components/base/') &&
            !spec.startsWith('@/components/common/'))
        ) {
          violations.push(`${rel(file)} -> ${spec}`)
        }
      }
    }
    const upperLayerFiles = [
      ...listFiles(resolve(sourceRoot, 'views')),
      ...listFiles(resolve(sourceRoot, 'layouts')),
      ...listFiles(resolve(sourceRoot, 'components')).filter(
        (file) =>
          !rel(file).startsWith('components/ui/') &&
          !rel(file).startsWith('components/base/') &&
          !rel(file).startsWith('components/common/'),
      ),
    ]
    for (const file of upperLayerFiles) {
      for (const spec of collectAllImports(file)) {
        if (spec === 'reka-ui' || spec.startsWith('reka-ui/')) {
          violations.push(
            `${rel(file)} -> ${spec} (wrap reka-ui inside components/ui, base, or common)`,
          )
        }
      }
    }
    expect(violations).toEqual([])
  })

  it('technical infrastructure does not own UI or business orchestration', () => {
    const forbidden = [
      '@/views',
      '@/layouts',
      '@/components',
      '@/stores',
      '@/composables',
      '@/api',
      '@/router',
      '@bindings',
      '@wailsio/runtime',
    ]
    const violations: string[] = []
    for (const file of listFiles(resolve(sourceRoot, 'lib'))) {
      for (const spec of collectAllImports(file)) {
        if (forbidden.some((prefix) => spec === prefix || spec.startsWith(`${prefix}/`)))
          violations.push(`${rel(file)} -> ${spec}`)
      }
    }
    expect(violations).toEqual([])
  })

  it('Rule 2: utils/, domain/, validate/, and constants/ remain framework-free', () => {
    const pureFolders = ['utils', 'domain', 'validate', 'constants']
    const forbiddenPrefixes = [
      'vue',
      'pinia',
      'vue-router',
      '@/api',
      '@/stores',
      '@/composables',
      '@/components',
      '@/views',
      '@/layouts',
      '@/router',
      '@/presentation',
      '@bindings',
      '@wailsio/runtime',
    ]
    const violations: string[] = []
    for (const folder of pureFolders) {
      for (const file of listFiles(resolve(sourceRoot, folder), ['.ts'])) {
        for (const spec of collectAllImports(file)) {
          if (
            forbiddenPrefixes.some((prefix) => spec === prefix || spec.startsWith(`${prefix}/`))
          ) {
            violations.push(`${rel(file)} -> ${spec}`)
          }
        }
      }
    }
    expect(violations).toEqual([])
  })

  it('Rule 2b: presentation/ and router/ respect outbound dependency boundaries', () => {
    const presentationForbidden = [
      '@/api',
      '@/stores',
      '@/composables',
      '@/components',
      '@/views',
      '@/layouts',
      '@/router',
      '@bindings',
      '@wailsio/runtime',
    ]
    const routerForbidden = ['@/api', '@/stores', '@bindings', '@wailsio/runtime']
    const violations: string[] = []
    for (const file of listFiles(resolve(sourceRoot, 'presentation'), ['.ts'])) {
      for (const spec of collectAllImports(file)) {
        if (
          presentationForbidden.some((prefix) => spec === prefix || spec.startsWith(`${prefix}/`))
        ) {
          violations.push(`${rel(file)} -> ${spec}`)
        }
      }
    }
    for (const file of listFiles(resolve(sourceRoot, 'router'), ['.ts'])) {
      for (const spec of collectAllImports(file)) {
        if (routerForbidden.some((prefix) => spec === prefix || spec.startsWith(`${prefix}/`))) {
          violations.push(`${rel(file)} -> ${spec}`)
        }
      }
    }
    expect(violations).toEqual([])
  })

  it('Rule 3: stores/ do not import views/, components/, layouts/, router/, @bindings, or @wailsio/runtime', () => {
    const forbidden = [
      '@/views',
      '@/components',
      '@/layouts',
      '@/router',
      '@bindings',
      '@wailsio/runtime',
    ]
    const violations: string[] = []
    for (const file of listFiles(resolve(sourceRoot, 'stores'), ['.ts'])) {
      for (const spec of collectAllImports(file)) {
        if (forbidden.some((prefix) => spec === prefix || spec.startsWith(`${prefix}/`))) {
          violations.push(`${rel(file)} -> ${spec}`)
        }
      }
    }
    expect(violations).toEqual([])
  })

  it('Rule 4: api/ does not import stores/, composables/, components/, views/, router/, layouts/, or presentation/', () => {
    const forbidden = [
      '@/stores',
      '@/composables',
      '@/components',
      '@/views',
      '@/router',
      '@/layouts',
      '@/presentation',
    ]
    const violations: string[] = []
    for (const file of listFiles(resolve(sourceRoot, 'api'), ['.ts'])) {
      const r = rel(file)
      for (const spec of collectAllImports(file)) {
        if (forbidden.some((prefix) => spec === prefix || spec.startsWith(`${prefix}/`))) {
          violations.push(`${r} -> ${spec}`)
        }
      }
    }
    expect(violations).toEqual([])
  })

  it('Rule 5: types/ contains only type declarations or type-only imports/exports', () => {
    const files = listFiles(resolve(sourceRoot, 'types'), ['.ts'])
    for (const file of files) {
      for (const statement of parseSource(file).statements) {
        const typeOnly =
          ts.isInterfaceDeclaration(statement) ||
          ts.isTypeAliasDeclaration(statement) ||
          (ts.isImportDeclaration(statement) && Boolean(statement.importClause?.isTypeOnly)) ||
          (ts.isExportDeclaration(statement) && statement.isTypeOnly)
        expect(Boolean(typeOnly), `${rel(file)}: ${statement.getText().slice(0, 70)}`).toBe(true)
      }
    }
  })

  it('Rule 6: api/ and stores/ have no internal runtime dependency cycles', () => {
    for (const folder of ['api', 'stores']) {
      const folderPath = resolve(sourceRoot, folder)
      const modules = new Map<string, string[]>()
      for (const file of listFiles(folderPath, ['.ts'])) {
        const deps: string[] = []
        for (const spec of collectRuntimeImports(file)) {
          const target = resolveInternalModule(file, spec)
          if (target && target.startsWith(folderPath)) {
            deps.push(target)
          }
        }
        modules.set(file, deps)
      }
      const done = new Set<string>()
      const visiting = new Set<string>()
      function visit(file: string, chain: string[]) {
        if (done.has(file) || !modules.has(file)) return
        expect(visiting.has(file), [...chain, file].map(rel).join(' -> ')).toBe(false)
        visiting.add(file)
        for (const target of modules.get(file)!) visit(target, [...chain, file])
        visiting.delete(file)
        done.add(file)
      }
      for (const file of modules.keys()) visit(file, [])
    }
  })

  it('Rule 7: source avoids unsafe HTML rendering, native dialogs, debug logs, unchecked double assertions, and banned anti-patterns', () => {
    const violations: string[] = []
    for (const file of listFiles(sourceRoot, ['.vue', '.ts'])) {
      const r = rel(file)
      const raw = readFileSync(file, 'utf8')
      const content = extractTsSource(file)
      if (file.endsWith('.vue')) {
        const templateMatch = raw.match(/<template\b[^>]*>([\s\S]*?)<\/template>/i)
        if (templateMatch?.[1] && /\bv-html\b/.test(templateMatch[1])) {
          violations.push(`${r}: v-html`)
        }
      }
      if (/\b(?:window|globalThis)\.(?:alert|confirm|prompt)\s*\(/.test(content)) {
        violations.push(`${r}: window/globalThis alert/confirm/prompt`)
      }
      if (/\bconsole\.(?:log|debug)\s*\(/.test(content)) {
        violations.push(`${r}: console.log/debug`)
      }
      if (/\bcreateGlobalState\b/.test(content)) {
        violations.push(`${r}: createGlobalState`)
      }
      if (/\bas\s+unknown\s+as\b/.test(content)) {
        violations.push(`${r}: as unknown as`)
      }
      if (/\b(?:interface|type)\s+Raw[A-Z]\w*/.test(content)) {
        violations.push(`${r}: Raw* interface/type`)
      }
      if (/\bget\s+label\s*\(/.test(content)) {
        violations.push(`${r}: get label()`)
      }
      if (/\b(?:badgeClass|textClass|pillClass)\b/.test(content)) {
        violations.push(`${r}: badgeClass/textClass/pillClass`)
      }
    }
    expect(violations).toEqual([])
  })

  it('Rule 8: single-file line budget and semantic naming across frontend allowlist (src/*.vue <= 300, src|config scripts <= 400, tests scripts <= 800)', () => {
    const violations: string[] = []
    let checkedCount = 0
    const scopes: Array<{
      folder: string
      extensions: string[]
      limit: number
      isExempt: (relToFrontend: string) => boolean
    }> = [
      {
        folder: sourceRoot,
        extensions: ['.vue'],
        limit: 300,
        isExempt: (relToFrontend) => relToFrontend.startsWith('src/components/ui/'),
      },
      {
        folder: sourceRoot,
        extensions: scriptExtensions,
        limit: 400,
        isExempt: (relToFrontend) =>
          relToFrontend.endsWith('.d.ts') || relToFrontend.startsWith('src/components/ui/'),
      },
      {
        folder: configRoot,
        extensions: scriptExtensions,
        limit: 400,
        isExempt: (relToFrontend) => relToFrontend.endsWith('.d.ts'),
      },
      {
        folder: testsRoot,
        extensions: scriptExtensions,
        limit: 800,
        isExempt: (relToFrontend) =>
          relToFrontend.endsWith('.d.ts') || relToFrontend.startsWith('tests/fixtures/'),
      },
    ]

    for (const scope of scopes) {
      for (const file of listFiles(scope.folder, scope.extensions)) {
        const relToFrontend = relative(frontendRoot, file).replaceAll('\\', '/')
        if (scope.isExempt(relToFrontend)) continue
        checkedCount++
        if (isDisallowedFrontendFilename(basename(file))) {
          violations.push(`${relToFrontend} (disallowed generic or numbered split filename)`)
        }
        const lines = countTextLines(readFileSync(file, 'utf8'))
        if (lines > scope.limit) {
          violations.push(`${relToFrontend} (${lines} lines > ${scope.limit})`)
        }
      }
    }

    expect(checkedCount).toBeGreaterThan(0)
    expect(violations).toEqual([])
  })

  it('Rule 8a: line counter and filename guard behave accurately on edge cases', () => {
    expect(countTextLines('')).toBe(0)
    expect(countTextLines('\n')).toBe(1)
    expect(countTextLines('\r\n')).toBe(1)
    expect(countTextLines('const a = 1')).toBe(1)
    expect(countTextLines('const a = 1\n')).toBe(1)
    expect(countTextLines('const a = 1\r\n')).toBe(1)
    expect(countTextLines('a\nb\n')).toBe(2)
    expect(countTextLines('a\r\nb\r\n')).toBe(2)
    expect(countTextLines('a\n\n')).toBe(2)
    expect(countTextLines('x\n'.repeat(300))).toBe(300)

    for (const disallowed of [
      'extra.ts',
      'extra.spec.ts',
      'extra.test.ts',
      'misc.vue',
      'misc.spec.js',
      'WorkspaceView-2.vue',
      'WorkspaceView-2.spec.ts',
      'profile_2.ts',
      'mappers.part1.ts',
      'WorkspaceView_split.spec.ts',
    ]) {
      expect(isDisallowedFrontendFilename(disallowed), disallowed).toBe(true)
    }

    for (const allowed of [
      'ipv4.ts',
      'socks5.ts',
      'runtime-boundary.ts',
      'useVpnStore.ts',
      'WorkspaceView.spec.ts',
      'ProfileFormPanel.vue',
      'http2Client.ts',
    ]) {
      expect(isDisallowedFrontendFilename(allowed), allowed).toBe(false)
    }
  })

  it('Rule 9: frontend/src and frontend/tests never import outside the frontend/ root directory', () => {
    const files = [
      ...listFiles(sourceRoot, ['.ts', '.vue']),
      ...listFiles(testsRoot, ['.ts', '.vue']),
    ]
    const violations: string[] = []
    for (const file of files) {
      const r = relative(frontendRoot, file).replaceAll('\\', '/')
      const sf = parseSource(file)
      const checkSpec = (node: ts.Expression) => {
        if (!ts.isStringLiteral(node)) return
        const spec = node.text
        if (spec.startsWith('.')) {
          const resolved = resolve(dirname(file), spec)
          const relToRoot = relative(frontendRoot, resolved).replaceAll('\\', '/')
          if (relToRoot.startsWith('../') || relToRoot === '..') {
            violations.push(`${r} -> ${spec} (escapes frontend/)`)
          }
        }
      }
      const visit = (node: ts.Node) => {
        if (ts.isImportDeclaration(node)) checkSpec(node.moduleSpecifier)
        if (ts.isExportDeclaration(node) && node.moduleSpecifier) checkSpec(node.moduleSpecifier)
        if (
          ts.isCallExpression(node) &&
          node.expression.kind === ts.SyntaxKind.ImportKeyword &&
          node.arguments[0]
        ) {
          checkSpec(node.arguments[0])
        }
        ts.forEachChild(node, visit)
      }
      visit(sf)
    }
    expect(violations).toEqual([])
  })
})
