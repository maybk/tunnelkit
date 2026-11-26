import { getActivePinia } from 'pinia'
import { onScopeDispose, ref } from 'vue'
import { useNoticeStore } from '@/stores/useNoticeStore'

/** 轻量剪贴板复制与短暂「已复制」状态反馈，失败时主动提示，组件卸载时自动清理定时器。 */
export function useCopyFeedback(resetDelayMs = 1600) {
  const copiedKey = ref<string | null>(null)
  let timer: ReturnType<typeof setTimeout> | undefined
  let disposed = false

  const clearTimer = () => {
    if (timer !== undefined) {
      clearTimeout(timer)
      timer = undefined
    }
  }

  const reportFailure = () => {
    if (!disposed && getActivePinia()) {
      useNoticeStore().showWarning('复制失败，请手动复制')
    }
  }

  const reset = () => {
    clearTimer()
    copiedKey.value = null
  }

  onScopeDispose(() => {
    disposed = true
    clearTimer()
  })

  const copy = async (text: string, key = 'default'): Promise<boolean> => {
    if (!text) return false
    if (typeof navigator === 'undefined' || !navigator.clipboard?.writeText) {
      reportFailure()
      return false
    }
    try {
      await navigator.clipboard.writeText(text)
      if (disposed) return false
      copiedKey.value = key
      clearTimer()
      timer = setTimeout(() => {
        if (copiedKey.value === key) copiedKey.value = null
        timer = undefined
      }, resetDelayMs)
      return true
    } catch {
      reportFailure()
      return false
    }
  }

  return { copiedKey, copy, reset }
}
