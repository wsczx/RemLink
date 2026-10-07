<script setup lang="ts">
// 用户门户：公告、快捷链接、客户端指引、卡片显隐、主题色与自定义 CSS；右侧实时预览
import { computed, onMounted, ref } from 'vue'
import { NColorPicker, NInput, NSwitch } from 'naive-ui'
import Eye from '~icons/fluent/eye-20-regular'
import Open from '~icons/fluent/open-20-regular'
import Megaphone from '~icons/fluent/megaphone-20-regular'
import Checkmark from '~icons/fluent/checkmark-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import SettingRow from '@/components/SettingRow.vue'
import BrandSaveBar from './components/BrandSaveBar.vue'
import PortalLinksEditor from './components/PortalLinksEditor.vue'
import PortalGuideEditor from './components/PortalGuideEditor.vue'
import PortalCssHelp from './components/PortalCssHelp.vue'
import PortalPreview from './components/PortalPreview.vue'
import { parseJSON, useSettingForm } from './components/BrandUseForm'
import {
  CARD_OPTIONS, LEVELS, THEME_SWATCHES, defaultCards, defaultGuide, isHexColor, normHex,
  type GuideGroup, type QuickLink,
} from './components/PortalDefs'
import { get, post } from '@/api/http'

interface PortalForm {
  dash: Record<string, any>
  links: QuickLink[]
  guide: GuideGroup[]
  cards: Record<string, boolean>
}

const { form, dirty, loading, saving, save, reset } = useSettingForm<PortalForm>(
  () => ({ dash: {}, links: [], guide: defaultGuide(), cards: defaultCards() }),
  {
    fetch: async () => {
      const d = (await get('/api/v1/set/portal_dashboard')) || {}
      const ql = parseJSON<any[]>(d.quick_links, [])
      const cg = parseJSON<any[]>(d.client_guide, [])
      return {
        dash: d,
        links: Array.isArray(ql) ? ql : [],
        guide: Array.isArray(cg) && cg.length ? cg : defaultGuide(),
        cards: { ...defaultCards(), ...parseJSON<Record<string, boolean>>(d.cards_visible, {}) },
      }
    },
    submit: v => post('/api/v1/set/portal_dashboard/edit', {
      ...v.dash,
      quick_links: JSON.stringify(v.links),
      client_guide: JSON.stringify(v.guide),
      cards_visible: JSON.stringify(v.cards),
    }),
    successText: '门户设置已保存，用户刷新门户后生效',
  },
)

// 开关：1=开启 2=关闭；公告 / 快捷链接未配置时视为开，客户端指引未配置时视为关（与门户一致）
const flag = (key: string, defOn: boolean) => computed({
  get: () => (defOn ? form.value.dash[key] !== 2 : form.value.dash[key] === 1),
  set: (v: boolean) => { form.value.dash[key] = v ? 1 : 2 },
})
const announcementOn = flag('announcement_enabled', true)
const linksOn = flag('quick_links_enabled', true)
const guideOn = flag('client_guide_enabled', false)

const visibleCount = computed(() => CARD_OPTIONS.filter(c => form.value.cards[c.key] !== false).length)
const themeValid = computed(() => !form.value.dash.theme_color || isHexColor(form.value.dash.theme_color))
const isSwatch = (c: string) => isHexColor(form.value.dash.theme_color) && normHex(form.value.dash.theme_color).toLowerCase() === c

const portalUrl = `${import.meta.env.BASE_URL}#/portal`

// 预览用品牌信息（只读，在「品牌」页修改）
const brand = ref<Record<string, any>>({})
onMounted(async () => {
  try { brand.value = (await get('/api/v1/set/portal_brand')) || {} } catch { /* 预览可缺省 */ }
})
</script>

<template>
  <PageHeader title="用户门户" description="决定用户登录门户后首页看到什么：公告、常用入口、连接指引和外观">
    <a :href="portalUrl" target="_blank" rel="noopener" class="portal-open"><Open />打开门户</a>
  </PageHeader>

  <div class="portal-layout" :class="loading && 'pointer-events-none opacity-60'">
    <div class="grid min-w-0 content-start gap-16px">
      <!-- 公告 -->
      <RlPanel title="公告横幅" subtitle="在门户首页顶部向所有用户广播通知，如维护窗口、策略变更" :padded="false">
        <template #extra><NSwitch v-model:value="announcementOn" /></template>
        <div v-if="announcementOn" class="grid gap-12px p-20px">
          <div class="flex flex-wrap items-center gap-8px">
            <span class="mr-4px text-sm text-secondary">级别</span>
            <button
              v-for="l in LEVELS" :key="l.value" type="button" class="portal-level" :class="[`text-${l.value}`, (form.dash.announcement_level || 'info') === l.value && 'portal-level--on']"
              @click="form.dash.announcement_level = l.value"
            >
              <Megaphone /><span class="text-primary">{{ l.label }}</span>
            </button>
          </div>
          <NInput
            v-model:value="form.dash.announcement" type="textarea" :autosize="{ minRows: 3, maxRows: 8 }"
            placeholder="<strong>系统维护通知</strong>：本周六 22:00–24:00 VPN 将短暂中断。"
          />
          <div class="text-sm text-tertiary">支持 HTML。内容为空时即使开启也不会显示。</div>
        </div>
      </RlPanel>

      <!-- 快捷链接 -->
      <RlPanel title="快捷链接" subtitle="把员工常用的内部系统放在门户首页" :padded="false">
        <template #extra><NSwitch v-model:value="linksOn" /></template>
        <div v-if="linksOn" class="px-20px pb-16px pt-8px">
          <PortalLinksEditor v-model="form.links" />
        </div>
      </RlPanel>

      <!-- 客户端指引 -->
      <RlPanel title="客户端连接指引" subtitle="按平台告诉用户如何下载客户端并连接" :padded="false">
        <template #extra><NSwitch v-model:value="guideOn" /></template>
        <template v-if="guideOn">
          <div class="p-20px">
            <div class="mb-12px text-sm text-secondary">
              步骤支持 HTML，<span v-pre class="rl-kbd">{{server_addr}}</span> 会自动替换为用户实际连接的服务器地址。删除全部平台则门户展示默认指引。
            </div>
            <PortalGuideEditor v-model="form.guide" />
          </div>
          <SettingRow label="客户端下载弹窗" description="指引卡片中「客户端下载」按钮弹出的 HTML，可放各平台安装包链接；留空则不显示该按钮">
            <NInput
              v-model:value="form.dash.client_download_html" type="textarea" :autosize="{ minRows: 4, maxRows: 12 }" class="rl-mono"
              placeholder="<a href='/files/remlink-windows-amd64.exe'>Windows 客户端</a>"
            />
          </SettingRow>
        </template>
      </RlPanel>

      <!-- 卡片显隐 -->
      <RlPanel title="首页模块" :subtitle="`当前向用户展示 ${visibleCount} / ${CARD_OPTIONS.length} 个模块`">
        <div class="grid grid-cols-1 gap-10px sm:grid-cols-2 xl:grid-cols-3">
          <label v-for="o in CARD_OPTIONS" :key="o.key" class="portal-card" :class="form.cards[o.key] === false && 'portal-card--off'">
            <div class="min-w-0 flex-1">
              <div class="truncate text-md font-semibold">{{ o.label }}</div>
              <div class="truncate text-sm text-secondary">{{ o.desc }}</div>
            </div>
            <NSwitch v-model:value="form.cards[o.key]" size="small" />
          </label>
        </div>
      </RlPanel>

      <!-- 外观 -->
      <RlPanel title="外观" subtitle="仅作用于登录后的门户首页" :padded="false">
        <SettingRow label="主题色" description="门户按钮、高亮等使用的主色，留空使用默认配色">
          <div class="flex flex-wrap items-center gap-8px">
            <button
              v-for="c in THEME_SWATCHES" :key="c" type="button" class="portal-swatch" :style="{ background: c }" :title="c"
              @click="form.dash.theme_color = c"
            >
              <Checkmark v-if="isSwatch(c)" />
            </button>
            <NColorPicker
              v-model:value="form.dash.theme_color" :modes="['hex']" :show-alpha="false" class="w-36px!" size="small"
            />
          </div>
          <NInput
            v-model:value="form.dash.theme_color" placeholder="默认" clearable class="rl-mono mt-10px w-180px!"
            :status="themeValid ? undefined : 'warning'"
          />
          <div v-if="!themeValid" class="mt-4px text-sm text-warning">需为 6 位十六进制颜色，如 #2f7cff，否则门户忽略此设置</div>
        </SettingRow>
        <SettingRow label="自定义 CSS" description="注入到门户首页 <head>，可调整配色甚至布局；登录页不受影响">
          <NInput
            v-model:value="form.dash.custom_css" type="textarea" :autosize="{ minRows: 5, maxRows: 16 }" class="rl-mono"
            placeholder=".portal-footer { display: none !important; }"
          />
          <PortalCssHelp />
        </SettingRow>
      </RlPanel>
    </div>

    <!-- 预览 -->
    <aside class="portal-aside">
      <RlPanel title="实时预览" subtitle="示意图，自定义 CSS 不在此呈现">
        <template #extra><Eye class="text-lg text-tertiary" /></template>
        <PortalPreview
          :brand="brand" :theme-color="form.dash.theme_color"
          :announcement-on="announcementOn" :announcement="form.dash.announcement" :level="form.dash.announcement_level"
          :links-on="linksOn" :links="form.links" :guide-on="guideOn" :guide="form.guide" :cards="form.cards"
        />
        <RouterLink to="/admin/settings/brand" class="mt-12px inline-block text-sm text-info hover:underline">名称和 Logo 在「品牌」中设置</RouterLink>
      </RlPanel>
    </aside>
  </div>

  <BrandSaveBar :dirty="dirty" :saving="saving" @reset="reset" @save="save" />
</template>

<style scoped>
.portal-layout { display: grid; grid-template-columns: minmax(0, 1fr); gap: 16px; align-items: start; }
.portal-aside { min-width: 0; }
@media (min-width: 1200px) {
  .portal-layout { grid-template-columns: minmax(0, 1fr) 380px; }
  .portal-aside { position: sticky; top: 16px; }
}
.portal-open {
  display: inline-flex; align-items: center; gap: 6px; height: 34px; padding: 0 12px; border-radius: 8px; font-size: 14px;
  border: 1px solid var(--color-border-default); color: var(--color-text-primary);
}
.portal-open:hover { border-color: var(--color-border-strong); }
.portal-level {
  display: inline-flex; align-items: center; gap: 6px; height: 30px; padding: 0 12px; border-radius: 15px; cursor: pointer;
  border: 1px solid var(--color-border-default); background: transparent; font: inherit; font-size: 13px;
}
.portal-level--on { border-color: currentColor; box-shadow: inset 0 0 0 1px currentColor; }
.portal-card {
  display: flex; align-items: center; gap: 10px; padding: 12px 14px; border-radius: 10px; cursor: pointer;
  border: 1px solid var(--color-border-default); transition: opacity .15s, border-color .15s;
}
.portal-card:hover { border-color: var(--color-border-strong); }
.portal-card--off { opacity: .55; border-style: dashed; }
.portal-swatch {
  display: inline-flex; align-items: center; justify-content: center; width: 28px; height: 28px; border-radius: 8px; cursor: pointer;
  border: 1px solid var(--color-border-default); color: white; font-size: 16px; padding: 0;
}
</style>
