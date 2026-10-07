<script setup lang="ts">
// 品牌：左侧配置（名称 / 图标 / 登录页亮点 / 页脚），右侧实时预览；保存后广播 remlink:brand-updated 立即生效
import { computed } from 'vue'
import { NInput, NSwitch } from 'naive-ui'
import Eye from '~icons/fluent/eye-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import SettingRow from '@/components/SettingRow.vue'
import BrandImageField from './components/BrandImageField.vue'
import BrandPreview from './components/BrandPreview.vue'
import BrandSaveBar from './components/BrandSaveBar.vue'
import { parseJSON, useSettingForm } from './components/BrandUseForm'
import { get, post } from '@/api/http'
import { normalizeImageSrc } from '@/utils/brand'

interface BrandForm {
  brand: { title: string; logo: string; favicon: string; desc: string; footer: string; features_enabled: number; [k: string]: any }
  features: { label: string; desc: string }[]
}

const defaultFeatures = () => [
  { label: '账号状态', desc: '查看在线设备与连接状态' },
  { label: '用户分组', desc: '查看所属用户分组' },
  { label: '安全设置', desc: '自主修改密码与验证方式' },
]

const { form, dirty, loading, saving, save, reset } = useSettingForm<BrandForm>(
  () => ({ brand: { title: '', logo: '', favicon: '', desc: '', footer: '', features_enabled: 0 }, features: defaultFeatures() }),
  {
    fetch: async () => {
      const d = (await get('/api/v1/set/portal_brand')) || {}
      const f = parseJSON<any[]>(d.features, [])
      return {
        brand: { title: '', logo: '', favicon: '', desc: '', footer: '', features_enabled: 0, ...d },
        features: Array.isArray(f) && f.length ? f : defaultFeatures(),
      }
    },
    submit: async v => {
      // SVG 源码转 data URI，使各展示页 <img> 可直接渲染
      v.brand.logo = normalizeImageSrc(v.brand.logo)
      v.brand.favicon = normalizeImageSrc(v.brand.favicon)
      await post('/api/v1/set/portal_brand/edit', { ...v.brand, features: JSON.stringify(v.features) })
      window.dispatchEvent(new Event('remlink:brand-updated'))
    },
    successText: '品牌设置已保存，已立即生效',
  },
)

// features_enabled：0 未配置（默认显示）/ 1 显示 / 2 关闭
const featuresOn = computed({
  get: () => form.value.brand.features_enabled !== 2,
  set: v => { form.value.brand.features_enabled = v ? 1 : 2 },
})
</script>

<template>
  <PageHeader title="品牌" description="自定义名称、Logo 与登录页文案，统一应用到用户门户、WebAuth 和管理后台" />

  <div class="brand-layout" :class="loading && 'pointer-events-none opacity-60'">
    <!-- 左：配置 -->
    <div class="grid min-w-0 content-start gap-16px">
      <RlPanel title="品牌标识" subtitle="留空的项会回退到 RemLink 默认展示" :padded="false">
        <SettingRow label="品牌名称" description="显示在登录页、门户顶栏和浏览器标签上">
          <NInput v-model:value="form.brand.title" placeholder="RemLink" />
        </SettingRow>
        <SettingRow label="副标题" description="品牌名下方的一句话介绍">
          <NInput v-model:value="form.brand.desc" placeholder="企业级安全远程接入网关用户门户" />
        </SettingRow>
        <SettingRow label="Logo" description="建议 48×48 以上的方形 PNG 或 SVG">
          <BrandImageField v-model="form.brand.logo" label="Logo " hint="支持 PNG / SVG，≤ 1MB" :size="40" />
        </SettingRow>
        <SettingRow label="网站图标" description="浏览器标签页上的小图标（favicon）">
          <BrandImageField v-model="form.brand.favicon" label="Favicon " hint="建议 32×32，支持 ICO / PNG / SVG" :size="24" />
        </SettingRow>
      </RlPanel>

      <RlPanel title="登录页亮点" subtitle="门户登录框下方的三条功能介绍" :padded="false">
        <template #extra><NSwitch v-model:value="featuresOn" /></template>
        <div v-if="featuresOn" class="grid grid-cols-1 gap-12px p-20px md:grid-cols-3">
          <div v-for="(f, i) in form.features" :key="i" class="brand-feature">
            <div class="mb-8px text-sm text-tertiary tabular-nums">亮点 {{ i + 1 }}</div>
            <NInput v-model:value="f.label" placeholder="标题" />
            <NInput v-model:value="f.desc" type="textarea" :autosize="{ minRows: 2, maxRows: 3 }" placeholder="一句话描述" class="mt-8px" />
          </div>
        </div>
        <div v-else class="px-20px py-16px text-md text-tertiary">已隐藏，登录页只显示登录表单。</div>
      </RlPanel>

      <RlPanel title="页脚" subtitle="显示在登录页底部，支持 HTML，留空使用默认页脚" :padded="false">
        <div class="p-20px">
          <NInput
            v-model:value="form.brand.footer" type="textarea" :rows="4" class="rl-mono"
            placeholder="© 2026 Example Inc. · <a href='https://example.com'>帮助中心</a>"
          />
          <div class="mt-8px text-sm text-tertiary">
            可使用 <span class="rl-kbd">.github-badge</span> 样式实现徽章式页脚。
          </div>
        </div>
      </RlPanel>
    </div>

    <!-- 右：实时预览 -->
    <aside class="brand-aside">
      <RlPanel title="实时预览" subtitle="未保存的修改也会立即呈现">
        <template #extra><Eye class="text-lg text-tertiary" /></template>
        <BrandPreview
          :title="form.brand.title" :desc="form.brand.desc" :logo="form.brand.logo" :favicon="form.brand.favicon"
          :footer="form.brand.footer" :features-on="featuresOn" :features="form.features"
        />
      </RlPanel>
    </aside>
  </div>

  <BrandSaveBar :dirty="dirty" :saving="saving" hint="保存后门户、WebAuth 与后台即时生效" @reset="reset" @save="save" />
</template>

<style scoped>
.brand-layout { display: grid; grid-template-columns: minmax(0, 1fr); gap: 16px; align-items: start; }
.brand-aside { min-width: 0; }
@media (min-width: 1200px) {
  .brand-layout { grid-template-columns: minmax(0, 1fr) 400px; }
  .brand-aside { position: sticky; top: 16px; }
}
.brand-feature { padding: 12px; border: 1px solid var(--color-border-default); border-radius: 10px; background: var(--color-secondary); }
</style>
