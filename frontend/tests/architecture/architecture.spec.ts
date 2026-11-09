// @vitest-environment node
import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, relative, resolve } from 'node:path'
import ts from 'typescript'
import { describe, expect, it } from 'vite-plus/test'

const sourceRoot = resolve(process.cwd(), 'src')

function listFiles(folder: string, extensions: string[] = ['.ts', '.vue']): string[] {
  if (!existsSync(folder)) return []
  return readdirSync(folder, { withFileTypes: true }).flatMap((entry) => {
    const file = resolve(folder, entry.name)
    if (entry.isDirectory()) return listFiles(file, extensions)
    return extensions.some((ext) => file.endsWith(ext)) ? [file] : []
  })
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

  it('Rule 2a: components/ui/ remains decoupled from business layers', () => {
    const violations: string[] = []
    for (const file of listFiles(resolve(sourceRoot, 'components/ui'))) {
      for (const spec of collectAllImports(file)) {
        if (
          spec.startsWith('@/') &&
          !spec.startsWith('@/lib/') &&
          !spec.startsWith('@/components/ui/')
        ) {
          violations.push(`${rel(file)} -> ${spec}`)
        }
      }
    }
    expect(violations).toEqual([])
  })

  it('project base components depend only on ui, base and shared UI utilities', () => {
    const violations: string[] = []
    for (const file of listFiles(resolve(sourceRoot, 'components/base'))) {
      for (const spec of collectAllImports(file)) {
        if (
          spec.startsWith('@/') &&
          !['@/components/ui/', '@/components/base/', '@/lib/'].some((prefix) =>
            spec.startsWith(prefix),
          )
        ) {
          violations.push(`${rel(file)} -> ${spec}`)
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

  it('source avoids unsafe HTML rendering and unchecked double assertions', () => {
    const violations: string[] = []
    for (const file of listFiles(sourceRoot, ['.vue', '.ts'])) {
      const r = rel(file)
      const raw = readFileSync(file, 'utf8')
      const content = extractTsSource(file)
      if (/\bas\s+unknown\s+as\b/.test(content)) {
        violations.push(`${r}: as unknown as`)
      }
      if (file.endsWith('.vue') && /\bv-html\b/.test(raw)) {
        violations.push(`${r}: v-html`)
      }
    }
    expect(violations).toEqual([])
  })

  it('Rule 9: frontend/src and frontend/tests never import outside the frontend/ root directory', () => {
    const frontendRoot = resolve(process.cwd())
    const testsRoot = resolve(frontendRoot, 'tests')
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
