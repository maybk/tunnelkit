import { ref } from 'vue'
import { defineStore } from 'pinia'
import type { Tone } from '@/constants/tone'
import { formatErrorMessage } from '@/presentation/error'
import type { NoticeItem } from '@/types/system'

const DEFAULT_DURATION_MS = 3800

export const useNoticeStore = defineStore('notice', () => {
  const items = ref<NoticeItem[]>([])
  let nextId = 1
  const timers = new Map<number, ReturnType<typeof setTimeout>>()

  const dismiss = (id: number) => {
    const timer = timers.get(id)
    if (timer) {
      clearTimeout(timer)
      timers.delete(id)
    }
    items.value = items.value.filter((item) => item.id !== id)
  }

  const push = (
    tone: Tone,
    title: string,
    message?: string,
    durationMs: number = DEFAULT_DURATION_MS,
  ): number => {
    const id = nextId++
    const item: NoticeItem = { id, tone, title, message }
    items.value = [...items.value.slice(-3), item]
    if (durationMs > 0) {
      const timer = setTimeout(() => dismiss(id), durationMs)
      timers.set(id, timer)
    }
    return id
  }

  const showSuccess = (title: string, message?: string) => push('success', title, message)
  const showInfo = (title: string, message?: string) => push('info', title, message)
  const showWarning = (title: string, message?: string) => push('warning', title, message)
  const showError = (err: unknown, fallbackTitle: string = '操作失败') => {
    const detail = formatErrorMessage(err, fallbackTitle)
    return push('danger', fallbackTitle, detail === fallbackTitle ? undefined : detail, 5000)
  }

  const clearAll = () => {
    for (const timer of timers.values()) {
      clearTimeout(timer)
    }
    timers.clear()
    items.value = []
  }

  return {
    items,
    push,
    dismiss,
    clearAll,
    showSuccess,
    showInfo,
    showWarning,
    showError,
  }
})
