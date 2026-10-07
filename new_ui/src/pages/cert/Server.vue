<script setup lang="ts">
// 服务器证书：当前状态 → 主证书 / WebVPN 泛域名证书卡 → 抽屉替换
import { computed, onMounted, ref } from 'vue'
import { NButton } from 'naive-ui'
import { RouterLink } from 'vue-router'
import ArrowUpload from '~icons/fluent/arrow-upload-20-regular'
import ShieldCheck from '~icons/fluent/shield-checkmark-20-regular'
import ShieldError from '~icons/fluent/shield-error-20-regular'
import ShieldQuestion from '~icons/fluent/shield-question-20-regular'
import ChevronRight from '~icons/fluent/chevron-right-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import StatusDot from '@/components/StatusDot.vue'
import ServerCertCard from './components/ServerCertCard.vue'
import ReplaceCertDrawer from './components/ReplaceCertDrawer.vue'
import { type CertInfo, type CertRecord, type CertSlot, daysLeft, expiryText, expiryTone } from './components/pem'
import { get, errMsg } from '@/api/http'

const webvpnDomain = ref('')
const acme = ref<{ domain: string; name: string; renew: boolean } | null>(null)
const records = ref<Partial<Record<CertSlot, CertRecord & { source?: string }>>>({})
const loadError = ref('')

// 服务端实际加载的证书（/set/cert/info）→ 卡片展示结构
const SOURCE: Record<string, string> = { acme: "Let's Encrypt 自动签发", upload: '手动上传', self_signed: '系统自签名' }
function toInfo(c: any): CertInfo {
  return {
    cn: c.subject_cn || '', org: '',
    sans: [...(c.sans || []), ...(c.ip_sans || [])],
    issuerCn: c.issuer_cn || '', issuerOrg: c.issuer_org || '',
    notBefore: c.not_before ? new Date(c.not_before) : null,
    notAfter: c.not_after ? new Date(c.not_after) : null,
    selfSigned: !!c.self_signed, chainLen: c.chain_length || 1,
  }
}
async function loadCerts() {
  try {
    const d = await get('/api/v1/set/cert/info')
    const next: typeof records.value = {}
    for (const s of ['main', 'wild'] as const) {
      if (d?.[s]) next[s] = { info: toInfo(d[s]), fileName: '', at: Date.now(), source: SOURCE[d[s].source] || '' }
    }
    records.value = next
    loadError.value = d?.main_error || ''
  } catch (e) {
    loadError.value = errMsg(e)
  }
}

onMounted(async () => {
  loadCerts()
  get('/api/v1/webvpn/domain').then(d => { webvpnDomain.value = d?.domain || '' }).catch(() => {})
  get('/api/v1/set/other/getcertset').then(d => { acme.value = d?.name && d?.domain ? d : null }).catch(() => {})
})

// —— 替换 ——
const drawer = ref(false)
const drawerSlot = ref<CertSlot>('main')
function openReplace(s: CertSlot) {
  drawerSlot.value = s
  drawer.value = true
}
// 替换后证书立即生效，重新读取服务端实际证书
function onUploaded() {
  loadCerts()
}

const wildName = computed(() => (webvpnDomain.value ? `*.${webvpnDomain.value}` : ''))
const sourceText = (r?: { source?: string }) => (r?.source ? `来源：${r.source}（按颁发者推断）` : '')

// —— 顶部总状态：取已知证书中最差的一张 ——
const overall = computed(() => {
  const known = (['main', 'wild'] as const)
    .map(s => ({ s, d: daysLeft(records.value[s]?.info.notAfter) }))
    .filter(x => x.d != null) as { s: CertSlot; d: number }[]
  if (!known.length) return { tone: 'muted' as const, title: '证书状态未知', desc: loadError.value || '未能读取服务端证书信息' }
  const worst = known.reduce((a, b) => (b.d < a.d ? b : a))
  const tone = expiryTone(worst.d)
  const name = worst.s === 'main' ? '主证书' : '泛域名证书'
  if (tone === 'success') return { tone, title: '证书有效', desc: `最近到期的是${name}，${expiryText(worst.d)}。` }
  return { tone, title: worst.d < 0 ? `${name}已过期` : `${name}即将到期`, desc: `${expiryText(worst.d)}，请尽快替换或重新签发。` }
})
const overallIcon = computed(() => (overall.value.tone === 'success' ? ShieldCheck : overall.value.tone === 'muted' ? ShieldQuestion : ShieldError))
const overallClass = computed(() => ({ success: 'text-success', warning: 'text-warning', error: 'text-error', muted: 'text-secondary' })[overall.value.tone])
</script>

<template>
  <div>
    <PageHeader title="服务器证书" description="门户、管理后台、VPN 接入与 WebVPN 使用的 TLS 证书">
      <NButton type="primary" @click="openReplace('main')">
        <template #icon><ArrowUpload /></template>替换证书
      </NButton>
    </PageHeader>

    <div class="grid gap-16px">
      <!-- 当前状态 -->
      <section class="rl-panel flex flex-wrap items-center gap-x-24px gap-y-12px px-20px py-16px">
        <div class="flex min-w-0 flex-1 items-center gap-14px">
          <span class="h-40px w-40px flex flex-shrink-0 items-center justify-center rounded-10px bg-fill text-xl" :class="overallClass">
            <component :is="overallIcon" />
          </span>
          <div class="min-w-0">
            <div class="text-lg font-semibold">{{ overall.title }}</div>
            <div class="text-sm text-secondary">{{ overall.desc }}</div>
          </div>
        </div>
        <RouterLink to="/admin/cert/acme" class="flex items-center gap-8px rounded-8px px-10px py-6px text-md text-primary no-underline hover:bg-action-hover">
          <span class="text-secondary">自动签发</span>
          <StatusDot v-if="acme" :tone="acme.renew ? 'success' : 'warning'">
            Let's Encrypt · {{ acme.renew ? '自动续期已开启' : '自动续期未开启' }}
          </StatusDot>
          <StatusDot v-else tone="muted">未配置</StatusDot>
          <ChevronRight class="text-tertiary" />
        </RouterLink>
      </section>

      <!-- 两张证书 -->
      <div class="grid gap-16px lg:grid-cols-2">
        <ServerCertCard
          label="主证书" purpose="用户门户、管理后台与 VPN 接入"
          :info="records.main?.info" :source="sourceText(records.main)"
          @replace="openReplace('main')"
        />
        <ServerCertCard
          label="WebVPN 泛域名证书" purpose="WebVPN 子域访问，如 app.example.com"
          :target="wildName" :info="records.wild?.info" :source="sourceText(records.wild)"
          :disabled-reason="webvpnDomain ? '' : '尚未配置 WebVPN 域名，配置后才能部署泛域名证书'"
          @replace="openReplace('wild')"
        />
      </div>

      <!-- 说明 -->
      <RlPanel title="部署须知">
        <ul class="m-0 grid gap-8px pl-18px text-md text-secondary lg:grid-cols-2">
          <li>证书与私钥需为 PEM 格式，服务端会校验两者是否配对，替换后<strong class="text-primary">立即生效</strong>，无需重启。</li>
          <li>证书文件建议包含完整证书链，缺少中间证书时部分客户端（如移动端 AnyConnect）可能不信任。</li>
          <li>泛域名证书的 SAN 必须包含 <span class="rl-kbd">{{ wildName || '*.(WebVPN 域名)' }}</span>，否则浏览器访问 WebVPN 子域会报证书不匹配。</li>
          <li>不想手动维护？前往 <RouterLink to="/admin/cert/acme" class="text-info">自动签发</RouterLink> 用 Let's Encrypt 免费签发并自动续期。</li>
        </ul>
      </RlPanel>
    </div>

    <ReplaceCertDrawer v-model:show="drawer" :initial-slot="drawerSlot" :webvpn-domain="webvpnDomain" @uploaded="onUploaded" />
  </div>
</template>
