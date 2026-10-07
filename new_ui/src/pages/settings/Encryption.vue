<script setup lang="ts">
// 数据加密：状态卡（是否加密 / 密钥文件）→ 加密说明 → 危险区关闭加密；启用 / 导入走分步向导
import { onMounted, ref } from 'vue'
import { NAlert, NButton, NSkeleton, NSpin } from 'naive-ui'
import LockClosed from '~icons/fluent/lock-closed-20-regular'
import LockOpen from '~icons/fluent/lock-open-20-regular'
import Copy from '~icons/fluent/copy-20-regular'
import Key from '~icons/fluent/key-20-regular'
import ArrowClockwise from '~icons/fluent/arrow-clockwise-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import SettingRow from '@/components/SettingRow.vue'
import StatusDot from '@/components/StatusDot.vue'
import EncSetupDrawer from './components/EncSetupDrawer.vue'
import { errMsg, get, post } from '@/api/http'
import { confirm, ui } from '@/utils/ui'

const loading = ref(true)
const enabled = ref(false)
const keyPath = ref('')
const busy = ref('')
const showSetup = ref(false)
// 关闭加密后后端返回的提示
const disableWarnings = ref<string[]>([])

async function load() {
  loading.value = true
  try {
    const d = await get('/api/v1/set/secret/status')
    enabled.value = !!d?.enabled
    keyPath.value = d?.key_path || ''
  } catch {
    enabled.value = false
  } finally {
    loading.value = false
  }
}

async function disable() {
  if (!(await confirm('系统会把所有敏感字段解密为明文写回数据库，然后删除加密密钥。此操作不可撤销，确定关闭？', '关闭数据加密'))) return
  busy.value = '正在解密数据…'
  try {
    const d = await post('/api/v1/set/secret/disable')
    enabled.value = false
    keyPath.value = ''
    disableWarnings.value = d?.warnings || []
    ui.message.success('加密已关闭，敏感字段已恢复为明文存储')
  } catch (e) {
    ui.message.error(errMsg(e, '关闭加密失败'))
  } finally {
    busy.value = ''
  }
}

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    ui.message.success('已复制')
  } catch {
    ui.message.warning('复制失败，请手动选择复制')
  }
}

function onSetupDone() {
  disableWarnings.value = []
  load()
}

const facts = [
  { label: '加密范围', desc: '系统设置里的密码与 API 密钥（邮件、短信等）、认证源配置（如 LDAP 绑定密码）、客户端证书私钥、JWT 签名密钥、管理员 OTP 密钥' },
  { label: '加密算法', desc: 'AES-256；用户名、日志、统计等普通数据不加密，不影响查询与性能' },
  { label: '密钥存放', desc: '服务器工作目录下的密钥文件，可用环境变量 REMLINK_ENCRYPTION_KEY_DIR 指定目录' },
  { label: '迁移与恢复', desc: '把数据库搬到新服务器时，需要一并带上同一把密钥，并在新服务器上「导入已有密钥」' },
]

onMounted(load)
</script>

<template>
  <PageHeader title="数据加密" description="用 AES-256 加密数据库中的密码、密钥、证书等敏感字段，即使数据库文件泄露也无法直接读取。">
    <button class="rl-icon-btn" title="刷新" @click="load"><ArrowClockwise /></button>
  </PageHeader>

  <NSpin :show="!!busy" :description="busy">
    <div class="grid gap-16px">
      <!-- 状态卡 -->
      <section class="rl-panel enc-hero">
        <div v-if="loading" class="p-20px"><NSkeleton text :repeat="3" /></div>
        <template v-else>
          <div class="enc-hero__main">
            <div class="enc-hero__icon" :class="enabled ? 'text-success' : 'text-tertiary'">
              <component :is="enabled ? LockClosed : LockOpen" />
            </div>
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-sm">
                <span class="text-xl font-semibold">{{ enabled ? '敏感数据已加密' : '敏感数据未加密' }}</span>
                <StatusDot :tone="enabled ? 'success' : 'warning'">{{ enabled ? '已启用' : '未启用' }}</StatusDot>
              </div>
              <div class="mt-4px text-md text-secondary">
                {{ enabled
                  ? '数据库中的密码、密钥、证书等字段均以密文存储，新写入的数据会自动加密。'
                  : '敏感字段目前以明文保存在数据库中，拿到数据库文件即可读取。建议启用加密。' }}
              </div>
            </div>
            <NButton v-if="!enabled" type="primary" @click="showSetup = true">
              <template #icon><Key /></template>
              启用加密
            </NButton>
          </div>
          <div v-if="enabled" class="enc-hero__key">
            <div class="min-w-0 flex-1">
              <div class="text-sm text-secondary">密钥文件</div>
              <div class="mt-2px flex min-w-0 items-center gap-4px">
                <code class="rl-mono min-w-0 select-all break-all">{{ keyPath || '—' }}</code>
                <button v-if="keyPath" class="rl-icon-btn enc-mini flex-shrink-0" title="复制路径" @click="copy(keyPath)"><Copy /></button>
              </div>
            </div>
            <div class="enc-hero__tip">
              <span class="ax-badge text-warning">务必备份</span>
              <span class="text-sm text-secondary">密钥丢失 = 加密数据永久无法恢复</span>
            </div>
          </div>
        </template>
      </section>

      <NAlert v-for="(w, i) in disableWarnings" :key="i" type="warning" :bordered="false" closable>{{ w }}</NAlert>

      <!-- 说明 -->
      <RlPanel title="加密如何工作" :padded="false">
        <SettingRow v-for="f in facts" :key="f.label" :label="f.label">
          <span class="text-md text-secondary">{{ f.desc }}</span>
        </SettingRow>
      </RlPanel>

      <!-- 危险区 -->
      <RlPanel v-if="enabled" :padded="false" class="enc-danger">
        <template #header><div class="text-lg font-semibold text-error">危险区</div></template>
        <SettingRow label="关闭数据加密">
          <template #description>
            所有敏感字段会被<b class="text-primary">解密为明文</b>写回数据库，随后<b class="text-primary">删除密钥文件</b>。
            之后任何能读取数据库文件的人都能看到这些数据。
          </template>
          <NButton type="error" secondary :loading="!!busy" @click="disable">关闭加密</NButton>
        </SettingRow>
      </RlPanel>
    </div>
  </NSpin>

  <EncSetupDrawer v-model:show="showSetup" @done="onSetupDone" />
</template>

<style scoped>
.enc-hero { overflow: hidden; }
.enc-hero__main { display: flex; flex-wrap: wrap; align-items: center; gap: 16px; padding: 22px 24px; }
.enc-hero__icon {
  display: flex; flex-shrink: 0; align-items: center; justify-content: center; width: 52px; height: 52px;
  border-radius: 14px; background: var(--color-fill); font-size: 26px;
}
.enc-hero__key {
  display: flex; flex-wrap: wrap; align-items: center; gap: 12px 24px; padding: 14px 24px;
  background: var(--color-secondary); border-top: 1px solid var(--color-border-default);
}
.enc-hero__tip { display: flex; align-items: center; gap: 8px; }
.enc-mini { width: 26px; height: 26px; font-size: 16px; }
.enc-danger { border-color: color-mix(in srgb, var(--color-text-error) 40%, var(--color-border-default)); }
</style>
