<script setup lang="ts">
// WebVPN 应用新建 / 编辑抽屉：先选协议，再按协议显示表单（HTTP 反代 / SSH / SFTP / SMB / FTP / RDP / VNC）
import { computed, ref, watch } from 'vue'
import {
  NButton, NDrawer, NDrawerContent, NForm, NFormItem, NInput, NInputNumber, NRadioButton, NRadioGroup, NSelect, NSwitch,
  type FormInst, type FormItemRule, type FormRules,
} from 'naive-ui'
import ArrowLeft from '~icons/fluent/arrow-left-20-regular'
import Key from '~icons/fluent/key-20-regular'
import { post, errMsg } from '@/api/http'
import { ui } from '@/utils/ui'
import {
  PROTOCOLS, accessModeOf, accessOf, appPayload, defaultPort, hasKeyAuth, isDesktopProto, isFileProto, parseCorsOrigins, parseOptions,
  pathAddr, protocolDef, protocolOf, subdomainAddr,
  type AccessMode, type CredMode, type Protocol, type WebVpnApp,
} from './app'

const props = defineProps<{ app: WebVpnApp | null; domain: string; allUsers: string[]; allGroups: string[] }>()
const show = defineModel<boolean>('show', { required: true })
const emit = defineEmits<{ saved: [] }>()

const MB = 1024 * 1024

const emptyForm = () => ({
  id: 0, name: '', note: '', backend: '', host_rewrite: '', skip_verify: false,
  users: [] as string[], groups: [] as string[], allow_path: [] as string[], ip_allow_list: [] as string[],
  allowCrossSite: false, corsOriginsText: '', statusOn: true,
  // 访问方式：未配置 WebVPN 域名时新建默认走路径
  access_mode: (props.domain ? 'subdomain' : 'path') as AccessMode,
  // 非 HTTP
  protocol: 'http' as Protocol,
  target_host: '', target_port: null as number | null,
  cred_mode: 'prompt' as CredMode, cred_user: '',
  cred_secret: '', cred_key: '', clear_secret: false, clear_key: false, has_secret: false, has_key: false,
  host_key_fp: '', root: '', optionsRest: {} as Record<string, any>,
  // 协议扩展参数（options JSON 中的已知字段）
  opt: {
    share: '', domain: '', // smb（domain 也用于 rdp）
    tls: 'none' as FtpTLS, encoding: 'utf-8' as 'utf-8' | 'gbk', skip_verify: false, // ftp
    security: 'any' as RdpSecurity, ignore_cert: false, // rdp
  },
  allow_upload: true, allow_download: true, allow_delete: false, allow_clipboard: true,
  maxFileMB: null as number | null, record: false, idleMin: null as number | null, max_sessions_per_user: null as number | null,
})
type FtpTLS = 'none' | 'explicit' | 'implicit'
type RdpSecurity = 'any' | 'nla' | 'nla-ext' | 'tls' | 'vmconnect' | 'rdp'
// options 中由表单管理的字段；其余字段原样保留
const KNOWN_OPTS = ['root', 'share', 'domain', 'tls', 'encoding', 'skip_verify', 'passive', 'security', 'ignore_cert']
// SMB 共享名非法字符
const SMB_BAD = /[\\/:*?"<>|]/
const trimSlashes = (v: string) => v.trim().replace(/^[\\/]+|[\\/]+$/g, '')

const form = ref(emptyForm())
const origFp = ref('')
const formRef = ref<FormInst | null>(null)
const saving = ref(false)
// 新建时先显示协议画廊
const picking = ref(false)
const keyFile = ref<HTMLInputElement>()

const isHttp = computed(() => form.value.protocol === 'http')
const isSSH = computed(() => form.value.protocol === 'ssh')
const isSSHish = computed(() => hasKeyAuth(form.value.protocol)) // ssh/sftp：私钥、主机指纹
const isDesktop = computed(() => isDesktopProto(form.value.protocol))
const isFile = computed(() => isFileProto(form.value.protocol))
const isVNC = computed(() => form.value.protocol === 'vnc')
const portDefault = computed(() => defaultPort(form.value.protocol, form.value.opt.tls))
const proto = computed(() => protocolDef(form.value.protocol))

const req = (msg: string, when: () => boolean): FormItemRule => ({
  trigger: 'blur',
  validator: (_r, v) => (!when() || (v != null && String(v).trim() !== '') ? true : new Error(msg)),
})
const rules = computed<FormRules>(() => ({
  name: [
    { required: true, message: '请输入应用标识', trigger: 'blur' },
    { pattern: /^[a-z0-9-]+$/, message: '仅允许小写字母、数字、短横线', trigger: 'blur' },
  ],
  note: [{ required: true, message: '请输入应用名称', trigger: 'blur' }],
  backend: [req('请输入后端地址', () => isHttp.value)],
  target_host: [req('请输入目标主机', () => !isHttp.value)],
  cred_user: [req('托管凭据需要目标账号', () => !isHttp.value && !isVNC.value && form.value.cred_mode === 'stored')],
  'opt.share': [{
    trigger: 'blur',
    validator: (_r, v: string) => {
      if (form.value.protocol !== 'smb') return true
      const t = trimSlashes(v || '')
      if (!t) return new Error('请输入共享名')
      return SMB_BAD.test(t) ? new Error('共享名不能包含 \\ / : * ? " < > |') : true
    },
  }],
  root: [{
    trigger: 'blur',
    validator: (_r, v: string) => (!v || (v.startsWith('/') && !v.split('/').includes('..')) ? true : new Error('须为以 / 开头的绝对路径，且不含 ..')),
  }],
}))

watch(show, v => {
  if (!v) return
  formRef.value?.restoreValidation()
  const r = props.app
  if (!r) {
    form.value = emptyForm()
    origFp.value = ''
    picking.value = true
    return
  }
  picking.value = false
  const opts = parseOptions(r.options)
  const root = opts.root
  const rest = Object.fromEntries(Object.entries(opts).filter(([k]) => !KNOWN_OPTS.includes(k)))
  const str = (v: unknown, d = '') => (typeof v === 'string' && v ? v : d)
  origFp.value = r.host_key_fp || ''
  form.value = {
    ...emptyForm(),
    id: r.id, name: r.name, note: r.note, backend: r.backend || '', host_rewrite: r.host_rewrite || '',
    skip_verify: !!r.skip_verify, users: [...(r.users || [])], groups: [...(r.groups || [])],
    allow_path: [...(r.allow_path || [])], ip_allow_list: [...(r.ip_allow_list || [])],
    allowCrossSite: !!r.allow_cross_site, corsOriginsText: (r.cors_allowed_origins || []).join('\n'),
    statusOn: r.status === 1,
    access_mode: accessModeOf(r),
    protocol: protocolOf(r),
    target_host: r.target_host || '', target_port: r.target_port || null,
    cred_mode: (r.cred_mode || 'prompt') as CredMode, cred_user: r.cred_user || '',
    has_secret: !!r.has_secret, has_key: !!r.has_key,
    host_key_fp: r.host_key_fp || '', root: typeof root === 'string' ? root : '', optionsRest: rest,
    opt: {
      share: str(opts.share), domain: str(opts.domain),
      tls: str(opts.tls, 'none') as FtpTLS, encoding: str(opts.encoding, 'utf-8') === 'gbk' ? 'gbk' : 'utf-8', skip_verify: !!opts.skip_verify,
      security: str(opts.security, 'any') as RdpSecurity, ignore_cert: !!opts.ignore_cert,
    },
    allow_upload: !!r.allow_upload, allow_download: !!r.allow_download, allow_delete: !!r.allow_delete,
    allow_clipboard: !!r.allow_clipboard,
    maxFileMB: r.max_file_size ? Math.round((r.max_file_size / MB) * 100) / 100 : null,
    record: !!r.record,
    idleMin: r.idle_timeout ? Math.round(r.idle_timeout / 60) : null,
    max_sessions_per_user: r.max_sessions_per_user || null,
  }
})

function pick(p: Protocol) {
  if (!protocolDef(p).ready) return
  form.value.protocol = p
  picking.value = false
}

// 默认端口随协议 / FTP TLS 模式自动填充：端口为空或仍等于上一个默认值时才覆盖（管理员改过则保留）
let lastForm: unknown = null
watch(() => [form.value, form.value.protocol, form.value.opt.tls] as const, ([f, p, t], old) => {
  if (f !== lastForm) {
    lastForm = f
    if (f.id) return // 编辑时载入不改动已保存的端口
  }
  if (p === 'http') return
  const prev = old && old[0] === f ? defaultPort(old[1], old[2]) : 0
  const next = defaultPort(p, t)
  if (f.target_port == null || f.target_port === prev) f.target_port = next || null
})

const opts = (list: string[]) => list.map(v => ({ label: v, value: v }))
// 选项 = 候选 + 已选（允许手动输入未在列表中的值）
const userOpts = computed(() => opts([...new Set([...props.allUsers, ...form.value.users])]))
const groupOpts = computed(() => opts([...new Set([...props.allGroups, ...form.value.groups])]))
const pathOpts = computed(() => opts(form.value.allow_path))
const ipOpts = computed(() => opts(form.value.ip_allow_list))
// 访问方式
const access = computed(() => accessOf({ access_mode: form.value.access_mode, protocol: form.value.protocol }, props.domain))
const accessModes = computed<{ value: AccessMode; label: string; desc: string; disabled: boolean }[]>(() => [
  { value: 'subdomain', label: '子域名', desc: '每个应用独立域名，隔离最好；需配置 WebVPN 泛域名与证书', disabled: !props.domain },
  { value: 'path', label: '路径', desc: '挂在 VPN 主域名下，无需额外域名和证书', disabled: false },
  { value: 'both', label: '两者', desc: '同时提供两种地址，门户优先使用子域名', disabled: !props.domain },
])
const nameLabel = computed(() => (access.value.sub || form.value.access_mode !== 'path' ? '子域名 / 应用标识' : '应用标识'))
const previews = computed(() => {
  const n = form.value.name || '<应用标识>'
  const out: { label: string; addr: string }[] = []
  if (access.value.sub) out.push({ label: '子域名', addr: subdomainAddr(n, props.domain) })
  if (access.value.path) out.push({ label: '路径', addr: '<VPN 主域名>' + pathAddr(n, form.value.protocol) })
  return out
})
// 未配置域名时的提示
const accessWarn = computed(() => {
  if (props.domain || form.value.access_mode === 'path') return ''
  return isHttp.value
    ? '尚未配置 WebVPN 域名，子域名方式不可用。HTTP 应用需选择「路径」才能访问。'
    : '尚未配置 WebVPN 域名，子域名方式不可用，该应用将自动以路径方式访问。'
})

const credModes = computed<{ value: CredMode; label: string; desc: string }[]>(() => [
  {
    value: 'prompt', label: '每次输入',
    desc: isSSHish.value ? '用户连接时输入账号密码或私钥，凭据不落盘' : isVNC.value ? '用户连接时输入密码（账号可选），凭据不落盘' : '用户连接时输入账号密码，凭据不落盘',
  },
  { value: 'stored', label: '托管凭据', desc: isSSHish.value ? '管理员录入账号与密码 / 私钥，加密保存，用户免输入' : '管理员录入账号与密码，加密保存，用户免输入' },
  { value: 'passthrough', label: '门户账号透传', desc: '使用用户的门户用户名和登录密码登录目标主机' },
])

const ftpTLSOpts: { value: FtpTLS; label: string }[] = [
  { value: 'none', label: '不加密' },
  { value: 'explicit', label: '显式 TLS' },
  { value: 'implicit', label: '隐式 TLS' },
]
const encodingOpts = [
  { value: 'utf-8', label: 'UTF-8' },
  { value: 'gbk', label: 'GBK（旧版 Windows / 国产 FTP 服务端）' },
]
const rdpSecOpts = [
  { value: 'any', label: '自动协商（推荐）' },
  { value: 'nla', label: 'NLA（网络级身份验证）' },
  { value: 'nla-ext', label: 'NLA 扩展（Extended NLA）' },
  { value: 'tls', label: 'TLS' },
  { value: 'vmconnect', label: 'Hyper-V 虚拟机控制台（VMConnect）' },
  { value: 'rdp', label: 'RDP 标准加密（旧版）' },
]
const rootHint = computed(() => {
  switch (form.value.protocol) {
    case 'smb': return '留空 = 共享根目录，如 /projects'
    case 'ftp': return '留空 = 登录后的初始目录，如 /pub'
  }
  return '留空 = 目标账号的家目录，如 /data'
})

const secretHint = computed(() => (form.value.has_secret && !form.value.clear_secret ? '已设置，留空不修改' : '目标账号密码'))
const keyHint = computed(() => (form.value.has_key && !form.value.clear_key ? '已设置，留空不修改；粘贴新私钥将替换' : '-----BEGIN OPENSSH PRIVATE KEY-----'))

async function onKeyFile(ev: Event) {
  const input = ev.target as HTMLInputElement
  const f = input.files?.[0]
  input.value = ''
  if (!f) return
  if (f.size > 32 << 10) { ui.message.warning('文件过大，不像是私钥'); return }
  form.value.cred_key = (await f.text()).trim()
  form.value.clear_key = false
}

function resetFp() {
  form.value.host_key_fp = ''
  ui.message.info('保存后生效：下次连接将重新记录主机指纹')
}

async function submit() {
  try { await formRef.value?.validate() } catch { return }
  const f = form.value
  // VNC 允许无密码；其余协议托管模式至少需要密码（ssh/sftp 可用私钥代替）
  if (!isHttp.value && !isVNC.value && f.cred_mode === 'stored') {
    const hasSecret = !!f.cred_secret || (f.has_secret && !f.clear_secret)
    const hasKey = isSSHish.value && (!!f.cred_key || (f.has_key && !f.clear_key))
    if (!hasSecret && !hasKey) {
      ui.message.warning(isSSHish.value ? '托管凭据需要填写密码或私钥' : '托管凭据需要填写密码')
      return
    }
  }
  // 按协议组装 options（已知字段由表单生成，未知字段原样保留）
  const options: Record<string, any> = { ...f.optionsRest }
  const o = f.opt
  if (isFile.value && f.root.trim()) options.root = f.root.trim()
  switch (f.protocol) {
    case 'smb':
      options.share = trimSlashes(o.share)
      if (o.domain.trim()) options.domain = o.domain.trim()
      break
    case 'ftp':
      options.tls = o.tls
      options.encoding = o.encoding
      options.passive = true
      if (o.tls !== 'none' && o.skip_verify) options.skip_verify = true
      break
    case 'rdp':
      options.security = o.security
      if (o.ignore_cert) options.ignore_cert = true
      if (o.domain.trim()) options.domain = o.domain.trim()
      break
  }
  saving.value = true
  try {
    await post('/api/v1/webvpn/app/set', appPayload({
      id: f.id, name: f.name, note: f.note, backend: isHttp.value ? f.backend : '', host_rewrite: f.host_rewrite, skip_verify: f.skip_verify,
      users: f.users, groups: f.groups, allow_path: isHttp.value ? f.allow_path : [], ip_allow_list: f.ip_allow_list,
      allow_cross_site: isHttp.value && f.allowCrossSite, cors_allowed_origins: isHttp.value ? parseCorsOrigins(f.corsOriginsText) : [],
      status: f.statusOn ? 1 : 0,
      access_mode: f.access_mode,
      protocol: f.protocol,
      target_host: isHttp.value ? '' : f.target_host.trim(),
      target_port: isHttp.value ? 0 : f.target_port || 0,
      cred_mode: f.cred_mode,
      cred_user: f.cred_user.trim(),
      cred_secret: f.cred_mode === 'stored' ? f.cred_secret : '',
      cred_key: f.cred_mode === 'stored' && isSSHish.value ? f.cred_key : '',
      clear_secret: f.clear_secret,
      clear_key: f.clear_key || (!isSSHish.value && f.has_key),
      host_key_fp: isSSHish.value ? f.host_key_fp.trim() : '',
      options: Object.keys(options).length ? JSON.stringify(options) : '',
      allow_upload: f.allow_upload, allow_download: f.allow_download, allow_delete: f.allow_delete, allow_clipboard: f.allow_clipboard,
      max_file_size: f.maxFileMB ? Math.round(f.maxFileMB * MB) : 0,
      record: (isSSH.value || isDesktop.value) && f.record,
      idle_timeout: f.idleMin ? f.idleMin * 60 : 0,
      max_sessions_per_user: f.max_sessions_per_user || 0,
    }))
    ui.message.success('已保存')
    show.value = false
    emit('saved')
  } catch (e) {
    ui.message.error(errMsg(e, '保存失败'))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <NDrawer v-model:show="show" :width="680" :mask-closable="false">
    <NDrawerContent :title="form.id ? '编辑 WebVPN 应用' : '添加 WebVPN 应用'" closable :native-scrollbar="false">
      <!-- 第一步：选择协议 -->
      <template v-if="picking">
        <div class="mb-16px text-md text-secondary">选择要接入的应用类型。用户经浏览器访问，无需安装客户端。</div>
        <div class="grid grid-cols-1 gap-12px sm:grid-cols-2">
          <button
            v-for="p in PROTOCOLS" :key="p.value" type="button" class="proto" :class="[!p.ready && 'proto--off', form.protocol === p.value && p.ready && 'proto--on']"
            :disabled="!p.ready" @click="pick(p.value)"
          >
            <span class="proto__icon"><component :is="p.icon" /></span>
            <span class="min-w-0 flex-1 text-left">
              <span class="flex items-center gap-6px">
                <span class="text-md font-semibold">{{ p.label }}</span>
                <span v-if="!p.ready" class="ax-badge h-18px! px-6px! text-secondary">即将支持</span>
              </span>
              <span class="block text-sm text-secondary">{{ p.desc }}</span>
            </span>
          </button>
        </div>
      </template>

      <NForm v-else ref="formRef" :model="form" :rules="rules" label-placement="top" require-mark-placement="right-hanging">
        <!-- 协议 -->
        <div class="proto proto--static mb-20px">
          <span class="proto__icon"><component :is="proto.icon" /></span>
          <span class="min-w-0 flex-1">
            <span class="block text-md font-semibold">{{ proto.label }}</span>
            <span class="block text-sm text-secondary">{{ proto.desc }}</span>
          </span>
          <NButton size="small" text @click="picking = true">
            <template #icon><ArrowLeft /></template>更换类型
          </NButton>
        </div>

        <section class="wv-sec">
          <div class="wv-sec__title">基本信息</div>
          <NFormItem :label="nameLabel" path="name">
            <NInput v-model:value="form.name" placeholder="如 oa">
              <template v-if="access.sub" #suffix><span class="rl-mono text-tertiary">.{{ domain }}</span></template>
            </NInput>
          </NFormItem>
          <div class="wv-help">唯一标识该应用，同时用作子域名前缀与路径段，仅含小写字母 / 数字 / 短横线。</div>
          <NFormItem label="应用名称" path="note">
            <NInput v-model:value="form.note" :placeholder="isHttp ? '如 OA 系统（门户页展示用）' : '如 生产跳板机（门户页展示用）'" />
          </NFormItem>
          <div class="flex items-center justify-between gap-16px">
            <div>
              <div class="text-md">启用</div>
              <div class="text-sm text-secondary">停用后该应用不可访问，也不在门户中展示；在线会话会被断开</div>
            </div>
            <NSwitch v-model:value="form.statusOn" />
          </div>
        </section>

        <section class="wv-sec">
          <div class="wv-sec__title">访问方式</div>
          <div class="mb-12px grid grid-cols-1 gap-8px sm:grid-cols-3">
            <button
              v-for="m in accessModes" :key="m.value" type="button" class="mode"
              :class="[form.access_mode === m.value && 'mode--on', m.disabled && 'mode--off']" :disabled="m.disabled"
              @click="form.access_mode = m.value"
            >
              <span class="block text-md font-semibold">{{ m.label }}</span>
              <span class="block text-sm text-secondary">{{ m.disabled ? '未配置 WebVPN 域名，不可用' : m.desc }}</span>
            </button>
          </div>
          <div v-if="previews.length" class="grid gap-4px text-sm">
            <div v-for="p in previews" :key="p.label" class="flex min-w-0 items-center gap-6px">
              <span class="ax-badge h-18px! flex-shrink-0 px-6px! text-secondary">{{ p.label }}</span>
              <span class="rl-mono truncate text-primary">{{ p.addr }}</span>
            </div>
          </div>
          <div v-if="accessWarn" class="wv-note wv-note--warn mt-12px">{{ accessWarn }}</div>
          <div v-if="isHttp && access.path" class="wv-note wv-note--warn mt-12px">
            <div class="mb-4px font-semibold">HTTP 应用使用路径方式前请确认</div>
            <ul class="m-0 pl-18px">
              <li>兼容性：网关会改写 HTML / CSS 中以 / 开头的链接、跳转与 Cookie 路径，但 JavaScript 运行时拼接的绝对路径（如 fetch('/api')）无法改写，复杂的单页应用可能部分功能异常；建议优先使用子域名，或确认应用支持部署在子路径下。</li>
              <li>安全：内网应用的页面与 VPN 门户同源。网关会剥离其作用于 / 的 Cookie、禁止覆盖 RemLink 会话 Cookie，并以 CSP 限制其只能请求本应用路径；但仍只应对可信的内网应用开启。</li>
            </ul>
          </div>
        </section>

        <!-- HTTP：后端 -->
        <section v-if="isHttp" class="wv-sec">
          <div class="wv-sec__title">后端</div>
          <NFormItem label="后端地址" path="backend">
            <NInput v-model:value="form.backend" placeholder="http://10.0.0.5:8080 或 https://..." />
          </NFormItem>
          <div class="wv-help">内网 Web 应用地址；HTTPS 且为自签证书时开启下方「跳过证书校验」。</div>
          <NFormItem label="Host 改写">
            <NInput v-model:value="form.host_rewrite" placeholder="可选，如 backend.internal（虚拟主机后端需要）" />
          </NFormItem>
          <div class="flex items-center justify-between gap-16px">
            <div>
              <div class="text-md">跳过证书校验</div>
              <div class="text-sm text-secondary">后端使用自签 / 内网证书时开启（不校验对端证书）</div>
            </div>
            <NSwitch v-model:value="form.skip_verify" />
          </div>
        </section>

        <!-- 远程协议：目标与凭据 -->
        <template v-else>
          <section class="wv-sec">
            <div class="wv-sec__title">目标主机</div>
            <div class="grid grid-cols-[1fr_140px] gap-12px">
              <NFormItem label="主机" path="target_host">
                <NInput v-model:value="form.target_host" placeholder="内网域名或 IP，如 10.0.0.12" :input-props="{ spellcheck: false }" />
              </NFormItem>
              <NFormItem label="端口">
                <NInputNumber v-model:value="form.target_port" :min="1" :max="65535" :placeholder="String(portDefault)" :show-button="false" class="w-full" />
              </NFormItem>
            </div>
            <div class="wv-help">目标地址只由管理员配置，用户无法指定其它主机。端口留空使用默认 {{ portDefault }}。</div>
          </section>

          <!-- SMB 连接参数 -->
          <section v-if="form.protocol === 'smb'" class="wv-sec">
            <div class="wv-sec__title">共享</div>
            <NFormItem label="共享名" path="opt.share" required>
              <NInput v-model:value="form.opt.share" class="rl-mono" placeholder="如 data、C$" :input-props="{ spellcheck: false }" />
            </NFormItem>
            <div class="wv-help">目标主机上的共享名称（不含主机名与路径），可在 Windows「计算机管理 → 共享文件夹」中查看。</div>
            <NFormItem label="域" :show-feedback="false">
              <NInput v-model:value="form.opt.domain" placeholder="可选，如 CORP" :input-props="{ spellcheck: false }" />
            </NFormItem>
            <div class="wv-help wv-help--tight">NTLM 认证域；留空时也可在账号中写成 CORP\alice。</div>
          </section>

          <!-- FTP 连接参数 -->
          <section v-else-if="form.protocol === 'ftp'" class="wv-sec">
            <div class="wv-sec__title">连接</div>
            <NFormItem label="加密方式" :show-feedback="false">
              <NRadioGroup v-model:value="form.opt.tls">
                <NRadioButton v-for="o in ftpTLSOpts" :key="o.value" :value="o.value" :label="o.label" />
              </NRadioGroup>
            </NFormItem>
            <div class="wv-help wv-help--tight mb-16px!">
              显式 TLS 在 21 端口上通过 AUTH TLS 升级（FTPES）；隐式 TLS 一连接即握手，默认端口 990。不加密时账号密码以明文传输。
            </div>
            <NFormItem label="文件名编码" :show-feedback="false">
              <NSelect v-model:value="form.opt.encoding" :options="encodingOpts" class="w-320px!" />
            </NFormItem>
            <div class="wv-help wv-help--tight mb-16px!">中文文件名显示为乱码时改为 GBK。</div>
            <div class="sw-list">
              <div class="sw-row">
                <div><div class="text-md">被动模式</div><div class="text-sm text-secondary">数据连接始终连到控制连接的对端地址（防 FTP 跳转攻击）；暂不支持主动模式</div></div>
                <NSwitch :value="true" disabled />
              </div>
              <div class="sw-row">
                <div><div class="text-md">跳过证书校验</div><div class="text-sm text-secondary">FTPS 服务器使用自签证书时开启（不校验对端证书）</div></div>
                <NSwitch v-model:value="form.opt.skip_verify" :disabled="form.opt.tls === 'none'" />
              </div>
            </div>
          </section>

          <!-- RDP 连接参数 -->
          <section v-else-if="form.protocol === 'rdp'" class="wv-sec">
            <div class="wv-sec__title">连接</div>
            <NFormItem label="安全模式" :show-feedback="false">
              <NSelect v-model:value="form.opt.security" :options="rdpSecOpts" class="w-320px!" />
            </NFormItem>
            <div class="wv-help wv-help--tight mb-16px!">连接失败时可先试「自动协商」并开启「忽略证书」；目标要求 NLA 时必须提供正确的账号密码。</div>
            <NFormItem label="域" :show-feedback="false">
              <NInput v-model:value="form.opt.domain" placeholder="可选，如 CORP" :input-props="{ spellcheck: false }" />
            </NFormItem>
            <div class="wv-help wv-help--tight mb-16px!">Windows 域；留空时也可在账号中写成 CORP\alice。</div>
            <div class="sw-list">
              <div class="sw-row">
                <div><div class="text-md">忽略证书</div><div class="text-sm text-secondary">目标主机使用自签证书时开启（大多数 Windows 主机需要）</div></div>
                <NSwitch v-model:value="form.opt.ignore_cert" />
              </div>
            </div>
          </section>

          <section class="wv-sec">
            <div class="wv-sec__title">登录凭据</div>
            <div class="mb-16px grid grid-cols-1 gap-8px sm:grid-cols-3">
              <button
                v-for="m in credModes" :key="m.value" type="button" class="mode" :class="form.cred_mode === m.value && 'mode--on'"
                @click="form.cred_mode = m.value"
              >
                <span class="block text-md font-semibold">{{ m.label }}</span>
                <span class="block text-sm text-secondary">{{ m.desc }}</span>
              </button>
            </div>

            <template v-if="form.cred_mode === 'prompt'">
              <NFormItem label="固定账号">
                <NInput v-model:value="form.cred_user" :placeholder="isVNC ? '可选；VNC 多为仅密码认证，一般留空' : '可选；留空则由用户输入账号'" />
              </NFormItem>
            </template>

            <template v-else-if="form.cred_mode === 'stored'">
              <NFormItem :label="isVNC ? '目标账号（可选）' : '目标账号'" path="cred_user">
                <NInput v-model:value="form.cred_user" :placeholder="isVNC ? '仅密码认证时留空' : isSSHish ? '如 deploy' : '如 administrator 或 CORP\\alice'" />
              </NFormItem>
              <NFormItem label="密码" :show-feedback="false">
                <div class="w-full">
                  <NInput
                    v-model:value="form.cred_secret" type="password" show-password-on="click" :placeholder="secretHint"
                    :disabled="form.clear_secret" :input-props="{ autocomplete: 'new-password' }"
                  />
                  <div v-if="form.has_secret" class="mt-6px text-sm">
                    <template v-if="form.clear_secret">
                      <span class="text-warning">保存后将清除已保存的密码</span>
                      <NButton text size="tiny" class="ml-8px" @click="form.clear_secret = false">撤销</NButton>
                    </template>
                    <NButton v-else text size="tiny" type="error" @click="form.clear_secret = true; form.cred_secret = ''">清除已保存的密码</NButton>
                  </div>
                </div>
              </NFormItem>
              <div v-if="isSSHish" class="wv-help wv-help--tight mb-16px!">私钥带口令时，在此填写私钥口令。</div>
              <div v-else-if="isVNC" class="wv-help wv-help--tight mb-16px!">VNC 未设置密码时可留空。</div>
              <NFormItem v-if="isSSHish" label="私钥" :show-feedback="false">
                <div class="w-full">
                  <NInput
                    v-model:value="form.cred_key" type="textarea" :rows="4" class="rl-mono" :placeholder="keyHint"
                    :disabled="form.clear_key" :input-props="{ spellcheck: false }"
                  />
                  <div class="mt-6px flex items-center gap-12px text-sm">
                    <NButton text size="tiny" :disabled="form.clear_key" @click="keyFile?.click()">
                      <template #icon><Key /></template>从文件读取
                    </NButton>
                    <template v-if="form.has_key">
                      <template v-if="form.clear_key">
                        <span class="text-warning">保存后将清除已保存的私钥</span>
                        <NButton text size="tiny" @click="form.clear_key = false">撤销</NButton>
                      </template>
                      <NButton v-else text size="tiny" type="error" @click="form.clear_key = true; form.cred_key = ''">清除已保存的私钥</NButton>
                    </template>
                  </div>
                  <input ref="keyFile" type="file" hidden @change="onKeyFile">
                </div>
              </NFormItem>
              <div class="wv-help wv-help--tight">凭据加密保存，任何接口都不会回显{{ isSSHish ? '；密码和私钥至少填写一项' : '' }}。</div>
            </template>

            <div v-else class="text-sm text-secondary">
              用户以门户用户名登录目标主机，密码取自其门户登录时的缓存（仅保存在内存中）；缓存失效时用户需在连接时输入。适合目标主机与门户对接同一 LDAP / AD 的场景。
            </div>
          </section>

          <section v-if="isSSHish" class="wv-sec">
            <div class="wv-sec__title">主机指纹</div>
            <NFormItem label="SSH 主机指纹" :show-feedback="false">
              <NInput v-model:value="form.host_key_fp" class="rl-mono" placeholder="留空 = 首次连接时自动记录" :input-props="{ spellcheck: false }" />
            </NFormItem>
            <div class="wv-help wv-help--tight flex items-start gap-8px">
              <span class="flex-1">首次连接自动记录指纹（TOFU），之后指纹不一致将拒绝连接以防中间人攻击。可填写多个，逗号分隔。修改目标主机或端口后会自动重新记录。</span>
              <NButton v-if="origFp" size="tiny" secondary :disabled="!form.host_key_fp" @click="resetFp">重置指纹</NButton>
            </div>
          </section>

          <section v-if="isFile" class="wv-sec">
            <div class="wv-sec__title">文件</div>
            <NFormItem label="根目录" path="root">
              <NInput v-model:value="form.root" class="rl-mono" :placeholder="rootHint" :input-props="{ spellcheck: false }" />
            </NFormItem>
            <div class="wv-help">
              用户只能浏览此目录及其子目录{{ form.protocol === 'smb' ? '（符号链接与目录联接一律不跟随）' : form.protocol === 'ftp' ? '' : '（含符号链接校验）' }}。
            </div>
            <div class="sw-list">
              <div class="sw-row">
                <div><div class="text-md">允许上传</div><div class="text-sm text-secondary">上传文件 / 文件夹、新建目录、重命名</div></div>
                <NSwitch v-model:value="form.allow_upload" />
              </div>
              <div class="sw-row">
                <div><div class="text-md">允许下载</div><div class="text-sm text-secondary">下载文件，目录打包为 zip</div></div>
                <NSwitch v-model:value="form.allow_download" />
              </div>
              <div class="sw-row">
                <div><div class="text-md">允许删除</div><div class="text-sm text-secondary">删除文件与目录（目录递归删除）</div></div>
                <NSwitch v-model:value="form.allow_delete" />
              </div>
            </div>
            <NFormItem label="单文件大小上限" class="mt-16px" :show-feedback="false">
              <NInputNumber v-model:value="form.maxFileMB" :min="0" :precision="0" placeholder="不限" clearable class="w-220px!">
                <template #suffix>MB</template>
              </NInputNumber>
            </NFormItem>
            <div class="wv-help wv-help--tight">同时限制上传、下载和目录打包的总大小；留空或 0 表示不限。</div>
          </section>

          <section v-if="isSSH || isDesktop" class="wv-sec">
            <div class="wv-sec__title">{{ isDesktop ? '远程桌面' : '终端' }}</div>
            <div v-if="isDesktop" class="wv-help mt-0!">
              远程桌面经 guacd 代理接入，需先部署 guacd 并在系统设置中配置其地址；文件传输（驱动器重定向）暂不开放。
            </div>
            <div class="sw-list">
              <div class="sw-row">
                <div>
                  <div class="text-md">允许剪贴板</div>
                  <div class="text-sm text-secondary">{{ isDesktop ? '关闭后浏览器与远程桌面之间不同步剪贴板（双向拦截）' : '关闭后终端内禁止复制和粘贴' }}</div>
                </div>
                <NSwitch v-model:value="form.allow_clipboard" />
              </div>
              <div class="sw-row">
                <div>
                  <div class="text-md">会话录像</div>
                  <div class="text-sm text-secondary">
                    {{ isDesktop ? '录制屏幕画面（Guacamole 录像，不含键盘输入），可在「远程会话」回放或下载' : '记录终端输出（asciicast，不含键盘输入），可在「远程会话」回放' }}；用户会看到录像提示
                  </div>
                </div>
                <NSwitch v-model:value="form.record" />
              </div>
            </div>
          </section>

          <section class="wv-sec">
            <div class="wv-sec__title">会话</div>
            <div class="grid grid-cols-2 gap-12px">
              <NFormItem label="空闲断开" :show-feedback="false">
                <NInputNumber v-model:value="form.idleMin" :min="0" :precision="0" placeholder="默认 30" clearable class="w-full">
                  <template #suffix>分钟</template>
                </NInputNumber>
              </NFormItem>
              <NFormItem label="单用户并发会话" :show-feedback="false">
                <NInputNumber v-model:value="form.max_sessions_per_user" :min="0" :precision="0" placeholder="默认 5" clearable class="w-full" />
              </NFormItem>
            </div>
          </section>
        </template>

        <section class="wv-sec">
          <div class="wv-sec__title">访问授权</div>
          <div class="mb-16px -mt-4px text-sm text-secondary">授权用户与用户组都留空时，所有登录用户均可访问。</div>
          <NFormItem label="授权用户">
            <NSelect v-model:value="form.users" multiple filterable tag :options="userOpts" placeholder="留空 = 不限制到具体用户" />
          </NFormItem>
          <NFormItem label="授权用户组">
            <NSelect v-model:value="form.groups" multiple filterable tag :options="groupOpts" placeholder="留空 = 允许全部登录用户" />
          </NFormItem>
          <template v-if="isHttp">
            <NFormItem label="允许路径" :show-feedback="false">
              <NSelect v-model:value="form.allow_path" multiple filterable tag :show-arrow="false" :options="pathOpts" placeholder="留空 = 全部路径，输入后回车添加" />
            </NFormItem>
            <div class="wv-help wv-help--tight">路径前缀白名单，如 /api/，仅允许访问匹配前缀。</div>
          </template>
          <NFormItem label="来源 IP" :show-feedback="false">
            <NSelect v-model:value="form.ip_allow_list" multiple filterable tag :show-arrow="false" :options="ipOpts" placeholder="留空 = 不限制来源，输入后回车添加" />
          </NFormItem>
          <div class="wv-help wv-help--tight">客户端公网 IP / CIDR 白名单，限制可访问的出口。</div>
        </section>

        <section v-if="isHttp" class="wv-sec">
          <div class="wv-sec__title">跨站访问</div>
          <div class="mb-16px flex items-center justify-between gap-16px">
            <div>
              <div class="text-md">允许跨站访问</div>
              <div class="text-sm text-secondary">仅在需要跨域页面、接口或 iframe 访问时启用；关闭时同源访问不受影响。</div>
            </div>
            <NSwitch v-model:value="form.allowCrossSite" />
          </div>
          <NFormItem label="允许跨域来源" :show-feedback="false">
            <NInput v-model:value="form.corsOriginsText" type="textarea" :rows="3" placeholder="每行一个完整地址，如 https://oa.mv.example.com" />
          </NFormItem>
          <div class="wv-help wv-help--tight">跨站访问开启后，仅允许精确匹配的 http/https Origin；留空或关闭开关 = 仅同源请求。</div>
        </section>
      </NForm>

      <template #footer>
        <div class="flex justify-end gap-8px">
          <NButton @click="show = false">取消</NButton>
          <NButton v-if="!picking" type="primary" :loading="saving" @click="submit">保存</NButton>
        </div>
      </template>
    </NDrawerContent>
  </NDrawer>
</template>

<style scoped>
.wv-sec + .wv-sec { margin-top: 8px; padding-top: 20px; border-top: 1px solid var(--color-border-default); }
.wv-help { margin: -6px 0 16px; font-size: 12px; color: var(--color-text-secondary); }
.wv-help--tight { margin-top: 6px; }
.wv-sec__title { margin-bottom: 12px; font-size: 16px; font-weight: 600; color: var(--color-text-primary); }
.proto {
  display: flex; align-items: center; gap: 12px; padding: 12px 14px; border: 1px solid var(--color-border-default); border-radius: 12px;
  background: var(--color-primary); color: var(--color-text-primary); font: inherit; cursor: pointer; transition: border-color .15s;
}
.proto:hover:not(:disabled) { border-color: var(--color-border-strong); }
.proto--on { border-color: var(--color-action) !important; }
.proto--off { cursor: not-allowed; opacity: .55; }
.proto--static { cursor: default; }
.proto--static:hover { border-color: var(--color-border-default); }
.proto__icon { display: flex; align-items: center; justify-content: center; width: 36px; height: 36px; flex-shrink: 0; border-radius: 10px; background: var(--color-fill); font-size: 18px; color: var(--color-text-secondary); }
.mode {
  padding: 10px 12px; border: 1px solid var(--color-border-default); border-radius: 10px; background: var(--color-primary);
  color: var(--color-text-primary); font: inherit; text-align: left; cursor: pointer; transition: border-color .15s;
}
.mode:hover { border-color: var(--color-border-strong); }
.mode--on { border-color: var(--color-action); box-shadow: inset 0 0 0 1px var(--color-action); }
.mode--off { cursor: not-allowed; opacity: .55; }
.mode--off:hover { border-color: var(--color-border-default); }
.wv-note { padding: 10px 12px; border-radius: 10px; font-size: 12px; line-height: 1.6; }
.wv-note--warn { background: color-mix(in srgb, var(--color-text-warning) 10%, transparent); color: var(--color-text-primary); border: 1px solid color-mix(in srgb, var(--color-text-warning) 35%, transparent); }
.sw-list { border: 1px solid var(--color-border-default); border-radius: 10px; }
.sw-row { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 12px 14px; }
.sw-row + .sw-row { border-top: 1px solid var(--color-border-default); }
</style>
