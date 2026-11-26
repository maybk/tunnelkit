import type { VariantProps } from 'class-variance-authority'
import { cva } from 'class-variance-authority'

export const cardVariants = cva('text-card-foreground', {
  variants: {
    variant: {
      elevated: 'surface-card rounded-2xl',
      interactive: 'surface-card-interactive rounded-2xl',
      divided: 'surface-card divide-y divide-border/50 overflow-hidden rounded-2xl',
    },
    padding: {
      none: 'p-0',
      sm: 'p-4',
      md: 'p-5',
      lg: 'p-6',
    },
  },
  defaultVariants: {
    variant: 'elevated',
    padding: 'md',
  },
})

export type CardVariants = VariantProps<typeof cardVariants>
