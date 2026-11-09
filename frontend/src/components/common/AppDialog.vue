<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import AppDialogContent, { type AppDialogSize } from '@/components/base/AppDialogContent.vue'
import AppDialogDescription from '@/components/base/AppDialogDescription.vue'
import AppDialogTitle from '@/components/base/AppDialogTitle.vue'
import { Dialog, DialogFooter, DialogHeader } from '@/components/ui/dialog'
import { cn } from '@/lib/utils'

export type { AppDialogSize } from '@/components/base/AppDialogContent.vue'

const open = defineModel<boolean>('open', { required: true })

const props = withDefaults(
  defineProps<{
    title: string
    description?: string
    size?: AppDialogSize
    contentClass?: HTMLAttributes['class']
    bodyClass?: HTMLAttributes['class']
    footerClass?: HTMLAttributes['class']
  }>(),
  {
    description: undefined,
    size: 'md',
    contentClass: undefined,
    bodyClass: undefined,
    footerClass: undefined,
  },
)
</script>

<template>
  <Dialog v-model:open="open">
    <AppDialogContent :size="props.size" :class="cn('gap-0 p-0', props.contentClass)">
      <DialogHeader class="shrink-0 border-b border-border/60 px-5 py-3.5 pr-12">
        <AppDialogTitle class="text-title font-semibold text-foreground">
          {{ title }}
        </AppDialogTitle>
        <AppDialogDescription
          :class="description ? 'text-caption text-muted-foreground' : 'sr-only'"
        >
          {{ description || title }}
        </AppDialogDescription>
      </DialogHeader>

      <div :class="cn('min-h-0 w-full flex-1 overflow-y-auto px-5 py-4', props.bodyClass)">
        <slot />
      </div>

      <DialogFooter
        v-if="$slots.footer"
        :class="
          cn(
            'm-0 flex shrink-0 items-center justify-end gap-2 border-t border-border/60 bg-muted/40 px-5 py-3',
            props.footerClass,
          )
        "
      >
        <slot name="footer" />
      </DialogFooter>
    </AppDialogContent>
  </Dialog>
</template>
