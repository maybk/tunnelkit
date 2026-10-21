<script setup lang="ts">
import { Trash2 } from 'lucide-vue-next'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'

defineProps<{
  profileName: string
  disabled?: boolean
  title?: string
}>()

const emit = defineEmits<{
  confirm: []
}>()
</script>

<template>
  <!-- 外层 span 承载 title：禁用的按钮不会触发悬停提示。 -->
  <span :title="title">
    <AlertDialog>
      <AlertDialogTrigger as-child>
        <Button type="button" variant="destructive" :disabled="disabled">
          <Trash2 class="size-3.5" />
          删除连接
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>删除「{{ profileName }}」？</AlertDialogTitle>
          <AlertDialogDescription>
            将同时删除已保存的密码和连接日志，此操作无法撤销。
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>取消</AlertDialogCancel>
          <AlertDialogAction @click="emit('confirm')">删除</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </span>
</template>
