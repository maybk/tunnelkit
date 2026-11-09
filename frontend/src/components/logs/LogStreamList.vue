<script setup lang="ts">
import { nextTick, useTemplateRef, watch } from 'vue'
import { Terminal } from '@lucide/vue'

const props = withDefaults(
  defineProps<{
    content: string
    autoScroll?: boolean
  }>(),
  {
    autoScroll: true,
  },
)

const containerRef = useTemplateRef<HTMLDivElement>('container')

const scrollToBottom = () => {
  const el = containerRef.value
  if (!el) return
  el.scrollTop = el.scrollHeight
}

watch(
  () => [props.content, props.autoScroll] as const,
  async ([, auto]) => {
    if (!auto) return
    await nextTick()
    scrollToBottom()
  },
  { immediate: true },
)
</script>

<template>
  <div
    ref="container"
    data-testid="log-stream-container"
    class="flex min-h-0 flex-1 overflow-auto bg-[#0C101A] p-4 font-mono text-caption leading-relaxed text-slate-200"
  >
    <pre v-if="content" class="m-0 min-w-full whitespace-pre-wrap break-all select-text">{{
      content
    }}</pre>
    <div v-else class="m-auto flex flex-col items-center gap-2 text-center text-slate-400">
      <Terminal class="size-6 opacity-50" />
      <p>暂无日志，连接后会在此显示运行记录。</p>
    </div>
  </div>
</template>
