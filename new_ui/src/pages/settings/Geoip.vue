<script setup lang="ts">
// IP 地理库：GeoIP 国家库状态 + 上传替换（热加载，无需重启）
import { computed, onMounted, ref } from 'vue'
import { NButton, NUpload, type UploadCustomRequestOptions } from 'naive-ui'
import ArrowUpload from '~icons/fluent/arrow-upload-20-regular'
import Globe from '~icons/fluent/globe-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import StatusDot from '@/components/StatusDot.vue'
import CountryFlag from '@/components/CountryFlag.vue'
import { get, post, errMsg } from '@/api/http'
import { ui } from '@/utils/ui'
import { formatBytes, formatDate } from '@/utils/format'

interface Status {
  enabled: boolean; path: string; file_exists: boolean; file_size: number; file_mod_time: string
  database_type: string; description: string; build_time: string; ip_version: number
  record_count: number; loaded_at: string; error?: string
}

const st = ref<Status | null>(null)
const uploading = ref(false)

async function load() {
  try {
    st.value = await get('/api/v1/set/geoip/status')
  } catch (e) {
    ui.message.error(errMsg(e))
  }
}
onMounted(load)

// 数据库超过 60 天未更新时提醒（IP 归属会随时间漂移）
const ageDays = computed(() => (st.value?.build_time ? Math.floor((Date.now() - new Date(st.value.build_time).getTime()) / 86400e3) : null))
const state = computed(() => {
  const s = st.value
  if (!s) return { tone: 'muted' as const, title: '加载中…', desc: '' }
  if (s.enabled) {
    const stale = ageDays.value != null && ageDays.value > 60
    return {
      tone: stale ? ('warning' as const) : ('success' as const),
      title: stale ? '国家识别已开启，但数据库较旧' : '国家识别已开启',
      desc: `${s.database_type || 'Country'} · 构建于 ${formatDate(s.build_time, 'YYYY-MM-DD')}${stale ? `（${ageDays.value} 天前，建议更新）` : ''}`,
    }
  }
  if (s.error) return { tone: 'error' as const, title: '数据库加载失败', desc: s.error }
  return { tone: 'muted' as const, title: '国家识别未开启', desc: '尚未放置 GeoIP 国家库，来源 IP 只区分「内网 / 未知」。上传数据库后立即生效。' }
})

async function upload({ file, onFinish, onError }: UploadCustomRequestOptions) {
  const fd = new FormData()
  fd.append('file', file.file as File)
  uploading.value = true
  try {
    st.value = await post('/api/v1/set/geoip/upload', fd)
    ui.message.success('已更新并热加载')
    onFinish()
  } catch (e) {
    ui.message.error(errMsg(e))
    onError()
  } finally {
    uploading.value = false
  }
}

const samples = ['CN', 'US', 'JP', 'SG', 'DE', 'GB']
</script>

<template>
  <div>
    <PageHeader title="IP 地理库" description="把来源 IP 识别为国家 / 地区，用于仪表盘地图、在线会话与审计日志中的国旗">
      <NUpload :show-file-list="false" accept=".mmdb,.gz,.tgz" :custom-request="upload">
        <NButton type="primary" :loading="uploading">
          <template #icon><ArrowUpload /></template>上传数据库
        </NButton>
      </NUpload>
    </PageHeader>

    <div class="grid gap-16px">
      <section class="rl-panel flex items-center gap-14px px-20px py-16px">
        <span class="h-40px w-40px flex flex-shrink-0 items-center justify-center rounded-10px bg-fill text-xl" :class="`text-${state.tone === 'muted' ? 'secondary' : state.tone}`">
          <Globe />
        </span>
        <div class="min-w-0 flex-1">
          <div class="text-lg font-semibold">{{ state.title }}</div>
          <div class="text-sm text-secondary">{{ state.desc }}</div>
        </div>
        <div class="hidden items-center gap-6px md:flex">
          <CountryFlag v-for="c in samples" :key="c" :code="st?.enabled ? c : ''" :size="14" />
        </div>
      </section>

      <RlPanel title="当前数据库" :padded="false">
        <dl class="m-0 grid grid-cols-1 md:grid-cols-2">
          <div class="rl-kv"><dt>状态</dt><dd><StatusDot :tone="st?.enabled ? 'success' : st?.error ? 'error' : 'muted'">{{ st?.enabled ? '已加载' : st?.error ? '加载失败' : '未加载' }}</StatusDot></dd></div>
          <div class="rl-kv"><dt>文件路径</dt><dd class="rl-mono truncate" :title="st?.path">{{ st?.path || '—' }}</dd></div>
          <div class="rl-kv"><dt>数据库类型</dt><dd>{{ st?.database_type || '—' }}</dd></div>
          <div class="rl-kv"><dt>构建时间</dt><dd>{{ st?.build_time && st.enabled ? formatDate(st.build_time, 'YYYY-MM-DD') : '—' }}</dd></div>
          <div class="rl-kv"><dt>文件大小</dt><dd>{{ st?.file_exists ? formatBytes(st.file_size) : '文件不存在' }}</dd></div>
          <div class="rl-kv"><dt>网段记录</dt><dd>{{ st?.enabled ? (st.record_count < 0 ? '统计中…' : st.record_count.toLocaleString()) : '—' }}</dd></div>
          <div class="rl-kv"><dt>IP 版本</dt><dd>{{ st?.enabled ? (st.ip_version === 6 ? 'IPv4 + IPv6' : 'IPv4') : '—' }}</dd></div>
          <div class="rl-kv"><dt>加载时间</dt><dd>{{ st?.enabled ? formatDate(st.loaded_at) : '—' }}</dd></div>
        </dl>
      </RlPanel>

      <RlPanel title="获取数据库">
        <ul class="m-0 grid gap-8px pl-18px text-md text-secondary">
          <li>
            <strong class="text-primary">DB-IP IP to Country Lite</strong>（免费，CC BY 4.0，需在页面中注明来源）：
            从 <a class="text-info" href="https://db-ip.com/db/download/ip-to-country-lite" target="_blank" rel="noopener">db-ip.com</a> 下载 MMDB 格式（.mmdb.gz），可直接上传。
          </li>
          <li>
            <strong class="text-primary">MaxMind GeoLite2 Country</strong>（免费，需注册并同意 EULA）：
            下载 GeoLite2-Country 的 .tar.gz，可直接上传，系统会自动解出其中的 .mmdb。
          </li>
          <li>也可以把文件直接放到上面的「文件路径」，服务会检测到变更并自动热加载；路径可在 <RouterLink to="/admin/settings/service" class="text-info">服务配置</RouterLink> 的 geoip_db 中修改。</li>
          <li>IP 归属会随时间变化，建议每月更新一次。</li>
        </ul>
      </RlPanel>
    </div>
  </div>
</template>

<style scoped>
.rl-kv { display: flex; align-items: center; gap: 16px; padding: 14px 20px; border-bottom: 1px solid var(--color-border-default); min-width: 0; }
.rl-kv dt { width: 88px; flex-shrink: 0; color: var(--color-text-secondary); font-size: 13px; }
.rl-kv dd { margin: 0; min-width: 0; }
</style>
