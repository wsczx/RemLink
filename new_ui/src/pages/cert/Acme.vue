<script setup lang="ts">
// 自动签发（Let's Encrypt）：状态卡 → 已配置时摘要 + 签发目标；未配置 / 编辑时进入向导
import { computed, onMounted, ref } from 'vue'
import { NButton, NSpin } from 'naive-ui'
import Edit from '~icons/fluent/edit-20-regular'
import ArrowSync from '~icons/fluent/arrow-sync-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import StatusDot from '@/components/StatusDot.vue'
import AcmeWizard from './components/AcmeWizard.vue'
import { type LetsForm, blankLets, fromServer, providerOf } from './components/acme'
import { errMsg, get, post } from '@/api/http'
import { confirm, ui } from '@/utils/ui'

const loading = ref(true)
const saved = ref<LetsForm>(blankLets()) // 服务端当前配置
const form = ref<LetsForm>(blankLets()) // 向导编辑副本
const webvpnDomain = ref('')
const editing = ref(false)

const configured = computed(() => !!(saved.value.name && saved.value.legomail))
const provider = computed(() => providerOf(saved.value.name))
const wildName = computed(() => (webvpnDomain.value ? `*.${webvpnDomain.value}` : ''))
const showWizard = computed(() => !configured.value || editing.value)

async function load() {
  loading.value = true
  try {
    const [d, w] = await Promise.all([get('/api/v1/set/other/getcertset'), get('/api/v1/webvpn/domain').catch(() => null)])
    saved.value = fromServer(d)
    webvpnDomain.value = w?.domain || ''
    if (!configured.value) form.value = fromServer(d)
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    loading.value = false
  }
}

function startEdit() {
  form.value = fromServer(JSON.parse(JSON.stringify(saved.value)))
  editing.value = true
}
function onDone() {
  editing.value = false
  load()
}

// 用已保存配置直接重新签发（掩码密钥由后端保留原值）
const issuing = ref<'' | 'main' | 'wild'>('')
async function reissue(certType: 'main' | 'wild') {
  const name = certType === 'wild' ? wildName.value : saved.value.domain
  if (!(await confirm(`将向 Let's Encrypt 签发 ${name}，成功后立即替换当前证书。签发过程需等待 DNS 生效，约 1–2 分钟。`, '立即签发', false))) return
  issuing.value = certType
  try {
    await post('/api/v1/set/other/createcert', { ...saved.value, certType })
    ui.message.success('证书签发成功，已自动部署')
  } catch (e) {
    ui.message.error(errMsg(e, '签发失败'))
  } finally {
    issuing.value = ''
  }
}

const HOW = [
  { t: '添加验证记录', d: '通过 DNS 服务商 API 写入 _acme-challenge TXT 记录' },
  { t: '签发证书', d: "Let's Encrypt 验证记录后签发 90 天有效期的证书" },
  { t: '部署与续期', d: '证书立即生效；开启自动续期后到期前自动重复以上流程' },
]

onMounted(load)
</script>

<template>
  <div>
    <PageHeader title="自动签发" description="使用 Let's Encrypt 免费签发服务器证书，到期前自动续期">
      <NButton v-if="configured && !editing" @click="startEdit">
        <template #icon><Edit /></template>编辑配置
      </NButton>
    </PageHeader>

    <NSpin :show="loading">
      <div class="grid gap-16px">
        <!-- 状态卡 -->
        <section class="rl-panel acme-stats grid overflow-hidden sm:grid-cols-2 xl:grid-cols-4">
          <div class="acme-cell">
            <div class="text-sm text-secondary">签发状态</div>
            <StatusDot :tone="configured ? 'success' : 'muted'" class="mt-6px text-lg font-semibold">{{ configured ? '已配置' : '未配置' }}</StatusDot>
          </div>
          <div class="acme-cell">
            <div class="text-sm text-secondary">自动续期</div>
            <StatusDot :tone="!configured ? 'muted' : saved.renew ? 'success' : 'warning'" :pulse="configured && saved.renew" class="mt-6px text-lg font-semibold">
              {{ !configured ? '—' : saved.renew ? '已开启' : '未开启' }}
            </StatusDot>
          </div>
          <div class="acme-cell">
            <div class="text-sm text-secondary">DNS 验证</div>
            <div class="mt-6px truncate text-lg font-semibold">{{ provider?.label || '—' }}</div>
          </div>
          <div class="acme-cell">
            <div class="text-sm text-secondary">联系邮箱</div>
            <div class="mt-6px truncate text-lg font-semibold" :title="saved.legomail">{{ saved.legomail || '—' }}</div>
          </div>
        </section>

        <!-- 已配置：签发目标 -->
        <RlPanel v-if="configured && !editing" title="签发目标" subtitle="使用已保存的配置立即签发，成功后自动替换对应的服务器证书" :padded="false">
          <div class="acme-row">
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-8px">
                <span class="text-md font-semibold">主证书</span>
                <span v-if="saved.renew && saved.domain" class="ax-badge text-success">自动续期</span>
              </div>
              <div class="rl-mono mt-2px truncate text-secondary">{{ saved.domain || '未设置域名' }}</div>
            </div>
            <NButton :disabled="!saved.domain || !!issuing" :loading="issuing === 'main'" @click="reissue('main')">
              <template #icon><ArrowSync /></template>立即签发
            </NButton>
          </div>
          <div class="acme-row">
            <div class="min-w-0 flex-1">
              <div class="text-md font-semibold">WebVPN 泛域名证书</div>
              <div class="rl-mono mt-2px truncate text-secondary">{{ wildName || '尚未配置 WebVPN 域名' }}</div>
            </div>
            <NButton :disabled="!wildName || !!issuing" :loading="issuing === 'wild'" @click="reissue('wild')">
              <template #icon><ArrowSync /></template>立即签发
            </NButton>
          </div>
          <template #footer>
            <span class="text-sm text-secondary">如需修改域名、凭据或自动续期开关，请「编辑配置」后重新签发。</span>
          </template>
        </RlPanel>

        <!-- 未配置 / 编辑：向导 -->
        <AcmeWizard
          v-if="!loading && showWizard" v-model:form="form" :webvpn-domain="webvpnDomain" :editing="editing"
          @cancel="editing = false" @done="onDone"
        />

        <RlPanel title="工作原理">
          <ol class="m-0 grid gap-12px pl-0 text-md sm:grid-cols-3">
            <li v-for="(s, i) in HOW" :key="i" class="flex list-none gap-10px">
              <span class="h-22px w-22px flex flex-shrink-0 items-center justify-center rounded-full bg-fill text-sm font-semibold">{{ i + 1 }}</span>
              <div>
                <div class="font-medium">{{ s.t }}</div>
                <div class="text-sm text-secondary">{{ s.d }}</div>
              </div>
            </li>
          </ol>
        </RlPanel>
      </div>
    </NSpin>
  </div>
</template>

<style scoped>
.acme-stats { gap: 1px; background: var(--color-border-default); }
.acme-cell { padding: 16px 20px; min-width: 0; background: var(--color-primary); }
.acme-row { display: flex; align-items: center; gap: 16px; padding: 16px 20px; }
.acme-row + .acme-row { border-top: 1px solid var(--color-border-default); }
</style>
