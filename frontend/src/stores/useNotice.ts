import { ref } from 'vue'
import { createGlobalState } from '@vueuse/core'
import { errorMessage } from '@/lib/format'

const AUTO_DISMISS_MS = 6000

/** 全局操作提示：承载不属于任何表单或连接状态的错误（如加载失败、操作被拒绝）。 */
export const useNotice = createGlobalState(() => {
  const message = ref('')
  let timer: ReturnType<typeof setTimeout> | null = null

  const dismiss = () => {
    message.value = ''
    if (timer) {
      clearTimeout(timer)
      timer = null
    }
  }

  const showError = (err: unknown) => {
    dismiss()
    message.value = errorMessage(err)
    timer = setTimeout(dismiss, AUTO_DISMISS_MS)
  }

  return { message, showError, dismiss }
})
