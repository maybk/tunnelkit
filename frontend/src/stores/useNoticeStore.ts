import { ref } from 'vue'
import { defineStore } from 'pinia'
import type { Tone } from '@/constants/tone'
import { formatErrorMessage } from '@/presentation/error'
import type { NoticeItem } from '@/types/system'

const DEFAULT_DURATION_MS = 3800
const MAX_NOTICES = 4

export interface NoticeOptions {
  durationMs?: number
  replaceKey?: string
}

export const useNoticeStore = defineStore('notice', () => {
  const items = ref<NoticeItem[]>([])
  let nextId = 1
  const timers = new Map<number, ReturnType<typeof setTimeout>>()

  const clearTimer = (id: number) => {
    const timer = timers.get(id)
    if (timer) {
      clearTimeout(timer)
      timers.delete(id)
    }
  }

  const scheduleDismiss = (id: number, durationMs: number) => {
    clearTimer(id)
    if (durationMs > 0) {
      const timer = setTimeout(() => dismiss(id), durationMs)
      timers.set(id, timer)
    }
  }

  const dismiss = (id: number) => {
    clearTimer(id)
    items.value = items.value.filter((item) => item.id !== id)
  }

  const push = (
    tone: Tone,
    title: string,
    message?: string,
    durationOrOptions: number | NoticeOptions = DEFAULT_DURATION_MS,
  ): number => {
    const opts: NoticeOptions =
      typeof durationOrOptions === 'number' ? { durationMs: durationOrOptions } : durationOrOptions
    const durationMs = opts.durationMs ?? DEFAULT_DURATION_MS
    const replaceKey = opts.replaceKey

    const existing = replaceKey
      ? items.value.find((item) => item.replaceKey === replaceKey)
      : items.value.find(
          (item) => item.tone === tone && item.title === title && item.message === message,
        )

    if (existing) {
      const updated: NoticeItem = {
        id: existing.id,
        tone,
        title,
        message,
        replaceKey: replaceKey ?? existing.replaceKey,
      }
      items.value = [...items.value.filter((item) => item.id !== existing.id), updated]
      scheduleDismiss(updated.id, durationMs)
      return updated.id
    }

    const id = nextId++
    const item: NoticeItem = { id, tone, title, message, replaceKey }
    const nextItems = [...items.value, item]
    if (nextItems.length > MAX_NOTICES) {
      for (const evicted of nextItems.slice(0, nextItems.length - MAX_NOTICES)) {
        clearTimer(evicted.id)
      }
      items.value = nextItems.slice(-MAX_NOTICES)
    } else {
      items.value = nextItems
    }
    scheduleDismiss(id, durationMs)
    return id
  }

  const showSuccess = (title: string, message?: string, options?: NoticeOptions) =>
    push('success', title, message, options ?? DEFAULT_DURATION_MS)
  const showInfo = (title: string, message?: string, options?: NoticeOptions) =>
    push('info', title, message, options ?? DEFAULT_DURATION_MS)
  const showWarning = (title: string, message?: string, options?: NoticeOptions) =>
    push('warning', title, message, options ?? DEFAULT_DURATION_MS)
  const showError = (err: unknown, fallbackTitle: string = '操作失败', options?: NoticeOptions) => {
    const detail = formatErrorMessage(err, fallbackTitle)
    return push('danger', fallbackTitle, detail === fallbackTitle ? undefined : detail, {
      durationMs: 5000,
      ...options,
    })
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
