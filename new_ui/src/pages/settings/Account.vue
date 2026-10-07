<script setup lang="ts">
// 管理员账号：顶部安全检查清单（密码 / 两步验证 / 系统告警），其下登录凭据、系统告警与紧急恢复命令
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { NButton, NInput, NModal } from 'naive-ui'
import Copy from '~icons/fluent/copy-20-regular'
import ShieldCheck from '~icons/fluent/shield-checkmark-20-regular'
import ShieldError from '~icons/fluent/shield-error-20-regular'
import Checkmark from '~icons/fluent/checkmark-circle-20-filled'
import Warning from '~icons/fluent/warning-20-filled'
import ErrorCircle from '~icons/fluent/error-circle-20-filled'
import Info from '~icons/fluent/info-20-filled'
import Password from '~icons/fluent/password-20-regular'
import Phone from '~icons/fluent/phone-key-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import SettingRow from '@/components/SettingRow.vue'
import StatusDot from '@/components/StatusDot.vue'
import AcctOtpModal from './components/AcctOtpModal.vue'
import AcctPasswordModal from './components/AcctPasswordModal.vue'
import { warnCode, warnLevel, warnMsg, warnPath, type SysWarning } from './components/SvcRestart'
import { errMsg, get, post } from '@/api/http'
import { ui } from '@/utils/ui'

// —— 系统告警 ——
const warnings = ref<SysWarning[]>([])
const loaded = ref(false)
async function loadWarnings() {
  try {
    warnings.value = (await get('/api/v1/set/soft/status'))?.warnings || []
  } catch {
    warnings.value = []
  }
}
const tempPassword = computed(() => warnings.value.some(w => warnCode(w) === 'admin_temp_password'))
const otherWarnings = computed(() => warnings.value.filter(w => warnCode(w) !== 'admin_temp_password'))
// 指向本页的告警不显示「去处理」
const warnLink = (w: SysWarning) => {
  const p = warnPath(w)
  return p && !/\/(set\/security|settings\/account)$/.test(p) ? p : ''
}
const levelIcon = { error: ErrorCircle, warning: Warning, info: Info, success: Checkmark }

// —— OTP ——
const otpEnabled = ref(false)
const otpModal = ref(false)
const otpMode = ref<'enable' | 'view'>('enable')
const disableShow = ref(false)
const disableCode = ref('')
const disabling = ref(false)

// GET /api/v1/set/otp_qr 返回 code=0 表示已启用
async function checkOtp() {
  try {
    await get('/api/v1/set/otp_qr')
    otpEnabled.value = true
  } catch {
    otpEnabled.value = false
  }
}
function openOtp(mode: 'enable' | 'view') {
  otpMode.value = mode
  otpModal.value = true
}
async function disableOtp() {
  if (disableCode.value.length < 6) return ui.message.warning('请输入当前 6 位动态验证码')
  disabling.value = true
  try {
    await post('/api/v1/set/otp_disable', { otp_code: disableCode.value })
    ui.message.success('两步验证已禁用')
    otpEnabled.value = false
    disableShow.value = false
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    disabling.value = false
  }
}

// —— 修改密码 ——
const pwdModal = ref(false)

// —— 安全检查清单 ——
const checks = computed(() => [
  tempPassword.value
    ? { ok: false, level: 'error', title: '正在使用临时密码', desc: '首次安装或命令重置后生成的临时密码只显示一次，请立即改成自己的密码。', action: '修改密码', run: () => (pwdModal.value = true) }
    : { ok: true, level: 'success', title: '已使用自定义密码', desc: '建议定期更换，并避免与其他系统共用。' },
  otpEnabled.value
    ? { ok: true, level: 'success', title: '两步验证已开启', desc: '登录时需要额外输入认证器中的 6 位动态码。' }
    : { ok: false, level: 'warning', title: '两步验证未开启', desc: '仅凭密码即可登录后台，建议开启。', action: '立即开启', run: () => openOtp('enable') },
  otherWarnings.value.length
    ? { ok: false, level: 'warning', title: `${otherWarnings.value.length} 条系统告警待处理`, desc: '可能影响服务运行或安全，见下方「系统告警」。', action: '查看', run: () => document.getElementById('acct-alerts')?.scrollIntoView({ behavior: 'smooth' }) }
    : { ok: true, level: 'success', title: '没有系统告警', desc: '配置检查未发现问题。' },
])
const passed = computed(() => checks.value.filter(c => c.ok).length)

// —— 紧急恢复 ——
const recovery = [
  { label: '重置管理员密码', desc: '忘记管理员密码时使用，执行后会生成新的临时密码', cmd: 'remlink --reset-admin-password' },
  { label: '强制关闭两步验证', desc: '认证器丢失、无法获取动态码时使用', cmd: 'remlink --disable-admin-otp' },
]
async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    ui.message.success('已复制')
  } catch {
    ui.message.warning('复制失败，请手动选择复制')
  }
}

onMounted(async () => {
  await Promise.all([loadWarnings(), checkOtp()])
  loaded.value = true
})
</script>

<template>
  <PageHeader title="管理员账号" description="后台管理员的登录密码、两步验证，以及无法登录时的紧急恢复方式。" />

  <div class="grid gap-16px">
    <!-- 安全检查 -->
    <section class="rl-panel acct-check">
      <div class="acct-check__head">
        <div class="acct-check__icon" :class="loaded && passed === checks.length ? 'text-success' : 'text-warning'">
          <component :is="!loaded || passed === checks.length ? ShieldCheck : ShieldError" />
        </div>
        <div class="min-w-0">
          <div class="text-lg font-semibold">
            {{ !loaded ? '正在检查账号安全…' : passed === checks.length ? '账号安全状况良好' : '账号存在安全隐患' }}
          </div>
          <div class="text-sm text-secondary">{{ loaded ? `${passed} / ${checks.length} 项检查通过` : '' }}</div>
        </div>
      </div>
      <div v-if="loaded" class="acct-check__list">
        <div v-for="c in checks" :key="c.title" class="acct-check__item">
          <component :is="levelIcon[c.level as keyof typeof levelIcon]" class="mt-1px flex-shrink-0 text-lg" :class="`text-${c.level}`" />
          <div class="min-w-0 flex-1">
            <div class="text-md font-semibold">{{ c.title }}</div>
            <div class="mt-2px text-sm text-secondary">{{ c.desc }}</div>
          </div>
          <NButton v-if="c.action" size="small" :type="c.level === 'error' ? 'error' : 'default'" @click="c.run?.()">{{ c.action }}</NButton>
        </div>
      </div>
    </section>

    <!-- 登录凭据 -->
    <RlPanel title="登录凭据" subtitle="登录后台需要的密码与动态验证码" :padded="false">
      <SettingRow label="密码">
        <template #description>
          <span class="inline-flex items-center gap-4px"><Password />新密码至少 8 位，修改后立即生效</span>
        </template>
        <div class="flex flex-wrap items-center gap-sm">
          <StatusDot :tone="tempPassword ? 'error' : 'success'" class="mr-sm">{{ tempPassword ? '临时密码' : '已设置' }}</StatusDot>
          <NButton :type="tempPassword ? 'primary' : 'default'" @click="pwdModal = true">修改密码</NButton>
        </div>
      </SettingRow>
      <SettingRow label="两步验证（TOTP）">
        <template #description>
          <span class="inline-flex items-center gap-4px"><Phone />使用 Google / Microsoft Authenticator 等应用生成动态码</span>
        </template>
        <div class="flex flex-wrap items-center gap-sm">
          <StatusDot :tone="otpEnabled ? 'success' : 'muted'" class="mr-sm">{{ otpEnabled ? '已开启' : '未开启' }}</StatusDot>
          <template v-if="otpEnabled">
            <NButton @click="openOtp('view')">查看密钥 / 二维码</NButton>
            <NButton type="error" secondary @click="disableShow = true">关闭</NButton>
          </template>
          <NButton v-else @click="openOtp('enable')">开启两步验证</NButton>
        </div>
      </SettingRow>
    </RlPanel>

    <!-- 系统告警 -->
    <RlPanel id="acct-alerts" title="系统告警" subtitle="服务端配置自检结果" :padded="false">
      <template #extra><span class="text-sm text-tertiary">{{ warnings.length }} 条</span></template>
      <div v-if="!warnings.length" class="flex items-center gap-8px px-20px py-16px text-md text-secondary">
        <Checkmark class="text-lg text-success" />一切正常，没有需要处理的告警
      </div>
      <div v-for="(w, i) in warnings" :key="i" class="acct-alert">
        <component :is="levelIcon[warnLevel(w)]" class="mt-1px flex-shrink-0 text-lg" :class="`text-${warnLevel(w)}`" />
        <span class="min-w-0 flex-1 text-md">{{ warnMsg(w) }}</span>
        <NButton v-if="warnCode(w) === 'admin_temp_password'" size="small" @click="pwdModal = true">修改密码</NButton>
        <RouterLink v-else-if="warnLink(w)" :to="warnLink(w)" class="flex-shrink-0 text-sm font-semibold text-primary">去处理 →</RouterLink>
      </div>
    </RlPanel>

    <!-- 紧急恢复 -->
    <RlPanel title="紧急恢复" subtitle="无法登录后台时，在服务器上通过命令行恢复" :padded="false">
      <ol class="acct-steps">
        <li><b>停止</b> RemLink 服务</li>
        <li>在服务器上<b>执行</b>对应命令</li>
        <li>重新<b>启动</b>服务后登录</li>
      </ol>
      <SettingRow v-for="r in recovery" :key="r.cmd" :label="r.label" :description="r.desc">
        <div class="flex min-w-0 items-center gap-xs rounded-8px bg-fill py-4px pl-12px pr-4px">
          <span class="select-none text-tertiary rl-mono">$</span>
          <code class="rl-mono min-w-0 flex-1 select-all truncate">{{ r.cmd }}</code>
          <button class="rl-icon-btn flex-shrink-0" title="复制命令" @click="copy(r.cmd)"><Copy /></button>
        </div>
      </SettingRow>
    </RlPanel>
  </div>

  <AcctPasswordModal v-model:show="pwdModal" @changed="loadWarnings" />
  <AcctOtpModal v-model:show="otpModal" :mode="otpMode" @enabled="otpEnabled = true" />

  <!-- 关闭两步验证 -->
  <NModal v-model:show="disableShow" preset="card" title="关闭两步验证" class="w-420px! max-w-[calc(100vw-32px)]" :bordered="false" @after-leave="disableCode = ''">
    <p class="m-0 mb-sm text-md text-secondary">关闭后仅凭密码即可登录后台，账号安全性会降低。请输入认证器中当前的动态码确认。</p>
    <NInput v-model:value="disableCode" :maxlength="6" placeholder="6 位动态验证码" class="rl-mono" @keyup.enter="disableOtp" />
    <template #footer>
      <div class="flex justify-end gap-sm">
        <NButton @click="disableShow = false">取消</NButton>
        <NButton type="error" :loading="disabling" @click="disableOtp">确认关闭</NButton>
      </div>
    </template>
  </NModal>
</template>

<style scoped>
.acct-check { overflow: hidden; }
.acct-check__head { display: flex; align-items: center; gap: 14px; padding: 18px 20px; }
.acct-check__icon {
  display: flex; flex-shrink: 0; align-items: center; justify-content: center; width: 44px; height: 44px;
  border-radius: 12px; background: var(--color-fill); font-size: 24px;
}
.acct-check__list { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); border-top: 1px solid var(--color-border-default); }
.acct-check__item { display: flex; align-items: flex-start; gap: 10px; padding: 14px 20px; min-width: 0; }
.acct-check__item + .acct-check__item { border-left: 1px solid var(--color-border-default); }
@media (max-width: 1100px) {
  .acct-check__list { grid-template-columns: 1fr; }
  .acct-check__item + .acct-check__item { border-left: 0; border-top: 1px solid var(--color-border-default); }
}
.acct-alert { display: flex; align-items: flex-start; gap: 10px; padding: 14px 20px; }
.acct-alert + .acct-alert { border-top: 1px solid var(--color-border-default); }
.acct-steps {
  display: flex; flex-wrap: wrap; gap: 8px 28px; margin: 0; padding: 14px 20px 14px 38px;
  font-size: 14px; color: var(--color-text-secondary); background: var(--color-secondary);
  border-bottom: 1px solid var(--color-border-default);
}
.acct-steps b { color: var(--color-text-primary); font-weight: 600; }
</style>
