import type { VariantProps } from 'class-variance-authority'
import { cva } from 'class-variance-authority'

export { default as Button } from './Button.vue'

export const buttonVariants = cva(
  'focus-visible:border-ring focus-visible:ring-ring/35 aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 aria-invalid:border-destructive dark:aria-invalid:border-destructive/50 rounded-lg border border-transparent bg-clip-padding text-body font-medium focus-visible:ring-3 aria-invalid:ring-3 active:not-aria-[haspopup]:scale-[0.98] [&_svg:not([class*=size-])]:size-4 group/button inline-flex shrink-0 items-center justify-center whitespace-nowrap transition-[color,background-color,border-color,box-shadow,transform,opacity] duration-150 ease-out outline-none select-none disabled:pointer-events-none disabled:opacity-45 [&_svg]:pointer-events-none [&_svg]:shrink-0',
  {
    variants: {
      variant: {
        default:
          'bg-primary text-primary-foreground shadow-[0_1px_2px_rgba(0,0,0,0.12),inset_0_1px_0_rgba(255,255,255,0.22)] hover:bg-primary/92 active:bg-primary/96',
        outline:
          'border-border bg-card text-foreground shadow-[var(--shadow-control)] hover:bg-muted/75 hover:text-foreground dark:bg-card/85 dark:border-input dark:hover:bg-accent/60 aria-expanded:bg-muted aria-expanded:text-foreground',
        secondary:
          'bg-secondary text-secondary-foreground shadow-2xs hover:bg-secondary/80 aria-expanded:bg-secondary aria-expanded:text-secondary-foreground',
        ghost:
          'hover:bg-accent/80 hover:text-accent-foreground dark:hover:bg-accent/60 aria-expanded:bg-muted aria-expanded:text-foreground',
        destructive:
          'border-destructive/25 bg-destructive/[0.08] text-destructive shadow-2xs hover:border-destructive/35 hover:bg-destructive/[0.14] active:bg-destructive/[0.18] focus-visible:ring-destructive/25 dark:border-destructive/35 dark:bg-destructive/15 dark:hover:bg-destructive/22',
        link: 'text-primary underline-offset-4 hover:underline',
      },
      size: {
        default:
          'h-8.5 gap-1.5 px-3.5 has-data-[icon=inline-end]:pr-3 has-data-[icon=inline-start]:pl-3',
        xs: 'h-7 gap-1 rounded-md px-2.5 text-caption [&_svg:not([class*=size-])]:size-3',
        sm: 'h-8 gap-1.5 rounded-lg px-3 [&_svg:not([class*=size-])]:size-3.5',
        lg: 'h-9 gap-1.5 rounded-lg px-4 text-body font-medium',
        icon: 'size-8.5',
        'icon-xs': 'size-7 rounded-md [&_svg:not([class*=size-])]:size-3.5',
        'icon-sm': 'size-8 rounded-lg',
        'icon-lg': 'size-9 rounded-lg',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
)
export type ButtonVariants = VariantProps<typeof buttonVariants>
