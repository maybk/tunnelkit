<script setup lang="ts">
import { AlertCircle, ShieldCheck } from 'lucide-vue-next'
import type { VpnError } from '@/api/vpn'
import CopyButton from '@/components/common/CopyButton.vue'
import { Button } from '@/components/ui/button'

defineProps<{
  error: VpnError
  busy: boolean
}>()

const emit = defineEmits<{
  trustAndReconnect: [sha256: string]
}>()
</script>

<template>
  <!-- 外层 rounded-2xl (16px) = 内层指纹盒 rounded-xl (12px) + 间距，严格满足同心圆角 -->
  <div
    role="alert"
    class="rounded-2xl border border-status-danger/30 bg-status-danger/[0.07] p-4 text-body shadow-2xs"
  >
    <div class="flex items-start gap-3">
      <div
        class="flex size-8 shrink-0 items-center justify-center rounded-lg bg-status-danger/15 text-status-danger"
      >
        <AlertCircle class="size-4" aria-hidden="true" />
      </div>

      <div class="min-w-0 flex-1 space-y-1.5">
        <div class="font-semibold text-foreground">{{ error.message }}</div>
        <p v-if="error.hint" class="text-caption leading-relaxed text-muted-foreground">
          {{ error.hint }}
        </p>

        <div
          v-if="error.trustedCert"
          class="mt-3 space-y-2.5 rounded-xl border border-border bg-card/90 p-3 shadow-2xs"
        >
          <div class="flex items-center justify-between gap-2">
            <span class="text-caption font-medium text-muted-foreground">
              服务器证书指纹 (SHA-256)
            </span>
            <CopyButton :text="error.trustedCert" />
          </div>
          <code
            class="block rounded-lg bg-muted/70 px-2.5 py-1.5 font-mono text-caption break-all text-foreground"
          >
            {{ error.trustedCert }}
          </code>
          <div class="pt-0.5">
            <Button
              size="sm"
              :disabled="busy"
              @click="emit('trustAndReconnect', error.trustedCert)"
            >
              <ShieldCheck class="size-3.5" aria-hidden="true" />
              信任此证书并重连
            </Button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
