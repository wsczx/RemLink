<script setup lang="ts">
// 客户端证书：CA 状态（未初始化时引导）→ 已签发证书列表 → 危险区（强制重置 CA）
import { onMounted, ref } from 'vue'
import { NButton, NSkeleton } from 'naive-ui'
import { RouterLink } from 'vue-router'
import Add from '~icons/fluent/add-20-regular'
import PeopleAdd from '~icons/fluent/people-add-20-regular'
import ShieldKeyhole from '~icons/fluent/shield-keyhole-20-regular'
import ShieldCheck from '~icons/fluent/shield-checkmark-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import StatusDot from '@/components/StatusDot.vue'
import ClientCertTable from './components/ClientCertTable.vue'
import ClientGenerateDrawer from './components/ClientGenerateDrawer.vue'
import ClientBatchDrawer from './components/ClientBatchDrawer.vue'
import type { CertStats, CertUser } from './components/client'
import { errMsg, get, post } from '@/api/http'
import { confirm, ui } from '@/utils/ui'

// —— CA ——
const caReady = ref(false)
const caLoading = ref(true)
const caBusy = ref(false)
async function loadCA() {
  try { caReady.value = !!(await get('/api/v1/set/client_cert/ca_status'))?.initialized } catch { /* 忽略 */ } finally { caLoading.value = false }
}
async function initCA(force: boolean) {
  const ok = force
    ? await confirm('将重新生成客户端 CA，所有已签发的客户端证书会立即失效，用户需要重新安装新证书。确定继续？', '强制重置客户端 CA')
    : await confirm('将生成客户端根 CA，用于为用户签发客户端证书。确定初始化？', '初始化客户端 CA', false)
  if (!ok) return
  caBusy.value = true
  try {
    await post(force ? '/api/v1/set/client_cert/init_ca?force=true' : '/api/v1/set/client_cert/init_ca')
    ui.message.success(force ? '客户端 CA 已重置' : '客户端 CA 已初始化')
    caReady.value = true
    table.value?.reload()
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    caBusy.value = false
  }
}

// —— 用户 / 组（签发与列表昵称用）——
const users = ref<CertUser[]>([])
const groups = ref<string[]>([])
async function loadUserInfo() {
  try {
    const d = await get('/api/v1/set/client_cert/user_cert_info')
    users.value = d?.users || []
    groups.value = (d?.groups || []).map((g: any) => g?.name || g)
  } catch { /* 忽略 */ }
}

const table = ref<InstanceType<typeof ClientCertTable>>()
const stats = ref<CertStats>({ all: 0, ok: 0, disabled: 0, expired: 0 })
const genShow = ref(false)
const batchShow = ref(false)
function openGen(batch: boolean) {
  loadUserInfo()
  if (batch) batchShow.value = true
  else genShow.value = true
}

const SETUP = [
  { t: '初始化客户端 CA', d: '生成用于签发客户端证书的根 CA' },
  { t: '在用户组启用证书认证', d: '在「用户组 → 认证方式」中添加 TLS 证书步骤', to: '/admin/group/list' },
  { t: '为用户签发并分发证书', d: '下载 P12 或通过邮件发送给用户安装' },
]

onMounted(() => {
  loadCA()
  loadUserInfo()
})
</script>

<template>
  <div>
    <PageHeader title="客户端证书" description="为用户签发 TLS 客户端证书，用于证书认证登录与设备绑定">
      <template v-if="caReady">
        <NButton @click="openGen(true)">
          <template #icon><PeopleAdd /></template>按组批量签发
        </NButton>
        <NButton type="primary" @click="openGen(false)">
          <template #icon><Add /></template>签发证书
        </NButton>
      </template>
    </PageHeader>

    <div class="grid gap-16px">
      <!-- CA 状态 -->
      <NSkeleton v-if="caLoading" height="96px" :sharp="false" class="rounded-12px!" />
      <section v-else-if="caReady" class="rl-panel flex flex-wrap items-center gap-x-32px gap-y-16px px-20px py-16px">
        <div class="flex min-w-0 flex-1 items-center gap-14px">
          <span class="h-40px w-40px flex flex-shrink-0 items-center justify-center rounded-10px bg-fill text-xl text-success"><ShieldCheck /></span>
          <div class="min-w-0">
            <StatusDot tone="success" class="text-lg font-semibold">客户端 CA 已就绪</StatusDot>
            <div class="text-sm text-secondary">由此 CA 签名的证书可用于开启了 TLS 证书认证的用户组</div>
          </div>
        </div>
        <div class="flex gap-28px">
          <div v-for="s in [
            { l: '已签发', v: stats.all, c: '' },
            { l: '启用', v: stats.ok, c: 'text-success' },
            { l: '禁用', v: stats.disabled, c: stats.disabled ? 'text-warning' : '' },
            { l: '已过期', v: stats.expired, c: stats.expired ? 'text-error' : '' },
          ]" :key="s.l">
            <div class="text-sm text-secondary">{{ s.l }}</div>
            <div class="text-4xl font-semibold tabular-nums leading-32px" :class="s.c">{{ s.v }}</div>
          </div>
        </div>
      </section>

      <!-- 未初始化：引导 -->
      <section v-else class="rl-panel grid gap-24px px-24px py-24px lg:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)] lg:items-center">
        <div>
          <span class="h-44px w-44px flex items-center justify-center rounded-10px bg-fill text-xl"><ShieldKeyhole /></span>
          <div class="mt-14px text-xl font-semibold">启用客户端证书认证</div>
          <div class="mt-4px max-w-440px text-md text-secondary">让用户凭设备上安装的证书登录 VPN，可与密码、OTP 组合，并可限制证书绑定的设备数量。首先需要初始化一个客户端 CA。</div>
          <NButton type="primary" class="mt-16px" :loading="caBusy" @click="initCA(false)">初始化 CA</NButton>
        </div>
        <ol class="m-0 grid gap-10px p-0">
          <li v-for="(s, i) in SETUP" :key="i" class="flex list-none items-start gap-12px rounded-10px bg-fill px-14px py-12px">
            <span class="h-22px w-22px flex flex-shrink-0 items-center justify-center rounded-full bg-primary text-sm font-semibold" :class="i === 0 && 'cl-step--now'">{{ i + 1 }}</span>
            <div class="min-w-0">
              <div class="text-md font-medium">{{ s.t }}</div>
              <div class="text-sm text-secondary">
                {{ s.d }}
                <RouterLink v-if="s.to" :to="s.to" class="ml-4px text-info">前往</RouterLink>
              </div>
            </div>
          </li>
        </ol>
      </section>

      <!-- 证书列表 -->
      <ClientCertTable ref="table" :users="users" @stats="v => (stats = v)" />

      <!-- 危险区 -->
      <RlPanel v-if="caReady" :padded="false">
        <template #header>
          <div class="text-lg font-semibold text-error">危险区</div>
        </template>
        <div class="flex flex-wrap items-center gap-16px px-20px py-16px">
          <div class="min-w-0 flex-1">
            <div class="text-md font-semibold">强制重置客户端 CA</div>
            <div class="mt-2px text-sm text-secondary">重新生成 CA 根证书与私钥。所有已签发的客户端证书（当前 {{ stats.all }} 张）将立即失效，用户需重新安装新签发的证书才能登录。</div>
          </div>
          <NButton type="error" secondary :loading="caBusy" @click="initCA(true)">重置 CA</NButton>
        </div>
      </RlPanel>
    </div>

    <ClientGenerateDrawer v-model:show="genShow" :users="users" @done="table?.reload()" />
    <ClientBatchDrawer v-model:show="batchShow" :users="users" :groups="groups" @done="table?.reload()" />
  </div>
</template>

<style scoped>
.cl-step--now { background: var(--color-text-primary); color: var(--color-primary); }
</style>
