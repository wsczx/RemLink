<script setup lang="ts">
// 我的证书：按用户组签发的客户端证书，设置密码后下载 P12
import { ref } from 'vue'
import axios from 'axios'
import { NButton, NInput, NModal, NSpin, NTooltip } from 'naive-ui'
import Certificate from '~icons/fluent/certificate-20-regular'
import ArrowClockwise from '~icons/fluent/arrow-clockwise-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import EmptyState from '@/components/EmptyState.vue'
import StatusDot from '@/components/StatusDot.vue'
import { ui } from '@/utils/ui'
import { usePortal } from '../store'
import { PORTAL_HEADERS, formatCertTime } from '../shared'

const p = usePortal()
const { certs, certsLoading } = p

const tone = (s: number) => (s === 0 ? 'success' : s === 1 ? 'warning' : 'error')

const visible = ref(false)
const group = ref('')
const password = ref('')
const downloading = ref(false)

function open(c: any) {
  group.value = c.groupname
  password.value = ''
  visible.value = true
}

async function download() {
  if (!password.value || password.value.length < 4) return ui.message.warning('P12 密码至少 4 位')
  downloading.value = true
  try {
    const resp = await axios.post('/api/v1/portal/certs/download',
      { groupname: group.value, password: password.value },
      { headers: PORTAL_HEADERS, responseType: 'blob' })
    const url = URL.createObjectURL(new Blob([resp.data]))
    const a = document.createElement('a')
    a.href = url
    a.download = group.value + '.p12'
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
    visible.value = false
    password.value = ''
    group.value = ''
  } catch {
    ui.message.error('下载失败')
  } finally {
    downloading.value = false
  }
}
</script>

<template>
  <div>
    <PageHeader title="我的证书" description="客户端证书用于证书认证登录，下载后导入 VPN 客户端即可使用">
      <button class="rl-icon-btn" title="刷新" :disabled="certsLoading" @click="p.loadCerts()"><ArrowClockwise /></button>
    </PageHeader>

    <RlPanel :padded="false">
      <NSpin :show="certsLoading && !certs.length">
        <EmptyState v-if="!certs.length" title="暂无客户端证书" description="你所在的用户组未启用证书认证，或管理员尚未签发" :icon="Certificate" />
        <div
          v-for="c in certs" :key="c.id"
          class="cert-row flex flex-wrap items-center gap-x-16px gap-y-10px px-20px py-16px"
          :class="c.status !== 0 && 'opacity-60'"
        >
          <div class="flex h-40px w-40px flex-shrink-0 items-center justify-center rounded-10px bg-fill text-xl text-secondary"><Certificate /></div>
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-8px">
              <span class="truncate text-md font-semibold">{{ c.groupname }}</span>
              <StatusDot :tone="tone(c.status)">{{ c.status_text }}</StatusDot>
            </div>
            <div class="mt-2px flex flex-wrap gap-x-12px text-sm text-secondary">
              <span>{{ formatCertTime(c.not_after) }} 到期</span>
              <span v-if="c.serial_number" class="rl-mono truncate">SN {{ c.serial_number }}</span>
              <span v-if="c.is_csr_based">CSR 模式</span>
              <span v-if="c.device_binding_enabled">已绑定 {{ c.device_count }} / {{ c.max_devices }} 台设备</span>
            </div>
          </div>
          <NButton v-if="c.status === 0" size="small" @click="open(c)">下载 P12</NButton>
          <NTooltip v-else>
            <template #trigger><NButton size="small" disabled>不可下载</NButton></template>
            {{ c.status === 1 ? '证书已禁用' : '证书已过期' }}
          </NTooltip>
        </div>
      </NSpin>
    </RlPanel>

    <NModal v-model:show="visible" preset="card" title="下载客户端证书" :style="{ width: 'min(400px, calc(100vw - 32px))' }" :segmented="{ footer: true }">
      <p class="m-0 mb-10px text-md text-secondary">为证书文件设置一个密码，导入客户端时需要输入此密码。</p>
      <NInput v-model:value="password" type="password" show-password-on="click" placeholder="至少 4 位" @keydown.enter.prevent="download" />
      <template #footer>
        <div class="flex justify-end gap-10px">
          <NButton @click="visible = false">取消</NButton>
          <NButton type="primary" :loading="downloading" @click="download">下载</NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>

<style scoped>
.cert-row + .cert-row { border-top: 1px solid var(--color-border-default); }
</style>
