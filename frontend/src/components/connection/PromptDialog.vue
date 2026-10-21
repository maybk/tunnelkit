<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Browser } from '@wailsio/runtime'
import { ExternalLink, KeyRound, Loader2, Smartphone } from 'lucide-vue-next'
import type { VpnPrompt } from '@/api/vpn'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

const props = defineProps<{
  prompt: VpnPrompt | null | undefined
  busy?: boolean
}>()

const emit = defineEmits<{
  submit: [promptId: string, value: string]
  cancel: [promptId: string]
}>()

const code = ref('')
const open = computed(() => Boolean(props.prompt))

watch(
  () => props.prompt?.id,
  () => {
    code.value = ''
    if (props.prompt?.kind === 'saml' && props.prompt.url) {
      Browser.OpenURL(props.prompt.url).catch(() => {})
    }
  },
  { immediate: true },
)

const title = computed(() => {
  switch (props.prompt?.kind) {
    case 'ftm_push':
      return 'FortiToken 移动端确认'
    case 'saml':
      return 'SAML 单点登录验证'
    default:
      return '输入双因素验证码'
  }
})

const handleSubmit = () => {
  if (!props.prompt) return
  if (props.prompt.kind === 'otp' && !code.value.trim()) return
  emit('submit', props.prompt.id, code.value.trim())
}

const handleCancel = () => {
  if (!props.prompt) return
  emit('cancel', props.prompt.id)
}

const openSamlBrowser = () => {
  if (props.prompt?.url) {
    Browser.OpenURL(props.prompt.url).catch(() => {})
  }
}
</script>

<template>
  <Dialog :open="open" @update:open="(val) => !val && handleCancel()">
    <DialogContent>
      <div class="flex items-center gap-3.5">
        <div
          class="flex size-11 shrink-0 items-center justify-center rounded-xl bg-primary text-primary-foreground shadow-[0_2px_8px_color-mix(in_oklch,var(--primary)_35%,transparent)]"
        >
          <Smartphone v-if="prompt?.kind === 'ftm_push'" class="size-5" aria-hidden="true" />
          <ExternalLink v-else-if="prompt?.kind === 'saml'" class="size-5" aria-hidden="true" />
          <KeyRound v-else class="size-5" aria-hidden="true" />
        </div>
        <div>
          <DialogTitle>{{ title }}</DialogTitle>
          <DialogDescription class="mt-0.5">
            {{ prompt?.message || '请完成网关要求的安全验证以继续建立隧道' }}
          </DialogDescription>
        </div>
      </div>

      <form class="mt-4 space-y-4" @submit.prevent="handleSubmit">
        <div v-if="prompt?.kind === 'saml'" class="space-y-3">
          <div
            class="rounded-xl border border-border bg-muted/40 p-3.5 text-caption text-muted-foreground"
          >
            已尝试在系统默认浏览器中打开单点登录页面。请在浏览器中完成登录，验证通过后连接会自动继续。
          </div>
          <Button type="button" variant="outline" class="w-full" @click="openSamlBrowser">
            <ExternalLink class="size-3.5" aria-hidden="true" />
            重新打开浏览器登录页
          </Button>
        </div>

        <div v-else class="space-y-1.5">
          <Label for="vpn-otp-code">
            {{
              prompt?.kind === 'ftm_push'
                ? '在手机上点击批准后直接确认，或在此输入令牌口令'
                : '动态验证码（OTP / Token）'
            }}
          </Label>
          <Input
            id="vpn-otp-code"
            v-model="code"
            autofocus
            autocomplete="one-time-code"
            class="font-mono tracking-wider"
            :required="prompt?.kind === 'otp'"
            :placeholder="
              prompt?.kind === 'ftm_push'
                ? '留空触发手机推送，或输入 6 位口令'
                : '输入 6 位动态验证码'
            "
          />
        </div>

        <div class="flex items-center justify-end gap-2.5 pt-1">
          <Button type="button" variant="outline" :disabled="busy" @click="handleCancel">
            取消连接
          </Button>
          <Button
            v-if="prompt?.kind !== 'saml'"
            type="submit"
            :disabled="busy || (prompt?.kind === 'otp' && !code.trim())"
          >
            <Loader2 v-if="busy" class="size-3.5 animate-spin" aria-hidden="true" />
            {{ prompt?.kind === 'ftm_push' && !code.trim() ? '已批准推送 / 继续' : '提交验证' }}
          </Button>
        </div>
      </form>
    </DialogContent>
  </Dialog>
</template>
