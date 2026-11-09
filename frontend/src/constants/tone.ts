export type Tone = 'neutral' | 'brand' | 'info' | 'success' | 'warning' | 'danger'

export interface ToneStyleSet {
  tone: Tone
  pill: string
  badge: string
  dot: string
  iconBox: string
  text: string
  border: string
  softBg: string
  solidBg: string
  panel: string
}

export const TONE_CLASSES: Record<Tone, ToneStyleSet> = {
  neutral: {
    tone: 'neutral',
    pill: 'bg-muted text-muted-foreground',
    badge: 'bg-tone-neutral-soft text-tone-neutral-fg border-tone-neutral-border',
    dot: 'bg-tone-neutral-solid/75',
    iconBox: 'bg-tone-neutral-soft text-tone-neutral-fg border-tone-neutral-border',
    text: 'text-tone-neutral-fg',
    border: 'border-tone-neutral-border',
    softBg: 'bg-tone-neutral-soft',
    solidBg: 'bg-tone-neutral-solid',
    panel: 'bg-tone-neutral-soft border-tone-neutral-border text-tone-neutral-fg',
  },
  brand: {
    tone: 'brand',
    pill: 'bg-tone-brand-soft text-tone-brand-fg',
    badge: 'bg-tone-brand-soft text-tone-brand-fg border-tone-brand-border',
    dot: 'bg-tone-brand-solid',
    iconBox: 'bg-tone-brand-soft text-tone-brand-fg border-tone-brand-border',
    text: 'text-tone-brand-fg',
    border: 'border-tone-brand-border',
    softBg: 'bg-tone-brand-soft',
    solidBg: 'bg-tone-brand-solid',
    panel: 'bg-tone-brand-soft border-tone-brand-border text-tone-brand-fg',
  },
  info: {
    tone: 'info',
    pill: 'bg-tone-info-soft text-tone-info-fg',
    badge: 'bg-tone-info-soft text-tone-info-fg border-tone-info-border',
    dot: 'bg-tone-info-solid',
    iconBox: 'bg-tone-info-soft text-tone-info-fg border-tone-info-border',
    text: 'text-tone-info-fg',
    border: 'border-tone-info-border',
    softBg: 'bg-tone-info-soft',
    solidBg: 'bg-tone-info-solid',
    panel: 'bg-tone-info-soft border-tone-info-border text-tone-info-fg',
  },
  success: {
    tone: 'success',
    pill: 'bg-tone-success-solid/14 text-tone-success-solid',
    badge: 'bg-tone-success-soft text-tone-success-fg border-tone-success-border',
    dot: 'bg-tone-success-solid shadow-[0_0_0_3px_color-mix(in_oklch,var(--tone-success-solid)_22%,transparent)]',
    iconBox: 'bg-tone-success-soft text-tone-success-fg border-tone-success-border',
    text: 'text-tone-success-fg',
    border: 'border-tone-success-border',
    softBg: 'bg-tone-success-soft',
    solidBg: 'bg-tone-success-solid',
    panel: 'bg-tone-success-soft border-tone-success-border text-tone-success-fg',
  },
  warning: {
    tone: 'warning',
    pill: 'bg-tone-warning-solid/16 text-tone-warning-solid',
    badge: 'bg-tone-warning-soft text-tone-warning-fg border-tone-warning-border',
    dot: 'bg-tone-warning-solid shadow-[0_0_0_3px_color-mix(in_oklch,var(--tone-warning-solid)_22%,transparent)]',
    iconBox: 'bg-tone-warning-soft text-tone-warning-fg border-tone-warning-border',
    text: 'text-tone-warning-fg',
    border: 'border-tone-warning-border',
    softBg: 'bg-tone-warning-soft',
    solidBg: 'bg-tone-warning-solid',
    panel: 'bg-tone-warning-soft border-tone-warning-border text-tone-warning-fg',
  },
  danger: {
    tone: 'danger',
    pill: 'bg-tone-danger-solid/14 text-tone-danger-solid',
    badge: 'bg-tone-danger-soft text-tone-danger-fg border-tone-danger-border',
    dot: 'bg-tone-danger-solid shadow-[0_0_0_3px_color-mix(in_oklch,var(--tone-danger-solid)_22%,transparent)]',
    iconBox: 'bg-tone-danger-soft text-tone-danger-fg border-tone-danger-border',
    text: 'text-tone-danger-fg',
    border: 'border-tone-danger-border',
    softBg: 'bg-tone-danger-soft',
    solidBg: 'bg-tone-danger-solid',
    panel: 'bg-tone-danger-soft border-tone-danger-border text-tone-danger-fg',
  },
}

export function getToneClasses(tone: Tone = 'neutral'): ToneStyleSet {
  return TONE_CLASSES[tone] ?? TONE_CLASSES.neutral
}
