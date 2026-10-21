<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import {
  AlertCircle,
  Check,
  ChevronDown,
  ChevronRight,
  FileCode2,
  Loader2,
  SlidersHorizontal,
} from 'lucide-vue-next'
import {
  DEFAULT_VPN_PORT,
  formatProfileText,
  parseProfileText,
  type ConfigWarning,
  type Profile,
  type ProfileInput,
} from '@/api/profile'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import DeleteProfileButton from './DeleteProfileButton.vue'

type EditMode = 'form' | 'text'

const CONFIG_TEXT_PLACEHOLDER = `host = vpn.example.com
port = 443
username = your_account
password = your_password
realm = corp
routes = gateway
dns = split
dns-domain = corp.example.com
reconnect = 1`

const props = defineProps<{
  profile?: Profile | null
  saving?: boolean
  deleting?: boolean
  error?: string
  canCancel?: boolean
  /** 配置正在使用时禁止删除。 */
  deleteDisabled?: boolean
}>()

const emit = defineEmits<{
  save: [input: ProfileInput]
  cancel: []
  delete: [id: string]
}>()

const isEdit = computed(() => Boolean(props.profile?.id))
const hasSavedPassword = computed(() => Boolean(props.profile?.hasPassword))
const editMode = ref<EditMode>('form')
const showAdvanced = ref(false)
const justSaved = ref(false)

const form = reactive({
  name: '',
  host: '',
  port: DEFAULT_VPN_PORT,
  username: '',
  password: '',
  realm: '',
  trustedCert: '',
  auth: 'password',
  routes: 'gateway',
  extraRoutesText: '',
  dns: 'split',
  dnsDomainsText: '',
  reconnect: true,
  userAgent: '',
  legacyTls: false,
  clientCert: '',
  clientKey: '',
})

const configText = ref('')
const textWarnings = ref<ConfigWarning[]>([])
const textParseError = ref('')

const splitList = (raw: string): string[] =>
  raw
    .split(/[\n,;]+/)
    .map((s) => s.trim())
    .filter((s) => s.length > 0)

const toProfileInput = (): ProfileInput => ({
  id: props.profile?.id ?? '',
  name: form.name.trim(),
  host: form.host.trim(),
  port: Number(form.port) || DEFAULT_VPN_PORT,
  username: form.username.trim(),
  password: form.password,
  realm: form.realm.trim(),
  trustedCert: form.trustedCert.trim(),
  auth: form.auth,
  routes: form.routes,
  extraRoutes: splitList(form.extraRoutesText),
  dns: form.dns,
  dnsDomains: splitList(form.dnsDomainsText),
  reconnect: form.reconnect,
  userAgent: form.userAgent.trim(),
  legacyTls: form.legacyTls,
  clientCert: form.clientCert.trim(),
  clientKey: form.clientKey.trim(),
})

const applyInputToForm = (inp: ProfileInput) => {
  form.host = inp.host ?? ''
  form.port = inp.port || DEFAULT_VPN_PORT
  form.username = inp.username ?? ''
  form.password = inp.password ?? ''
  form.realm = inp.realm ?? ''
  form.trustedCert = inp.trustedCert ?? ''
  form.auth = inp.auth || 'password'
  form.routes = inp.routes || 'gateway'
  form.extraRoutesText = (inp.extraRoutes ?? []).join('\n')
  form.dns = inp.dns || 'split'
  form.dnsDomainsText = (inp.dnsDomains ?? []).join(', ')
  form.reconnect = Boolean(inp.reconnect)
  form.userAgent = inp.userAgent ?? ''
  form.legacyTls = Boolean(inp.legacyTls)
  form.clientCert = inp.clientCert ?? ''
  form.clientKey = inp.clientKey ?? ''
}

const hasAdvancedValues = computed(() =>
  Boolean(
    form.realm.trim() ||
    form.trustedCert.trim() ||
    form.auth === 'saml' ||
    form.userAgent.trim() ||
    form.legacyTls ||
    form.clientCert.trim() ||
    form.clientKey.trim(),
  ),
)

const syncTextFromForm = async () => {
  try {
    configText.value = await formatProfileText(toProfileInput(), hasSavedPassword.value)
    textParseError.value = ''
  } catch {
    // ignore transient formatting errors
  }
}

const syncFormFromText = async (raw: string): Promise<boolean> => {
  try {
    const res = await parseProfileText(toProfileInput(), raw)
    applyInputToForm(res.input)
    textWarnings.value = res.warnings ?? []
    textParseError.value = ''
    if (hasAdvancedValues.value) {
      showAdvanced.value = true
    }
    return true
  } catch (err) {
    textParseError.value = err instanceof Error ? err.message : String(err)
    return false
  }
}

const setEditMode = async (mode: EditMode) => {
  if (mode === editMode.value) return
  if (textDebounceTimer) {
    clearTimeout(textDebounceTimer)
    textDebounceTimer = null
  }
  if (mode === 'text') {
    await syncTextFromForm()
  } else {
    await syncFormFromText(configText.value)
  }
  editMode.value = mode
}

watch(
  () => props.profile,
  (src) => {
    form.name = src?.name ?? ''
    form.host = src?.host ?? ''
    form.port = src?.port ?? DEFAULT_VPN_PORT
    form.username = src?.username ?? ''
    form.password = ''
    form.realm = src?.realm ?? ''
    form.trustedCert = src?.trustedCert ?? ''
    form.auth = src?.auth || 'password'
    form.routes = src?.routes || 'gateway'
    form.extraRoutesText = (src?.extraRoutes ?? []).join('\n')
    form.dns = src?.dns || 'split'
    form.dnsDomainsText = (src?.dnsDomains ?? []).join(', ')
    form.reconnect = src ? Boolean(src.reconnect) : true
    form.userAgent = src?.userAgent ?? ''
    form.legacyTls = Boolean(src?.legacyTls)
    form.clientCert = src?.clientCert ?? ''
    form.clientKey = src?.clientKey ?? ''
    textWarnings.value = []
    textParseError.value = ''
    showAdvanced.value = hasAdvancedValues.value
    void syncTextFromForm()
  },
  { immediate: true },
)

let textDebounceTimer: ReturnType<typeof setTimeout> | null = null
watch(configText, (raw) => {
  if (editMode.value !== 'text') return
  if (textDebounceTimer) clearTimeout(textDebounceTimer)
  textDebounceTimer = setTimeout(() => {
    void syncFormFromText(raw)
  }, 200)
})

let savedTimer: ReturnType<typeof setTimeout> | null = null
watch(
  () => props.saving,
  (now, before) => {
    if (!before || now || props.error || !isEdit.value) return
    justSaved.value = true
    if (savedTimer) clearTimeout(savedTimer)
    savedTimer = setTimeout(() => (justSaved.value = false), 2000)
  },
)

const submit = async () => {
  if (editMode.value === 'text') {
    const ok = await syncFormFromText(configText.value)
    if (!ok) return
  }
  emit('save', toProfileInput())
}
</script>

<template>
  <form class="space-y-3.5" @submit.prevent="submit">
    <div
      v-if="error || textParseError"
      role="alert"
      class="flex items-center gap-2.5 rounded-xl border border-destructive/30 bg-destructive/[0.08] px-4 py-3 text-body text-destructive"
    >
      <AlertCircle class="size-4 shrink-0" aria-hidden="true" />
      <span>{{ error || textParseError }}</span>
    </div>

    <!-- 主配置卡片：支持「表单填写」与「配置文本」实时双向联动 -->
    <div class="surface-card space-y-4 rounded-2xl p-5">
      <div class="space-y-1.5">
        <div class="flex items-center justify-between gap-2">
          <Label for="profile-name">连接名称</Label>
          <div
            role="tablist"
            aria-label="配置录入方式"
            class="inline-flex h-8 items-center gap-0.5 rounded-lg bg-muted/90 p-0.5 text-caption text-muted-foreground ring-1 ring-black/[0.04] dark:ring-white/[0.06]"
          >
            <button
              type="button"
              role="tab"
              data-testid="mode-form"
              :aria-selected="editMode === 'form'"
              class="inline-flex h-7 items-center gap-1 rounded-md px-2.5 font-medium transition-[color,background-color,box-shadow] duration-150"
              :class="
                editMode === 'form'
                  ? 'bg-card text-foreground shadow-[var(--shadow-card)]'
                  : 'hover:text-foreground'
              "
              @click="setEditMode('form')"
            >
              <SlidersHorizontal class="size-3.5" aria-hidden="true" />
              表单填写
            </button>
            <button
              type="button"
              role="tab"
              data-testid="mode-text"
              :aria-selected="editMode === 'text'"
              class="inline-flex h-7 items-center gap-1 rounded-md px-2.5 font-medium transition-[color,background-color,box-shadow] duration-150"
              :class="
                editMode === 'text'
                  ? 'bg-card text-foreground shadow-[var(--shadow-card)]'
                  : 'hover:text-foreground'
              "
              @click="setEditMode('text')"
            >
              <FileCode2 class="size-3.5" aria-hidden="true" />
              配置文本
            </button>
          </div>
        </div>
        <Input id="profile-name" v-model="form.name" required placeholder="例如：公司办公网" />
      </div>

      <!-- 模式 1：结构化表单 -->
      <template v-if="editMode === 'form'">
        <div class="grid grid-cols-3 gap-3.5">
          <div class="col-span-2 space-y-1.5">
            <Label for="profile-host">网关地址</Label>
            <Input
              id="profile-host"
              v-model="form.host"
              required
              class="font-mono"
              placeholder="vpn.example.com"
            />
          </div>
          <div class="space-y-1.5">
            <Label for="profile-port">端口</Label>
            <Input
              id="profile-port"
              v-model.number="form.port"
              type="number"
              min="1"
              max="65535"
              required
              class="font-mono tabular-nums"
            />
          </div>
        </div>

        <div v-if="form.auth === 'password'" class="grid grid-cols-2 gap-3.5">
          <div class="space-y-1.5">
            <Label for="profile-username">用户名</Label>
            <Input
              id="profile-username"
              v-model="form.username"
              required
              autocomplete="username"
              placeholder="输入登录账号"
            />
          </div>
          <div class="space-y-1.5">
            <div class="flex items-center justify-between gap-1.5">
              <Label for="profile-password">密码</Label>
              <span
                v-if="profile?.hasPassword && !form.password"
                class="rounded bg-status-ok/14 px-1.5 py-0.5 text-[10.5px] font-medium text-status-ok"
              >
                已加密保存
              </span>
            </div>
            <Input
              id="profile-password"
              v-model="form.password"
              type="password"
              autocomplete="current-password"
              :required="!profile?.hasPassword"
              :placeholder="profile?.hasPassword ? '留空则保持原密码' : '输入登录密码'"
            />
          </div>
        </div>

        <div class="grid grid-cols-2 gap-3.5 border-t border-border/60 pt-3.5">
          <div class="space-y-1.5">
            <Label for="profile-routes">路由策略</Label>
            <select
              id="profile-routes"
              v-model="form.routes"
              class="flex h-9 w-full rounded-lg border border-input bg-card px-3 py-1 text-body text-foreground shadow-2xs transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
            >
              <option value="gateway">智能分流（推荐，仅内网流量走 VPN）</option>
              <option value="full">全局代理（所有流量均走 VPN）</option>
              <option value="none">仅建立连接（不自动修改路由）</option>
            </select>
          </div>

          <div class="space-y-1.5">
            <Label for="profile-dns">DNS 策略</Label>
            <select
              id="profile-dns"
              v-model="form.dns"
              class="flex h-9 w-full rounded-lg border border-input bg-card px-3 py-1 text-body text-foreground shadow-2xs transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
            >
              <option value="split">智能分流（推荐，仅内网域名走 VPN）</option>
              <option value="full">全局使用 VPN DNS</option>
              <option value="off">保持本机默认 DNS</option>
            </select>
          </div>
        </div>

        <div class="grid grid-cols-2 gap-3.5">
          <div class="space-y-1.5">
            <Label for="profile-extra-routes">附加内网网段（可选）</Label>
            <Input
              id="profile-extra-routes"
              v-model="form.extraRoutesText"
              class="font-mono text-caption"
              placeholder="多个用逗号分隔，如 10.10.0.0/16, 172.20.0.0/16"
            />
          </div>
          <div class="space-y-1.5">
            <Label for="profile-dns-domains">内网域名后缀（可选）</Label>
            <Input
              id="profile-dns-domains"
              v-model="form.dnsDomainsText"
              class="font-mono text-caption"
              placeholder="多个用逗号分隔，如 corp.example.com, internal.local"
            />
          </div>
        </div>

        <label class="flex cursor-pointer items-center gap-2 pt-0.5 text-body text-foreground">
          <input
            v-model="form.reconnect"
            type="checkbox"
            class="size-4 rounded border-border accent-primary"
          />
          <span>网络波动或休眠唤醒后自动重连</span>
        </label>
      </template>

      <!-- 模式 2：完整配置文本 -->
      <div v-else class="space-y-2">
        <div class="flex items-center justify-between gap-2">
          <Label for="profile-config-text">配置内容</Label>
          <span class="text-caption text-muted-foreground">
            每行一条 key = value，修改后与表单实时同步
          </span>
        </div>
        <Textarea
          id="profile-config-text"
          v-model="configText"
          :rows="9"
          class="font-mono text-caption leading-relaxed"
          :placeholder="CONFIG_TEXT_PLACEHOLDER"
        />
        <div
          v-if="textWarnings.length > 0"
          class="rounded-xl border border-status-busy/35 bg-status-busy/10 px-3.5 py-2 text-caption text-muted-foreground"
        >
          <div v-for="(w, idx) in textWarnings" :key="idx">
            第 {{ w.line }} 行（{{ w.key }}）：{{ w.message }}
          </div>
        </div>
      </div>
    </div>

    <!-- 高级选项折叠卡（仅在表单模式下展示） -->
    <div v-if="editMode === 'form'" class="surface-card overflow-hidden rounded-2xl">
      <button
        type="button"
        class="flex w-full items-center justify-between px-5 py-3.5 text-left transition-colors hover:bg-muted/50"
        :aria-expanded="showAdvanced"
        @click="showAdvanced = !showAdvanced"
      >
        <div class="flex items-center gap-2">
          <SlidersHorizontal class="size-4 text-muted-foreground" aria-hidden="true" />
          <span class="text-body font-medium text-foreground">高级选项</span>
          <span class="text-caption text-muted-foreground"> · 登录方式、证书与旧版网关兼容 </span>
          <span
            v-if="hasAdvancedValues"
            class="rounded-md bg-primary/12 px-1.5 py-0.5 text-[10.5px] font-medium text-primary"
          >
            已设置
          </span>
        </div>
        <ChevronDown v-if="showAdvanced" class="size-4 text-muted-foreground" aria-hidden="true" />
        <ChevronRight v-else class="size-4 text-muted-foreground" aria-hidden="true" />
      </button>

      <div v-if="showAdvanced" class="space-y-4 border-t border-border px-5 py-4">
        <div class="grid grid-cols-2 gap-3.5">
          <div class="space-y-1.5">
            <Label for="profile-auth">认证方式</Label>
            <select
              id="profile-auth"
              v-model="form.auth"
              class="flex h-9 w-full rounded-lg border border-input bg-card px-3 py-1 text-body text-foreground shadow-2xs transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
            >
              <option value="password">账号密码（含动态口令 / 手机推送）</option>
              <option value="saml">SAML 浏览器单点登录</option>
            </select>
          </div>
          <div class="space-y-1.5">
            <Label for="profile-realm">Realm 认证域（可选）</Label>
            <Input
              id="profile-realm"
              v-model="form.realm"
              class="font-mono text-caption"
              placeholder="默认留空，例如 corp"
            />
          </div>
        </div>

        <div class="grid grid-cols-2 gap-3.5">
          <div class="space-y-1.5">
            <Label for="profile-cert">服务器证书指纹（可选）</Label>
            <Input
              id="profile-cert"
              v-model="form.trustedCert"
              class="font-mono text-caption"
              placeholder="留空则使用系统默认证书校验（64 位 SHA-256）"
            />
          </div>
          <div class="space-y-1.5">
            <Label for="profile-ua">自定义 User-Agent（可选）</Label>
            <Input
              id="profile-ua"
              v-model="form.userAgent"
              class="font-mono text-caption"
              placeholder="默认：Mozilla/5.0 SV1"
            />
          </div>
        </div>

        <div class="grid grid-cols-2 gap-3.5">
          <div class="space-y-1.5">
            <Label for="profile-client-cert">客户端证书路径（可选）</Label>
            <Input
              id="profile-client-cert"
              v-model="form.clientCert"
              class="font-mono text-caption"
              placeholder="/path/to/client.crt"
            />
          </div>
          <div class="space-y-1.5">
            <Label for="profile-client-key">客户端私钥路径（可选）</Label>
            <Input
              id="profile-client-key"
              v-model="form.clientKey"
              class="font-mono text-caption"
              placeholder="/path/to/client.key"
            />
          </div>
        </div>

        <label class="flex cursor-pointer items-start gap-2.5 pt-0.5 text-body text-foreground">
          <input
            v-model="form.legacyTls"
            type="checkbox"
            class="mt-0.5 size-4 rounded border-border accent-primary"
          />
          <div class="space-y-0.5">
            <div class="leading-tight font-medium">兼容旧版 VPN 网关</div>
            <p class="text-caption text-muted-foreground">
              连接老旧设备提示协议或握手错误时开启（允许 TLS 1.0/1.1 与传统加密算法）
            </p>
          </div>
        </label>
      </div>
    </div>

    <!-- 底部操作栏 -->
    <div class="flex items-center justify-between pt-1">
      <DeleteProfileButton
        v-if="isEdit && profile"
        :profile-name="profile.name"
        :disabled="deleting || saving || deleteDisabled"
        :title="deleteDisabled ? '请先断开该连接' : undefined"
        @confirm="emit('delete', profile.id)"
      />
      <span v-else />

      <div class="flex items-center gap-2.5">
        <span
          v-if="justSaved"
          class="inline-flex items-center gap-1 rounded-full bg-status-ok/14 px-2.5 py-1 text-caption font-medium text-status-ok"
        >
          <Check class="size-3.5" aria-hidden="true" />
          已保存
        </span>
        <Button v-if="canCancel" type="button" variant="outline" @click="emit('cancel')">
          取消
        </Button>
        <Button type="submit" :disabled="saving || deleting">
          <Loader2 v-if="saving" class="size-3.5 animate-spin" aria-hidden="true" />
          {{ isEdit ? '保存更改' : '创建连接' }}
        </Button>
      </div>
    </div>
  </form>
</template>
