<script setup lang="ts">
// 日志保留：当前策略与下次清理时间 → 保留时长（预设 + 自定义）→ 每日清理时刻；审计去重间隔只读展示
import { computed } from 'vue'
import { NInputNumber } from 'naive-ui'
import History from '~icons/fluent/history-20-regular'
import ArrowRight from '~icons/fluent/arrow-right-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import SettingRow from '@/components/SettingRow.vue'
import StatusDot from '@/components/StatusDot.vue'
import BrandStatusCard from './components/BrandStatusCard.vue'
import BrandSaveBar from './components/BrandSaveBar.vue'
import { useSettingForm } from './components/BrandUseForm'
import { RET_PRESETS, hourOf, nextCleanup, intervalText } from './components/RetDefs'
import { get, post } from '@/api/http'
import { formatDate } from '@/utils/format'

interface RetForm { life_day: number | null; clear_time: string; audit_interval?: number }

const { form, saved, dirty, loading, saving, save, reset } = useSettingForm<RetForm>(
  () => ({ life_day: 0, clear_time: '05:00' }),
  {
    fetch: async () => ({ ...({ life_day: 0, clear_time: '05:00' } as RetForm), ...((await get('/api/v1/set/other/audit_log')) || {}) }),
    // audit_interval 只读，不回传
    submit: v => post('/api/v1/set/other/audit_log/edit', { life_day: v.life_day, clear_time: v.clear_time }),
    validate: v => (v.life_day == null ? '请填写保留天数' : !v.clear_time ? '请选择清理时间' : null),
    successText: '日志保留策略已保存',
  },
)

const forever = computed(() => !saved.value.life_day)
const next = computed(() => nextCleanup(saved.value.clear_time))
const curHour = computed(() => hourOf(form.value.clear_time))
const pickHour = (h: number) => { form.value.clear_time = `${String(h).padStart(2, '0')}:00` }
const isPreset = (d: number) => form.value.life_day === d
</script>

<template>
  <PageHeader title="日志保留" description="控制审计日志保存多久、何时自动清理，平衡追溯需要与磁盘占用" />

  <div class="grid gap-16px" :class="loading && 'pointer-events-none opacity-60'">
    <!-- 当前策略 -->
    <BrandStatusCard title="审计日志" :tone="forever ? 'info' : 'success'">
      <template #icon><History /></template>
      <template #status>
        <StatusDot v-if="forever" tone="info">永久保存</StatusDot>
        <StatusDot v-else tone="success">保留 {{ saved.life_day }} 天</StatusDot>
      </template>
      <template v-if="forever">日志不会被自动清理，请留意数据库容量增长。</template>
      <template v-else>
        每天 <b class="text-primary">{{ saved.clear_time }}</b> 删除 {{ saved.life_day }} 天以前的审计日志。
      </template>
      <template v-if="!forever && next" #meta>
        <span class="ax-badge text-secondary">下次清理 {{ formatDate(next) }}</span>
      </template>
      <template #aside>
        <div class="mb-6px text-sm font-semibold text-secondary">审计去重</div>
        <div class="text-md">{{ intervalText(saved.audit_interval) }}</div>
        <div class="mt-6px text-sm text-tertiary">
          同一用户对同一目标的重复访问在间隔内只记一次。由服务配置项 <span class="rl-kbd">audit_interval</span> 控制，修改即时生效。
        </div>
        <RouterLink to="/admin/settings/service" class="mt-10px inline-flex items-center gap-4px text-md text-info hover:underline">
          前往服务配置<ArrowRight />
        </RouterLink>
      </template>
    </BrandStatusCard>

    <!-- 保留时长 -->
    <RlPanel title="保留时长" subtitle="超过时长的审计日志会在每日清理时删除" :padded="false">
      <div class="grid grid-cols-3 gap-8px p-20px sm:grid-cols-6">
        <button
          v-for="p in RET_PRESETS" :key="p.days" type="button" class="ret-preset" :class="isPreset(p.days) && 'ret-preset--on'"
          @click="form.life_day = p.days"
        >
          <span class="text-xl font-semibold tabular-nums">{{ p.big }}</span>
          <span class="text-sm text-secondary">{{ p.unit }}</span>
        </button>
      </div>
      <SettingRow label="自定义天数">
        <template #description>0 ~ 365 天，<b class="text-primary">0 表示永久保存</b></template>
        <NInputNumber v-model:value="form.life_day" :min="0" :max="365" :precision="0" class="w-180px!">
          <template #suffix>天</template>
        </NInputNumber>
      </SettingRow>
    </RlPanel>

    <!-- 清理时刻 -->
    <RlPanel title="每日清理时刻" subtitle="建议选在业务低峰，清理大量日志时会短暂占用数据库" :padded="false">
      <div class="p-20px">
        <div class="ret-hours">
          <button
            v-for="h in 24" :key="h" type="button" class="ret-hour" :class="[curHour === h - 1 && 'ret-hour--on', (h - 1 < 6) && 'ret-hour--night']"
            :title="`${String(h - 1).padStart(2, '0')}:00`" @click="pickHour(h - 1)"
          >
            {{ String(h - 1).padStart(2, '0') }}
          </button>
        </div>
        <div class="mt-10px flex flex-wrap items-center gap-12px text-sm text-secondary">
          <span>已选 <b class="rl-mono text-primary">{{ form.clear_time || '未选择' }}</b></span>
          <span class="inline-flex items-center gap-6px"><span class="ret-legend" />凌晨低峰时段</span>
        </div>
      </div>
    </RlPanel>
  </div>

  <BrandSaveBar :dirty="dirty" :saving="saving" @reset="reset" @save="save" />
</template>

<style scoped>
.ret-preset {
  display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 0; height: 72px; cursor: pointer;
  border: 1px solid var(--color-border-default); border-radius: 12px; background: transparent; font: inherit; color: var(--color-text-primary);
  transition: border-color .15s, box-shadow .15s;
}
.ret-preset:hover { border-color: var(--color-border-strong); }
.ret-preset--on { border-color: var(--color-action); box-shadow: inset 0 0 0 1px var(--color-action); }
.ret-hours { display: grid; grid-template-columns: repeat(12, minmax(0, 1fr)); gap: 6px; }
@media (min-width: 1100px) { .ret-hours { grid-template-columns: repeat(24, minmax(0, 1fr)); } }
.ret-hour {
  height: 36px; border: 1px solid var(--color-border-default); border-radius: 8px; background: transparent; cursor: pointer;
  font-family: v-mono, ui-monospace, monospace; font-size: 12px; color: var(--color-text-secondary);
}
.ret-hour:hover { border-color: var(--color-border-strong); color: var(--color-text-primary); }
.ret-hour--night { background: var(--color-fill); }
.ret-hour--on { background: var(--color-action); border-color: var(--color-action); color: var(--color-text-on-action); font-weight: 600; }
.ret-legend { width: 12px; height: 12px; border-radius: 3px; background: var(--color-fill); border: 1px solid var(--color-border-default); }
</style>
