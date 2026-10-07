<script setup lang="ts">
// 访问地址与模板：公网地址、客户端登录 Banner、根路径首页行为、邮件模板（编辑 + 示例数据预览）
import { computed, ref } from 'vue'
import { NButton, NInput, NInputNumber, NSwitch } from 'naive-ui'
import Globe from '~icons/fluent/globe-20-regular'
import Home from '~icons/fluent/home-20-regular'
import Code from '~icons/fluent/code-20-regular'
import Warning from '~icons/fluent/warning-20-regular'
import MailIcon from '~icons/fluent/mail-20-regular'
import Certificate from '~icons/fluent/certificate-20-regular'
import ArrowDownload from '~icons/fluent/arrow-download-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import SettingRow from '@/components/SettingRow.vue'
import StatusDot from '@/components/StatusDot.vue'
import BrandChoiceCard from './components/BrandChoiceCard.vue'
import BrandSaveBar from './components/BrandSaveBar.vue'
import TplCodeEditor from './components/TplCodeEditor.vue'
import { useSettingForm } from './components/BrandUseForm'
import { ACCOUNT_VARS, CERT_VARS, homeModeOf, renderSample, type HomeMode } from './components/TplDefs'
import { get, post, errMsg } from '@/api/http'
import { ui } from '@/utils/ui'

interface OtherForm {
  link_addr: string
  banner_enable: boolean
  banner: string
  homecode: number | null
  homeindex: string
  account_mail: string
  cert_mail: string
}
const empty = (): OtherForm => ({ link_addr: '', banner_enable: false, banner: '', homecode: 200, homeindex: '', account_mail: '', cert_mail: '' })

const { form, dirty, loading, saving, save, reset } = useSettingForm<OtherForm>(empty, {
  fetch: async () => ({ ...empty(), ...((await get('/api/v1/set/other')) || {}) }),
  submit: v => post('/api/v1/set/other/edit', v),
  validate: v => (v.homecode == null ? '请填写根路径返回的 HTTP 状态码' : null),
  successText: '访问地址与模板已保存',
})

// ---------- 访问地址 ----------
const addrHasScheme = computed(() => /^[a-z]+:\/\//i.test(form.value.link_addr || ''))

// ---------- 根路径行为 ----------
const homeMode = computed(() => homeModeOf(form.value.homecode))
const HOME_MODES: { value: HomeMode; title: string; desc: string }[] = [
  { value: 'portal', title: '门户优先', desc: '门户开启时跳转门户；门户关闭时展示自定义首页（状态码 200）' },
  { value: 'custom', title: '始终展示自定义首页', desc: '/ 直接展示下方 HTML，门户仍可从 /ui/#/portal 进入（状态码 0）' },
  { value: 'status', title: '返回 HTTP 状态码', desc: '门户关闭时 / 直接返回指定状态码，用于隐藏服务' },
]
function pickHome(m: HomeMode) {
  if (m === 'portal') form.value.homecode = 200
  else if (m === 'custom') form.value.homecode = 0
  else if (homeMode.value !== 'status') form.value.homecode = 404
}

// ---------- 邮件模板 ----------
type MailKey = 'account_mail' | 'cert_mail'
const mailSel = ref<MailKey>('account_mail')
const MAILS = [
  { key: 'account_mail' as const, title: '账号开通邮件', desc: '创建用户或重发开通邮件时发送', icon: MailIcon, vars: ACCOUNT_VARS },
  { key: 'cert_mail' as const, title: '证书下发邮件', desc: '向用户发送客户端证书（附 P12）时使用', icon: Certificate, vars: CERT_VARS },
]
const curMail = computed(() => MAILS.find(m => m.key === mailSel.value)!)
const useSample = ref(true)
const mailPreview = computed(() => {
  const html = form.value[mailSel.value] || ''
  return useSample.value ? renderSample(html, curMail.value.vars) : html
})

async function copyVar(k: string) {
  const text = `{{.${k}}}`
  try { await navigator.clipboard.writeText(text) } catch { /* 非 https 环境忽略 */ }
  ui.message.success(`已复制 ${text}`)
}

const certTplLoading = ref(false)
async function loadCertTemplate() {
  certTplLoading.value = true
  try {
    form.value.cert_mail = await get('/api/v1/set/client_cert/cert_mail_template')
    ui.message.success('已载入默认证书邮件模板，保存后生效')
  } catch (e) {
    ui.message.error(errMsg(e, '加载失败'))
  } finally {
    certTplLoading.value = false
  }
}
</script>

<template>
  <PageHeader title="访问地址与模板" description="对外访问地址、客户端登录提示、根路径首页，以及系统邮件的 HTML 模板" />

  <div class="grid gap-16px" :class="loading && 'pointer-events-none opacity-60'">
    <!-- 访问地址 -->
    <RlPanel title="公网访问地址" subtitle="写入开通邮件、证书邮件和客户端下载链接中的地址" :padded="false">
      <SettingRow label="VPN 对外地址" description="不含 https://，需与服务器证书的域名一致；非 443 端口请带上端口号">
        <NInput v-model:value="form.link_addr" placeholder="vpn.example.com 或 vpn.example.com:8443" class="rl-mono" :status="addrHasScheme ? 'warning' : undefined">
          <template #prefix><span class="text-tertiary">https://</span></template>
        </NInput>
        <div v-if="addrHasScheme" class="mt-6px flex items-center gap-4px text-sm text-warning">
          <Warning />请去掉协议头，只填写域名和端口
        </div>
        <div v-else-if="form.link_addr" class="mt-6px text-sm text-secondary">
          用户将在邮件中看到：<span class="rl-mono text-primary">https://{{ form.link_addr }}</span>
        </div>
        <div v-else class="mt-6px flex items-center gap-6px text-sm">
          <StatusDot tone="warning">未设置，邮件中的登录地址和二维码链接将不可用</StatusDot>
        </div>
      </SettingRow>
    </RlPanel>

    <!-- 登录 Banner -->
    <RlPanel title="客户端登录 Banner" subtitle="用户通过客户端登录时弹出的提示，如使用条款、安全须知" :padded="false">
      <template #extra><NSwitch v-model:value="form.banner_enable" /></template>
      <div v-if="form.banner_enable" class="p-20px">
        <NInput
          v-model:value="form.banner" type="textarea" :autosize="{ minRows: 4, maxRows: 10 }"
          placeholder="本系统仅限授权人员使用，所有操作均会被记录审计。"
        />
        <div class="mt-8px text-sm text-tertiary">支持 HTML；部分客户端只显示纯文本。</div>
      </div>
      <div v-else class="px-20px py-16px text-md text-tertiary">已关闭，用户登录时不弹出提示。</div>
    </RlPanel>

    <!-- 根路径首页 -->
    <RlPanel title="根路径（/）行为" subtitle="浏览器直接访问 VPN 地址时看到什么" :padded="false">
      <div class="grid grid-cols-1 gap-12px p-20px lg:grid-cols-3">
        <BrandChoiceCard
          v-for="m in HOME_MODES" :key="m.value" :title="m.title" :description="m.desc" :active="homeMode === m.value"
          @select="pickHome(m.value)"
        >
          <template #icon><component :is="m.value === 'status' ? Code : m.value === 'custom' ? Home : Globe" /></template>
        </BrandChoiceCard>
      </div>
      <SettingRow v-if="homeMode === 'status'" label="返回状态码" description="门户开启时仍会跳转到门户">
        <NInputNumber v-model:value="form.homecode" :min="1" :max="1000" class="w-160px!" />
      </SettingRow>
      <div class="b-t-1px b-t-solid b-[var(--color-border-default)] p-20px">
        <div class="mb-10px flex flex-wrap items-baseline gap-8px">
          <span class="text-md font-semibold">自定义首页 HTML</span>
          <span class="text-sm text-secondary">
            {{ homeMode === 'status' ? '当前模式不使用该内容' : '可参考程序目录下 index_template 中的示例' }}
          </span>
        </div>
        <TplCodeEditor v-model="form.homeindex" placeholder="<!doctype html>…" :height="360" />
      </div>
    </RlPanel>

    <!-- 邮件模板 -->
    <RlPanel title="邮件模板" subtitle="Go 模板语法的 HTML，发送时替换变量；需先在「邮件」中配置 SMTP" :padded="false">
      <div class="tpl-mail">
        <div class="tpl-mail__list">
          <button
            v-for="m in MAILS" :key="m.key" type="button" class="tpl-mail__item" :class="mailSel === m.key && 'tpl-mail__item--on'"
            @click="mailSel = m.key"
          >
            <component :is="m.icon" class="mt-2px flex-shrink-0 text-lg text-secondary" />
            <div class="min-w-0 flex-1">
              <div class="text-md font-semibold">{{ m.title }}</div>
              <div class="text-sm text-secondary">{{ m.desc }}</div>
              <div class="mt-4px">
                <StatusDot :tone="form[m.key] ? 'info' : 'muted'" class="text-sm">{{ form[m.key] ? '已自定义' : (m.key === 'cert_mail' ? '使用默认模板' : '未设置') }}</StatusDot>
              </div>
            </div>
          </button>
        </div>

        <div class="min-w-0 flex-1 p-20px">
          <div class="mb-10px flex flex-wrap items-center gap-6px">
            <span class="mr-2px text-sm text-secondary">变量</span>
            <button v-for="v in curMail.vars" :key="v.key" type="button" class="tpl-var" :title="`${v.label}，点击复制`" @click="copyVar(v.key)">
              .{{ v.key }}
            </button>
          </div>
          <TplCodeEditor
            v-model="form[mailSel]" :preview="mailPreview"
            :placeholder="mailSel === 'cert_mail' ? '留空使用系统默认证书邮件模板' : '<p>您好 {{.Username}}，您的 VPN 账号已开通…</p>'"
          >
            <template #toolbar>
              <label class="flex cursor-pointer items-center gap-6px text-sm text-secondary">
                <NSwitch v-model:value="useSample" size="small" />预览时填入示例数据
              </label>
              <NButton v-if="mailSel === 'cert_mail'" size="small" quaternary :loading="certTplLoading" @click="loadCertTemplate">
                <template #icon><ArrowDownload /></template>载入默认模板
              </NButton>
            </template>
          </TplCodeEditor>
        </div>
      </div>
    </RlPanel>
  </div>

  <BrandSaveBar :dirty="dirty" :saving="saving" @reset="reset" @save="save" />
</template>

<style scoped>
.tpl-mail { display: flex; flex-direction: column; }
.tpl-mail__list { display: grid; gap: 4px; padding: 12px; border-bottom: 1px solid var(--color-border-default); }
@media (min-width: 1000px) {
  .tpl-mail { flex-direction: row; }
  .tpl-mail__list { width: 260px; flex-shrink: 0; align-content: start; border-bottom: 0; border-right: 1px solid var(--color-border-default); }
}
.tpl-mail__item {
  display: flex; gap: 10px; padding: 10px 12px; border: 0; border-radius: 10px; background: transparent; text-align: left;
  font: inherit; color: var(--color-text-primary); cursor: pointer;
}
.tpl-mail__item:hover { background: var(--color-action-hover); }
.tpl-mail__item--on { background: var(--color-fill); }
.tpl-var {
  height: 24px; padding: 0 8px; border: 1px solid var(--color-border-default); border-radius: 6px; background: transparent; cursor: pointer;
  font-family: v-mono, ui-monospace, monospace; font-size: 12px; color: var(--color-text-secondary);
}
.tpl-var:hover { border-color: var(--color-border-strong); color: var(--color-text-primary); }
</style>
