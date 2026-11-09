<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, toRef, watch, ref } from 'vue'
import FieldGroup from '@/components/common/FieldGroup.vue'
import InlineAlert from '@/components/common/InlineAlert.vue'
import SegmentedControl from '@/components/common/SegmentedControl.vue'
import AppInput from '@/components/base/AppInput.vue'
import { FileCode2, SlidersHorizontal, ChevronDown } from '@lucide/vue'
import AppTextarea from '@/components/base/AppTextarea.vue'
import { useProfileFormDraft, type ProfileEditorMode } from '@/composables/useProfileFormDraft'
import { CONFIG_TEXT_PLACEHOLDER } from '@/constants/profile'
import type { Profile } from '@/types/profile'
import { hasOpenOverlay } from '@/utils/dom'
import ProfileAdvancedSection from './ProfileAdvancedSection.vue'
import ProfileAuthSection from './ProfileAuthSection.vue'
import ProfileBasicSection from './ProfileBasicSection.vue'
import ProfileFormActions from './ProfileFormActions.vue'
import ProfileNetworkSection from './ProfileNetworkSection.vue'

const props = withDefaults(
  defineProps<{
    profile?: Profile | null
    isCreate?: boolean
    canCancel?: boolean
  }>(),
  {
    profile: null,
    isCreate: false,
    canCancel: true,
  },
)

const emit = defineEmits<{
  saved: [profile: Profile]
  cancel: []
  'update:dirty': [dirty: boolean]
}>()

const EDITOR_MODE_OPTIONS = [
  { value: 'visual' as const, label: '表单填写' },
  { value: 'text' as const, label: '配置文本' },
]

const formElement = ref<HTMLFormElement | null>(null)

const sourceRef = toRef(props, 'profile')
const {
  draft,
  editorMode,
  configText,
  configWarnings,
  fieldErrors,
  formError,
  isDirty,
  saving,
  normalizeHostOnBlur,
  switchEditorMode,
  resetDraft,
  submitSave,
} = useProfileFormDraft(sourceRef)

const hasAdvancedValues = computed(() =>
  Boolean(
    draft.value.auth !== 'password' ||
    draft.value.realm ||
    draft.value.trustedCert ||
    draft.value.userAgent ||
    draft.value.legacyTls ||
    draft.value.clientCert ||
    draft.value.clientKey,
  ),
)

const selectedEditorMode = computed<ProfileEditorMode>({
  get: () => editorMode.value,
  set: (mode) => {
    void switchEditorMode(mode)
  },
})

const handleSubmit = async () => {
  const saved = await submitSave()
  if (saved) {
    emit('saved', saved)
  }
}

const submitDraft = () => handleSubmit()

watch(isDirty, (dirty) => emit('update:dirty', dirty), { immediate: true })

const handleSaveShortcut = (event: KeyboardEvent) => {
  if (hasOpenOverlay(document) || formElement.value?.closest('[hidden]')) return
  const isMod = event.metaKey || event.ctrlKey
  if (!isMod || event.altKey || event.shiftKey) return
  if (event.key.toLowerCase() === 's') {
    event.preventDefault()
    if ((props.isCreate || isDirty.value) && !saving.value) {
      void handleSubmit()
    }
  }
}

onMounted(() => {
  window.addEventListener('keydown', handleSaveShortcut)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleSaveShortcut)
})

defineExpose({ resetDraft, submitDraft, saving })
</script>

<template>
  <form ref="formElement" class="space-y-3.5" @submit.prevent="handleSubmit">
    <InlineAlert v-if="formError" tone="danger" :message="formError" />

    <InlineAlert
      v-for="warn in configWarnings"
      :key="`${warn.line}-${warn.key}`"
      tone="warning"
      :message="`第 ${warn.line} 行 (${warn.key}): ${warn.message}`"
    />

    <section class="surface-card space-y-4 rounded-2xl p-5">
      <FieldGroup label="连接名称" :error="fieldErrors.name">
        <template #actions>
          <SegmentedControl
            v-model="selectedEditorMode"
            :options="EDITOR_MODE_OPTIONS"
            :disabled="saving"
            aria-label="配置录入方式"
          >
            <template #option="{ option }">
              <SlidersHorizontal
                v-if="option.value === 'visual'"
                class="size-3.5"
                aria-hidden="true"
              />
              <FileCode2 v-else class="size-3.5" aria-hidden="true" />
              {{ option.label }}
            </template>
          </SegmentedControl>
        </template>
        <template #default="{ fieldId }">
          <AppInput
            :id="fieldId"
            v-model="draft.name"
            :disabled="saving"
            placeholder="例如：公司办公网"
            data-testid="profile-name-input"
          />
        </template>
      </FieldGroup>
      <template v-if="editorMode === 'visual'">
        <ProfileBasicSection
          v-model:draft="draft"
          :errors="fieldErrors"
          :disabled="saving"
          @blur-host="normalizeHostOnBlur"
        />
        <ProfileAuthSection v-model:draft="draft" :errors="fieldErrors" :disabled="saving" />
        <ProfileNetworkSection v-model:draft="draft" :errors="fieldErrors" :disabled="saving" />
      </template>
      <FieldGroup v-else label="配置内容" description="每行一条 key = value，修改后与表单实时同步">
        <template #default="{ fieldId }">
          <AppTextarea
            :id="fieldId"
            v-model="configText"
            :disabled="saving"
            :rows="9"
            :placeholder="CONFIG_TEXT_PLACEHOLDER"
            data-testid="profile-config-textarea"
            class="font-mono text-caption leading-relaxed"
          />
        </template>
      </FieldGroup>
    </section>

    <details
      v-if="editorMode === 'visual'"
      :key="profile?.id"
      class="surface-card overflow-hidden rounded-2xl"
      :open="hasAdvancedValues || Boolean(fieldErrors.trustedCert || fieldErrors.clientCert)"
    >
      <summary
        class="group flex cursor-pointer list-none items-center justify-between px-5 py-3.5 text-body font-medium text-foreground transition-colors hover:bg-muted/50 [&::-webkit-details-marker]:hidden"
      >
        <span class="flex items-center gap-2"
          ><SlidersHorizontal class="size-4 text-muted-foreground" aria-hidden="true" />高级选项
          <span class="text-caption text-muted-foreground">· 登录方式、证书与旧版网关兼容</span
          ><span
            v-if="hasAdvancedValues"
            class="rounded-md bg-primary/12 px-1.5 py-0.5 text-[10.5px] font-medium text-primary"
            >已设置</span
          ></span
        >
        <ChevronDown class="size-4 text-muted-foreground in-open:rotate-180" aria-hidden="true" />
      </summary>
      <ProfileAdvancedSection
        v-model:draft="draft"
        :errors="fieldErrors"
        :disabled="saving"
        class="border-t border-border/60 px-5 py-4"
      />
    </details>

    <ProfileFormActions
      v-if="isCreate"
      :saving="saving"
      :can-cancel="canCancel"
      @cancel="emit('cancel')"
    />
  </form>
</template>
