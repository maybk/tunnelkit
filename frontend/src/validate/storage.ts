import * as v from 'valibot'
import { DEFAULT_THEME_MODE, THEME_MODES } from '@/constants/appearance'
import type { CachedAppearance } from '@/types/appearance'

const AppearanceObjectSchema = v.object({
  mode: v.optional(v.fallback(v.picklist(THEME_MODES), DEFAULT_THEME_MODE), DEFAULT_THEME_MODE),
})

const JsonStringToAppearanceSchema = v.pipe(v.string(), v.parseJson(), AppearanceObjectSchema)

export const DEFAULT_APPEARANCE: CachedAppearance = {
  mode: DEFAULT_THEME_MODE,
}

export function parseCachedAppearance(raw: string | null | undefined): CachedAppearance {
  if (!raw) return { ...DEFAULT_APPEARANCE }
  const parsed = v.safeParse(JsonStringToAppearanceSchema, raw)
  if (!parsed.success) return { ...DEFAULT_APPEARANCE }
  return {
    mode: parsed.output.mode,
  }
}
