import { computed, ref } from 'vue'
import { useClipboard } from '@vueuse/core'
import { defineStore } from 'pinia'
import { vpnApi } from '@/api/vpn'
import { useNoticeStore } from '@/stores/useNoticeStore'
import { redactSensitiveText } from '@/utils/redact'

export const useLogsStore = defineStore('logs', () => {
  const noticeStore = useNoticeStore()
  const { copy } = useClipboard({ legacy: true })
  const contentByProfile = ref<Record<string, string>>({})
  const updatedAtByProfile = ref<Record<string, string>>({})
  const pending = ref(new Set<string>())
  const queued = new Map<string, boolean>()
  const revisions = new Map<string, number>()
  const loading = computed(() => pending.value.size > 0)

  const contentForProfile = (profileId: string) => contentByProfile.value[profileId] ?? ''
  const updatedAtForProfile = (profileId: string) => updatedAtByProfile.value[profileId] ?? ''

  const fetchLog = async (profileId: string, silent = false) => {
    if (!profileId) return
    if (pending.value.has(profileId)) {
      queued.set(profileId, (queued.get(profileId) ?? false) || !silent)
      return
    }
    let nextSilent = silent
    for (;;) {
      const revision = revisions.get(profileId) ?? 0
      pending.value.add(profileId)
      try {
        const snapshot = await vpnApi.getLog(profileId)
        if ((revisions.get(profileId) ?? 0) === revision) {
          contentByProfile.value[profileId] = snapshot.content
          updatedAtByProfile.value[profileId] = snapshot.updatedAt
        }
      } catch (err) {
        if (!nextSilent) noticeStore.showError(err, '读取日志失败')
      } finally {
        pending.value.delete(profileId)
      }
      const wantLoud = queued.get(profileId)
      if (wantLoud === undefined) break
      queued.delete(profileId)
      nextSilent = !wantLoud
    }
  }

  const clearLog = async (profileId: string) => {
    if (!profileId) return
    queued.delete(profileId)
    revisions.set(profileId, (revisions.get(profileId) ?? 0) + 1)
    try {
      await vpnApi.clearLog(profileId)
      // 清空期间开始的读取同样失效，避免旧快照重新填回界面。
      queued.delete(profileId)
      revisions.set(profileId, (revisions.get(profileId) ?? 0) + 1)
      contentByProfile.value[profileId] = ''
      updatedAtByProfile.value[profileId] = ''
      noticeStore.showSuccess('已清空当前连接日志')
    } catch (err) {
      noticeStore.showError(err, '清空日志失败')
    }
  }

  const copyProfileLogs = async (profileId: string): Promise<boolean> => {
    const text = redactSensitiveText(contentForProfile(profileId))
    if (!text) return false
    try {
      await copy(text)
      noticeStore.showSuccess('已复制脱敏日志到剪贴板')
      return true
    } catch (err) {
      noticeStore.showError(err, '复制日志失败')
      return false
    }
  }

  return {
    loading,
    contentForProfile,
    updatedAtForProfile,
    fetchLog,
    clearLog,
    copyProfileLogs,
  }
})
