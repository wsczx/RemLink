<script setup lang="ts">
// 替换服务器证书：选择槽位 → 拖入证书与私钥 → 本地解析预览 → 上传（服务端校验配对后立即生效）
import { computed, ref, watch } from 'vue'
import { NAlert, NButton, NDrawer, NDrawerContent } from 'naive-ui'
import DocumentAdd from '~icons/fluent/document-add-20-regular'
import Key from '~icons/fluent/key-20-regular'
import Dismiss from '~icons/fluent/dismiss-20-regular'
import Checkmark from '~icons/fluent/checkmark-circle-20-filled'
import StatusDot from '@/components/StatusDot.vue'
import { type CertInfo, type CertSlot, certCovers, daysLeft, expiryText, expiryTone, parsePemCert } from './pem'
import { errMsg, post } from '@/api/http'
import { ui } from '@/utils/ui'
import { formatDate } from '@/utils/format'

const props = defineProps<{ webvpnDomain: string; initialSlot: CertSlot }>()
const show = defineModel<boolean>('show', { required: true })
const emit = defineEmits<{ uploaded: [slot: CertSlot, info: CertInfo | null, fileName: string] }>()

const slot = ref<CertSlot>('main')
const certFile = ref<File | null>(null)
const keyFile = ref<File | null>(null)
const info = ref<CertInfo | null>(null)
const parseErr = ref('')
const keyErr = ref('')
const uploading = ref(false)
const dragOver = ref<'' | 'cert' | 'key'>('')
const certInput = ref<HTMLInputElement>()
const keyInput = ref<HTMLInputElement>()

watch(show, v => {
  if (!v) return
  slot.value = props.initialSlot
  certFile.value = keyFile.value = info.value = null
  parseErr.value = keyErr.value = ''
})

const wildName = computed(() => (props.webvpnDomain ? `*.${props.webvpnDomain}` : ''))
// 泛域名槽位：证书必须包含 *.<WebVPN 域名>
const wildMismatch = computed(() => slot.value === 'wild' && !!info.value && !!wildName.value && !certCovers(info.value, wildName.value))
const days = computed(() => daysLeft(info.value?.notAfter))
const canSubmit = computed(() => !!certFile.value && !!keyFile.value && !(slot.value === 'wild' && !props.webvpnDomain))

const slots = computed(() => [
  { value: 'main' as const, title: '主证书', desc: '用户门户、管理后台与 VPN 接入共用' },
  { value: 'wild' as const, title: 'WebVPN 泛域名证书', desc: wildName.value ? `用于 WebVPN 子域，需包含 ${wildName.value}` : '尚未配置 WebVPN 域名', disabled: !props.webvpnDomain },
])

async function setCert(f: File) {
  certFile.value = f
  info.value = null
  parseErr.value = ''
  try {
    info.value = parsePemCert(await f.text())
  } catch (err: any) {
    parseErr.value = `无法解析证书：${err?.message || err}。仅支持 PEM 格式。`
  }
}
async function setKey(f: File) {
  keyFile.value = f
  keyErr.value = /-----BEGIN [A-Z ]*PRIVATE KEY-----/.test(await f.text()) ? '' : '文件中未找到 PEM 私钥块，请确认选择的是私钥文件'
}
function onPick(e: Event, kind: 'cert' | 'key') {
  const el = e.target as HTMLInputElement
  const f = el.files?.[0]
  el.value = ''
  if (f) kind === 'cert' ? setCert(f) : setKey(f)
}
function onDrop(e: DragEvent, kind: 'cert' | 'key') {
  dragOver.value = ''
  const f = e.dataTransfer?.files?.[0]
  if (f) kind === 'cert' ? setCert(f) : setKey(f)
}
function clearCert() {
  certFile.value = info.value = null
  parseErr.value = ''
}

async function submit() {
  if (!certFile.value || !keyFile.value) return
  const fd = new FormData()
  fd.append('slot', slot.value)
  fd.append('cert', certFile.value)
  fd.append('key', keyFile.value)
  uploading.value = true
  try {
    await post('/api/v1/set/other/customcert', fd)
    ui.message.success('证书已替换，立即生效')
    emit('uploaded', slot.value, info.value, certFile.value.name)
    show.value = false
  } catch (e) {
    ui.message.error(errMsg(e, '上传失败'))
  } finally {
    uploading.value = false
  }
}
</script>

<template>
  <NDrawer v-model:show="show" :width="620">
    <NDrawerContent title="替换服务器证书" closable>
      <div class="flex flex-col gap-24px">
        <!-- 1. 槽位 -->
        <section>
          <div class="mb-8px text-md font-semibold">替换哪张证书</div>
          <div class="grid gap-8px sm:grid-cols-2">
            <button
              v-for="s in slots" :key="s.value" type="button" :disabled="s.disabled"
              class="cert-opt rounded-10px bg-primary p-12px text-left" :class="slot === s.value && 'cert-opt--on'"
              @click="slot = s.value"
            >
              <div class="flex items-center justify-between gap-6px">
                <span class="text-md font-semibold">{{ s.title }}</span>
                <Checkmark v-if="slot === s.value" class="text-lg text-primary" />
              </div>
              <div class="mt-2px text-sm text-secondary">{{ s.desc }}</div>
            </button>
          </div>
        </section>

        <!-- 2. 文件 -->
        <section>
          <div class="mb-8px text-md font-semibold">证书与私钥</div>
          <div class="grid gap-8px sm:grid-cols-2">
            <div
              v-for="k in (['cert', 'key'] as const)" :key="k"
              class="cert-drop flex flex-col items-center justify-center gap-4px rounded-10px p-16px text-center"
              :class="[dragOver === k && 'cert-drop--over', (k === 'cert' ? certFile : keyFile) && 'cert-drop--filled']"
              @click="(k === 'cert' ? certInput : keyInput)?.click()"
              @dragover.prevent="dragOver = k" @dragleave="dragOver = ''" @drop.prevent="onDrop($event, k)"
            >
              <component :is="k === 'cert' ? DocumentAdd : Key" class="text-xl text-secondary" />
              <template v-if="k === 'cert' ? certFile : keyFile">
                <div class="flex max-w-full items-center gap-4px">
                  <span class="rl-mono truncate">{{ (k === 'cert' ? certFile : keyFile)!.name }}</span>
                  <button class="rl-icon-btn h-24px! w-24px! flex-shrink-0 text-md!" title="移除" @click.stop="k === 'cert' ? clearCert() : (keyFile = null, keyErr = '')"><Dismiss /></button>
                </div>
              </template>
              <template v-else>
                <div class="text-md font-medium">{{ k === 'cert' ? '证书文件' : '私钥文件' }}</div>
                <div class="text-sm text-tertiary">拖入或点击选择 · {{ k === 'cert' ? '.pem .crt .cer' : '.pem .key' }}</div>
              </template>
            </div>
          </div>
          <input ref="certInput" type="file" accept=".pem,.crt,.cer" class="hidden" @change="onPick($event, 'cert')">
          <input ref="keyInput" type="file" accept=".pem,.key" class="hidden" @change="onPick($event, 'key')">
          <div class="mt-6px text-sm text-secondary">PEM 格式；证书文件建议包含完整证书链（叶子证书在前）。</div>
          <div v-if="keyErr" class="mt-4px text-sm text-warning">{{ keyErr }}</div>
        </section>

        <!-- 3. 预览 -->
        <section>
          <div class="mb-8px flex items-center justify-between">
            <span class="text-md font-semibold">本地解析预览</span>
            <span class="text-sm text-tertiary">仅在浏览器中解析，最终以服务端校验为准</span>
          </div>
          <NAlert v-if="parseErr" type="error" :bordered="false">{{ parseErr }}</NAlert>
          <div v-else-if="info" class="rounded-10px b-1px b-[var(--color-border-default)] b-solid">
            <div class="flex items-center justify-between gap-8px px-14px py-12px">
              <span class="rl-mono truncate text-lg font-semibold">{{ info.cn || info.sans[0] || '（无通用名）' }}</span>
              <StatusDot :tone="expiryTone(days)" class="flex-shrink-0 text-sm">{{ expiryText(days) }}</StatusDot>
            </div>
            <dl class="m-0 grid grid-cols-[64px_minmax(0,1fr)] gap-x-12px gap-y-8px b-t-1px b-t-solid b-[var(--color-border-default)] px-14px py-12px text-md">
              <dt class="text-secondary">覆盖域名</dt>
              <dd class="m-0 flex flex-wrap gap-4px">
                <span v-if="!info.sans.length" class="text-tertiary">未包含 SAN</span>
                <span v-for="s in info.sans" :key="s" class="rl-kbd">{{ s }}</span>
              </dd>
              <dt class="text-secondary">颁发者</dt>
              <dd class="m-0 break-words">
                {{ [info.issuerCn, info.issuerOrg].filter(Boolean).join(' · ') || '—' }}
                <span v-if="info.selfSigned" class="ax-badge ml-4px text-warning">自签名</span>
              </dd>
              <dt class="text-secondary">有效期</dt>
              <dd class="m-0">{{ formatDate(info.notBefore, 'YYYY-MM-DD') || '—' }} → {{ formatDate(info.notAfter, 'YYYY-MM-DD') || '—' }}</dd>
              <dt class="text-secondary">证书链</dt>
              <dd class="m-0">{{ info.chainLen }} 张</dd>
            </dl>
          </div>
          <div v-else class="rounded-10px bg-fill px-14px py-20px text-center text-sm text-tertiary">选择证书文件后，这里会显示域名、颁发者和到期时间</div>
          <NAlert v-if="wildMismatch" type="warning" :bordered="false" class="mt-8px">证书未包含 {{ wildName }}，作为泛域名证书使用时浏览器会报证书不匹配。</NAlert>
          <NAlert v-if="info && days != null && days < 0" type="error" :bordered="false" class="mt-8px">该证书已过期，上传后客户端将无法建立安全连接。</NAlert>
        </section>
      </div>

      <template #footer>
        <div class="flex w-full items-center justify-between gap-sm">
          <span class="text-sm text-secondary">服务端会校验证书与私钥是否匹配，替换后立即生效</span>
          <div class="flex gap-sm">
            <NButton @click="show = false">取消</NButton>
            <NButton type="primary" :loading="uploading" :disabled="!canSubmit" @click="submit">上传并替换</NButton>
          </div>
        </div>
      </template>
    </NDrawerContent>
  </NDrawer>
</template>

<style scoped>
.cert-opt { border: 1px solid var(--color-border-default); cursor: pointer; font: inherit; color: inherit; transition: border-color .15s; }
.cert-opt:hover:not(:disabled) { border-color: var(--color-border-strong); }
.cert-opt--on { border-color: var(--color-text-primary); box-shadow: inset 0 0 0 1px var(--color-text-primary); }
.cert-opt:disabled { cursor: not-allowed; opacity: .5; }
.cert-drop { min-height: 112px; border: 1px dashed var(--color-border-strong); cursor: pointer; transition: border-color .15s, background-color .15s; }
.cert-drop:hover, .cert-drop--over { border-color: var(--color-text-primary); background: var(--color-secondary); }
.cert-drop--filled { border-style: solid; border-color: var(--color-border-default); background: var(--color-secondary); }
</style>
