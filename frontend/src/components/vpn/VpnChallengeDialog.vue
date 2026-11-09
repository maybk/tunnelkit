<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ExternalLink, KeyRound, Loader2, Smartphone } from '@lucide/vue'
import AppDialogContent from '@/components/base/AppDialogContent.vue'
import AppDialogDescription from '@/components/base/AppDialogDescription.vue'
import AppDialogTitle from '@/components/base/AppDialogTitle.vue'
import { Dialog } from '@/components/ui/dialog'
import AppLabel from '@/components/base/AppLabel.vue'
import InlineAlert from '@/components/common/InlineAlert.vue'
import AppButton from '@/components/base/button/AppButton.vue'
import AppInput from '@/components/base/AppInput.vue'
import type { VpnPrompt } from '@/types/vpn'
import { validateOtpCode } from '@/validate/vpn'

const props = defineProps<{
  prompt: VpnPrompt | null
  submitting?: boolean
  submitError?: string
}>()

const emit = defineEmits<{
  submit: [value: string]
  cancel: []
  openSaml: [url: string]
}>()

const otpCode = ref('')
const localError = ref('')

const dialogOpen = computed({
  get: () => Boolean(props.prompt),
  set: (val: boolean) => {
    if (!val && props.prompt) {
      emit('cancel')
    }
  },
})

watch(
  () => props.prompt?.id,
  () => {
    otpCode.value = ''
    localError.value = ''
    if (props.prompt?.kind === 'saml' && props.prompt.url) {
      emit('openSaml', props.prompt.url)
    }
  },
  { immediate: true },
)

const errorText = computed(() => props.submitError || localError.value)

const dialogTitle = computed(() => {
  if (props.prompt?.kind === 'ftm_push') return 'FortiToken 移动端推送确认'
  if (props.prompt?.kind === 'saml') return 'SAML 浏览器单点登录'
  return '双因素身份验证 (OTP)'
})

const handleSubmit = () => {
  if (props.prompt?.kind !== 'otp') return
  const check = validateOtpCode(otpCode.value)
  if (!check.valid) {
    localError.value = check.error
    return
  }
  localError.value = ''
  emit('submit', check.code)
}

const handlePushContinue = () => {
  if (props.prompt?.kind !== 'ftm_push') return
  localError.value = ''
  emit('submit', otpCode.value.trim())
}
</script>

<template>
  <Dialog v-model:open="dialogOpen">
    <AppDialogContent>
      <div class="flex items-center gap-3.5">
        <div
          class="flex size-11 shrink-0 items-center justify-center rounded-xl bg-primary text-primary-foreground shadow-[0_2px_8px_color-mix(in_oklch,var(--primary)_35%,transparent)]"
        >
          <Smartphone v-if="prompt?.kind === 'ftm_push'" class="size-5" aria-hidden="true" />
          <ExternalLink v-else-if="prompt?.kind === 'saml'" class="size-5" aria-hidden="true" />
          <KeyRound v-else class="size-5" aria-hidden="true" />
        </div>
        <div>
          <AppDialogTitle>{{ dialogTitle }}</AppDialogTitle>
          <AppDialogDescription class="mt-0.5">
            {{ prompt?.message || '请完成网关要求的安全验证以继续建立隧道' }}
          </AppDialogDescription>
        </div>
      </div>

      <InlineAlert v-if="errorText" tone="danger" :message="errorText" />
      <form
        class="mt-4 space-y-4"
        @submit.prevent="prompt?.kind === 'ftm_push' ? handlePushContinue() : handleSubmit()"
      >
        <div v-if="prompt?.kind === 'saml'" class="space-y-3">
          <div
            class="rounded-xl border border-border bg-muted/40 p-3.5 text-caption text-muted-foreground"
          >
            已尝试在系统默认浏览器中打开单点登录页面。请在浏览器中完成登录，验证通过后连接会自动继续。
          </div>
          <AppButton
            type="button"
            variant="outline"
            class="w-full"
            data-testid="challenge-open-saml-btn"
            @click="prompt?.url && emit('openSaml', prompt.url)"
          >
            <ExternalLink class="size-3.5" aria-hidden="true" />
            重新打开浏览器登录页
          </AppButton>
        </div>

        <div v-else class="space-y-1.5">
          <AppLabel for="vpn-otp-code">
            {{
              prompt?.kind === 'ftm_push'
                ? '在手机上点击批准后直接确认，或在此输入令牌口令'
                : '动态验证码（OTP / Token）'
            }}
          </AppLabel>
          <AppInput
            data-testid="challenge-otp-input"
            id="vpn-otp-code"
            v-model="otpCode"
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
          <AppButton
            type="button"
            variant="outline"
            data-testid="challenge-cancel-btn"
            @click="emit('cancel')"
          >
            取消连接
          </AppButton>
          <AppButton
            v-if="prompt?.kind !== 'saml'"
            type="submit"
            data-testid="challenge-submit-btn"
            :disabled="submitting || (prompt?.kind === 'otp' && !otpCode.trim())"
          >
            <Loader2 v-if="submitting" class="size-3.5 animate-spin" aria-hidden="true" />
            {{ prompt?.kind === 'ftm_push' && !otpCode.trim() ? '已批准推送 / 继续' : '提交验证' }}
          </AppButton>
        </div>
      </form>
    </AppDialogContent>
  </Dialog>
</template>
