<script setup lang="ts">
// 数据库切换向导：配置连接 → 测试连通 → 迁移方式 → 确认执行
import { computed, reactive, ref, watch } from 'vue'
import { NButton, NDrawer, NDrawerContent, NInput, NSelect, NStep, NSteps } from 'naive-ui'
import CheckmarkCircle from '~icons/fluent/checkmark-circle-20-regular'
import ErrorCircle from '~icons/fluent/error-circle-20-regular'
import Warning from '~icons/fluent/warning-20-regular'
import { post, errMsg } from '@/api/http'
import { ui } from '@/utils/ui'

const props = defineProps<{ currentType: string; currentSource: string }>()
const show = defineModel<boolean>('show', { required: true })
const emit = defineEmits<{ switched: [] }>()

const dbTypes = [
  { label: 'SQLite3', value: 'sqlite3' },
  { label: 'MySQL', value: 'mysql' },
  { label: 'PostgreSQL', value: 'postgres' },
  { label: 'MS SQL Server', value: 'mssql' },
]
const migrations = [
  { value: 'auto_migrate', title: '自动迁移数据', badge: '推荐', desc: '先备份当前数据库，然后将数据迁移到新数据库。迁移完成后自动写入配置并重启服务。' },
  { value: 'backup_only', title: '仅备份不迁移', desc: '创建当前数据库的全量备份文件，然后切换到新数据库启动。新库将从空数据库开始运行，备份文件可按需手动还原。' },
  { value: 'none', title: '跳过备份', desc: '不做任何备份和迁移，直接切换到新数据库。新库将以空数据库启动（会触发自动初始化）。' },
]

const step = ref(0)
const form = reactive({ db_type: '', db_source: '', migration: 'auto_migrate' })
const testing = ref(false)
const testOk = ref(false)
const testResult = ref('')
const executing = ref(false)

// 打开时预填当前数据库信息
watch(show, v => {
  if (!v) return
  Object.assign(form, { db_type: props.currentType, db_source: props.currentSource, migration: 'auto_migrate' })
  step.value = 0
  testing.value = testOk.value = executing.value = false
  testResult.value = ''
})
// 连接参数变化后需重新测试
watch(() => [form.db_type, form.db_source], () => { testOk.value = false; testResult.value = '' })

const nextDisabled = computed(() => {
  if (step.value === 0) return !form.db_type || !form.db_source
  if (step.value === 1) return !testOk.value
  return false
})
const nextText = computed(() => ['下一步', '选择迁移方式', '确认信息'][step.value])
const migrationLabel = computed(() => migrations.find(m => m.value === form.migration)?.title)

async function testConn() {
  testing.value = true
  testOk.value = false
  testResult.value = ''
  try {
    await post('/api/v1/set/db/test_connection', { db_type: form.db_type, db_source: form.db_source })
    testOk.value = true
    testResult.value = '连接测试成功，数据库可达'
  } catch (e) {
    testResult.value = '连接测试失败：' + errMsg(e, '未知错误')
  } finally {
    testing.value = false
  }
}

async function exec() {
  executing.value = true
  try {
    await post('/api/v1/set/db/switch', { ...form })
    // 标记数据库刚切换，下次加载时提示手动还原
    try { localStorage.setItem('remlink_db_switched', '1') } catch { /* ignore */ }
    ui.message.success('数据库切换成功，服务即将重启，页面将自动刷新')
    show.value = false
    emit('switched')
  } catch (e) {
    ui.message.error('数据库切换失败：' + errMsg(e, '未知错误'))
  } finally {
    executing.value = false
  }
}
</script>

<template>
  <NDrawer v-model:show="show" :width="640" :mask-closable="!executing">
    <NDrawerContent title="数据库切换向导" :closable="!executing">
      <NSteps :current="step + 1" size="small" class="mb-24px">
        <NStep title="配置连接" />
        <NStep title="测试连通" />
        <NStep title="数据迁移" />
        <NStep title="确认执行" />
      </NSteps>

      <!-- 0 配置连接 -->
      <div v-show="step === 0" class="grid gap-16px">
        <div class="rounded-10px bg-secondary px-16px py-12px text-md">
          <div class="mb-6px text-sm text-secondary">当前数据库（只读）</div>
          <div class="flex gap-8px"><span class="w-40px flex-shrink-0 text-secondary">类型</span><span class="rl-mono">{{ currentType || '-' }}</span></div>
          <div class="mt-4px flex gap-8px"><span class="w-40px flex-shrink-0 text-secondary">连接</span><span class="rl-mono break-all">{{ currentSource || '-' }}</span></div>
        </div>
        <div>
          <div class="mb-6px text-md font-semibold">新数据库类型</div>
          <NSelect v-model:value="form.db_type" :options="dbTypes" placeholder="选择数据库类型" />
        </div>
        <div>
          <div class="mb-6px text-md font-semibold">新连接字符串</div>
          <NInput v-model:value="form.db_source" placeholder="如 ./conf/remlink_new.db 或 user:pass@tcp(host:3306)/dbname" class="rl-mono" />
          <div class="mt-4px text-sm text-tertiary">SQLite 填写文件路径；MySQL / Postgres / MSSQL 填写 DSN 连接串</div>
        </div>
      </div>

      <!-- 1 测试连通 -->
      <div v-show="step === 1" class="grid gap-12px">
        <div
          v-if="testResult" class="flex items-start gap-8px rounded-8px bg-fill px-12px py-10px text-md"
          :class="testOk ? 'text-success' : 'text-error'"
        >
          <component :is="testOk ? CheckmarkCircle : ErrorCircle" class="mt-2px flex-shrink-0" />
          <span class="break-all">{{ testResult }}</span>
        </div>
        <NButton block :loading="testing" @click="testConn">{{ testing ? '连接测试中…' : '测试连接' }}</NButton>
        <div class="text-center text-sm text-tertiary">系统将使用新参数尝试连接数据库，不会修改任何配置</div>
      </div>

      <!-- 2 迁移方式 -->
      <div v-show="step === 2" class="grid gap-10px">
        <button
          v-for="m in migrations" :key="m.value" type="button" class="mig-opt"
          :class="form.migration === m.value && 'mig-opt--active'" @click="form.migration = m.value"
        >
          <span class="mig-opt__radio" />
          <span class="min-w-0 flex-1 text-left">
            <span class="flex items-center gap-6px text-md font-semibold">
              {{ m.title }}<span v-if="m.badge" class="ax-badge text-info">{{ m.badge }}</span>
            </span>
            <span class="mt-4px block text-sm leading-20px text-secondary">{{ m.desc }}</span>
          </span>
        </button>
      </div>

      <!-- 3 确认 -->
      <div v-show="step === 3" class="grid gap-16px">
        <div class="b-1px b-solid b-[var(--color-border-default)] rounded-10px px-16px py-4px text-md">
          <div class="flex gap-12px py-8px"><span class="w-100px flex-shrink-0 text-secondary">新数据库类型</span><span class="rl-mono">{{ form.db_type }}</span></div>
          <div class="flex gap-12px b-t-1px b-t-solid b-[var(--color-border-default)] py-8px"><span class="w-100px flex-shrink-0 text-secondary">新连接字符串</span><span class="rl-mono break-all">{{ form.db_source }}</span></div>
          <div class="flex gap-12px b-t-1px b-t-solid b-[var(--color-border-default)] py-8px">
            <span class="w-100px flex-shrink-0 text-secondary">数据迁移方式</span>
            <span class="ax-badge" :class="form.migration === 'none' ? 'text-error' : form.migration === 'backup_only' ? 'text-warning' : 'text-info'">{{ migrationLabel }}</span>
          </div>
        </div>
        <div class="rounded-8px bg-fill px-14px py-12px text-md">
          <div class="mb-6px flex items-center gap-6px font-semibold text-error"><Warning />重要提醒</div>
          <ul class="m-0 grid gap-4px pl-18px text-secondary">
            <li>切换后服务将<b class="text-primary">自动重启</b>，所有在线连接会中断</li>
            <li>请勿删除 <span class="rl-kbd">conf/db.json</span> 文件，否则重启后会回到默认 SQLite</li>
            <li>备份文件在 <span class="rl-kbd">conf/backup/</span> 目录，切换失败可手动处理</li>
          </ul>
        </div>
      </div>

      <template #footer>
        <div class="flex gap-sm">
          <NButton :disabled="executing" @click="show = false">取消</NButton>
          <NButton v-if="step > 0" :disabled="executing" @click="step--">上一步</NButton>
          <NButton v-if="step < 3" type="primary" :disabled="nextDisabled" @click="step++">{{ nextText }}</NButton>
          <NButton v-else type="primary" :loading="executing" @click="exec">确认切换</NButton>
        </div>
      </template>
    </NDrawerContent>
  </NDrawer>
</template>

<style scoped>
.mig-opt {
  display: flex; align-items: flex-start; gap: 12px; width: 100%; padding: 14px 16px;
  border: 1px solid var(--color-border-default); border-radius: 10px; background: var(--color-primary);
  color: inherit; font: inherit; cursor: pointer; transition: border-color .15s;
}
.mig-opt:hover { border-color: var(--color-border-strong); }
.mig-opt--active { border-color: var(--color-action); }
.mig-opt__radio {
  flex-shrink: 0; width: 16px; height: 16px; margin-top: 2px; border-radius: 50%;
  border: 1px solid var(--color-border-strong);
}
.mig-opt--active .mig-opt__radio { border: 5px solid var(--color-action); }
</style>
