<script setup lang="ts">
// 认证源详情：概览 / 连接配置 / 操作 / 危险区；id=new 时进入新建向导
import { computed, ref, watch } from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import { NButton, NSpin } from 'naive-ui'
import ArrowLeft from '~icons/fluent/arrow-left-20-regular'
import Warning from '~icons/fluent/warning-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import StatusDot from '@/components/StatusDot.vue'
import EmptyState from '@/components/EmptyState.vue'
import { get, post, errMsg } from '@/api/http'
import { confirm, ui } from '@/utils/ui'
import { formatDate } from '@/utils/format'
import CreateWizard from './components/CreateWizard.vue'
import ConfigFields from './components/ConfigFields.vue'
import CallbackList from './components/CallbackList.vue'
import TestLoginCard from './components/TestLoginCard.vue'
import SyncUsersCard from './components/SyncUsersCard.vue'
import { loadAllGroups } from './components/api'
import {
  connSummary, defaultConfig, groupRefs, isOAuth, isSyncable, isTestable, parseCfg, typeMeta,
  type GroupRef, type ProviderRow,
} from './components/types'

const route = useRoute()
const router = useRouter()

const isNew = computed(() => route.params.id === 'new')
const newType = computed(() => {
  const t = String(route.query.type || '')
  return typeMeta(t) ? t : ''
})

const loading = ref(false)
const notFound = ref(false)
const saving = ref(false)
const toggling = ref(false)
const p = ref<ProviderRow | null>(null)
const cfg = ref<Record<string, any>>({})
const snapshot = ref('')
const callbackBase = ref('')
const refs = ref<GroupRef[]>([])

const meta = computed(() => typeMeta(p.value?.type || ''))
const savedCfg = computed<Record<string, any>>(() => (snapshot.value ? JSON.parse(snapshot.value) : {}))
const dirty = computed(() => !!p.value && JSON.stringify(cfg.value) !== snapshot.value)
const active = computed(() => p.value?.status === 1)

async function load() {
  const id = Number(route.params.id)
  if (!id) { notFound.value = true; return }
  loading.value = true
  notFound.value = false
  try {
    const [d, groups] = await Promise.all([get('/api/v1/provider/detail', { id }), loadAllGroups()])
    const row: ProviderRow = d?.provider || d
    p.value = row
    const c = { ...defaultConfig(row.type), ...parseCfg(row.config) }
    cfg.value = c
    snapshot.value = JSON.stringify(c)
    callbackBase.value = d?.callbackBase || ''
    refs.value = groupRefs(groups)[row.name] || []
  } catch (e) {
    notFound.value = true
    ui.message.error(errMsg(e, '获取详情失败'))
  } finally {
    loading.value = false
  }
}

watch(() => route.params.id, () => { if (!isNew.value && route.path.startsWith('/admin/provider/')) load() }, { immediate: true })

function reset() { cfg.value = JSON.parse(snapshot.value) }

async function setProvider(status: number, config: Record<string, any>) {
  const v = p.value!
  await post('/api/v1/provider/set', { id: v.id, name: v.name, type: v.type, status, config })
}

async function save() {
  if (!p.value) return
  saving.value = true
  try {
    await setProvider(p.value.status, cfg.value)
    ui.message.success('保存成功')
    await load()
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    saving.value = false
  }
}

// 启用 / 停用：用已保存的配置提交（脱敏占位符由后端保留原值），不带上未保存的修改
async function toggleStatus() {
  if (!p.value) return
  const target = active.value ? 0 : 1
  if (target === 0 && !(await confirm(
    `停用后，引用「${p.value.name}」的用户组将无法通过它登录，已在线的会话不受影响。`, '停用认证源',
  ))) return
  toggling.value = true
  try {
    await setProvider(target, savedCfg.value)
    await load()
    if (p.value?.status !== target) ui.message.warning('状态未变更：服务端未接受该状态')
    else ui.message.success(target ? '已启用' : '已停用')
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    toggling.value = false
  }
}

async function remove() {
  if (!p.value) return
  if (!(await confirm(`确定要删除认证源「${p.value.name}」吗？此操作不可恢复。`, '删除认证源'))) return
  try {
    await post('/api/v1/provider/del', null, { params: { id: p.value.id } })
    ui.message.success('删除成功')
    snapshot.value = JSON.stringify(cfg.value)
    router.replace('/admin/provider/list')
  } catch (e) {
    ui.message.error(errMsg(e))
  }
}

onBeforeRouteLeave(async () => {
  if (!dirty.value) return true
  return confirm('连接配置有未保存的修改，离开后将丢失。', '放弃修改？')
})

const facts = computed(() => {
  const v = p.value
  if (!v) return []
  const c = savedCfg.value
  const list: { label: string; value: string; mono?: boolean }[] = [
    { label: '类型', value: meta.value?.full || v.type },
    { label: '连接', value: connSummary(v.type, c) || '—', mono: true },
  ]
  if (v.type === 'ldap') list.push({ label: 'OTP', value: c.enable_otp ? '同步时生成密钥' : '未启用' })
  if (isSyncable(v.type)) list.push({ label: '自动同步用户', value: c.sync_users ? '已开启' : '未开启' })
  if (isOAuth(v.type)) list.push({ label: '登录限制', value: c.allowed_departments ? `${String(c.allowed_departments).split(',').filter(Boolean).length} 个部门` : '不限部门' })
  list.push({ label: '创建时间', value: formatDate(v.created_at) || '—' })
  list.push({ label: '最近更新', value: formatDate(v.updated_at) || '—' })
  return list
})

const testDisabled = computed(() => (active.value ? '' : '认证源已停用，启用后才能测试登录'))
</script>

<template>
  <CreateWizard v-if="isNew && newType" :key="newType" :type="newType" />

  <div v-else-if="isNew">
    <EmptyState title="未指定认证源类型" description="请从认证源页面选择要接入的类型。">
      <NButton @click="router.push({ path: '/admin/provider/list', hash: '#pv-add' })">选择类型</NButton>
    </EmptyState>
  </div>

  <div v-else class="pb-24px">
    <button type="button" class="pv-back" @click="router.push('/admin/provider/list')"><ArrowLeft />认证源</button>

    <EmptyState v-if="notFound && !loading" title="认证源不存在" description="它可能已被删除。">
      <NButton @click="router.push('/admin/provider/list')">返回认证源</NButton>
    </EmptyState>

    <NSpin v-else :show="loading">
      <template v-if="p">
        <PageHeader class="pt-8px!" :title="p.name" :description="meta?.desc">
          <template #meta>
            <span class="pv-tile"><component :is="meta?.icon" /></span>
            <StatusDot :tone="active ? 'success' : 'muted'" :pulse="active">{{ active ? '启用' : '停用' }}</StatusDot>
          </template>
        </PageHeader>

        <div class="grid gap-32px">
          <!-- 概览 -->
          <section class="grid gap-16px">
            <div
              v-if="!active || !refs.length"
              class="flex flex-wrap items-center gap-10px rounded-10px px-16px py-12px pv-alert"
            >
              <Warning class="text-lg text-warning" />
              <div class="min-w-0 flex-1 text-md">
                <template v-if="!active">已停用：用户暂时无法通过该认证源登录。</template>
                <template v-else>还没有用户组在认证流程中引用它，用户暂时无法通过它登录。</template>
              </div>
              <NButton v-if="active" size="small" @click="router.push('/admin/group/list')">配置用户组</NButton>
              <NButton v-else size="small" :loading="toggling" @click="toggleStatus">启用</NButton>
            </div>

            <RlPanel title="概览" :padded="false">
              <dl class="pv-facts">
                <div v-for="f in facts" :key="f.label" class="pv-fact">
                  <dt>{{ f.label }}</dt>
                  <dd :class="f.mono && 'rl-mono'">{{ f.value }}</dd>
                </div>
                <div class="pv-fact pv-fact--wide">
                  <dt>用于用户组</dt>
                  <dd>
                    <span v-if="!refs.length" class="text-tertiary">未被引用</span>
                    <span v-else class="flex flex-wrap gap-6px">
                      <span v-for="g in refs" :key="g.id" class="rl-kbd" :class="g.status !== 1 && 'line-through'" :title="g.status !== 1 ? '该用户组已停用' : ''">{{ g.name }}</span>
                    </span>
                  </dd>
                </div>
              </dl>
              <div v-if="isOAuth(p.type)" class="pv-callbacks">
                <div class="mb-8px text-md font-semibold">回调地址</div>
                <div class="mb-10px text-sm text-secondary">在开放平台中填写以下地址，用户扫码后跳回 VPN 完成登录。</div>
                <CallbackList :type="p.type" :base="callbackBase" />
              </div>
            </RlPanel>
          </section>

          <!-- 连接配置 -->
          <section>
            <div class="mb-12px">
              <h2 class="pv-h2">连接配置</h2>
              <p class="m-0 mt-4px text-md text-secondary">名称与类型创建后不可修改。密钥类字段显示为掩码，不改动则保留原值。</p>
            </div>
            <ConfigFields v-model:cfg="cfg" :type="p.type" />
            <div class="pv-savebar" :class="dirty && 'pv-savebar--dirty'">
              <span class="text-md" :class="dirty ? 'font-semibold text-primary' : 'text-tertiary'">
                {{ dirty ? '连接配置有未保存的修改' : '没有未保存的修改' }}
              </span>
              <div class="flex-1" />
              <NButton :disabled="!dirty || saving" @click="reset">重置</NButton>
              <NButton type="primary" :disabled="!dirty" :loading="saving" @click="save">保存</NButton>
            </div>
          </section>

          <!-- 操作 -->
          <section v-if="isTestable(p.type) || isSyncable(p.type)">
            <div class="mb-12px">
              <h2 class="pv-h2">操作</h2>
              <p class="m-0 mt-4px text-md text-secondary">
                使用已保存的配置执行<span v-if="dirty" class="text-warning">（当前有未保存的修改，不会生效）</span>。
              </p>
            </div>
            <div class="grid grid-cols-1 gap-16px lg:grid-cols-2">
              <RlPanel v-if="isTestable(p.type)" title="测试登录" subtitle="用一个真实账号验证连接、查找与密码校验">
                <TestLoginCard :provider-id="p.id" :disabled-reason="testDisabled" />
              </RlPanel>
              <RlPanel v-if="isSyncable(p.type)" title="同步用户" :subtitle="`把${meta?.label}中的用户导入到本地用户组`">
                <SyncUsersCard :provider-id="p.id" :refs="refs" />
              </RlPanel>
            </div>
          </section>

          <!-- 危险区 -->
          <RlPanel :padded="false">
            <template #header><div class="text-lg font-semibold text-error">危险区</div></template>
            <div class="pv-danger">
              <div class="min-w-0 flex-1">
                <div class="text-md font-semibold">{{ active ? '停用认证源' : '启用认证源' }}</div>
                <div class="mt-2px text-sm text-secondary">
                  {{ active
                    ? '停用后，引用它的用户组无法再通过它登录，也无法测试登录；配置会保留，可随时重新启用。'
                    : '启用后，引用它的用户组即可通过它登录。' }}
                </div>
              </div>
              <NButton :type="active ? 'warning' : 'default'" secondary :loading="toggling" @click="toggleStatus">
                {{ active ? '停用' : '启用' }}
              </NButton>
            </div>
            <div class="pv-danger">
              <div class="min-w-0 flex-1">
                <div class="text-md font-semibold">删除认证源</div>
                <div class="mt-2px text-sm text-secondary">
                  永久删除该认证源及其配置，不可恢复。已同步到本地的用户不会被删除。
                  <template v-if="refs.length">
                    <br><span class="text-warning">正被 {{ refs.length }} 个用户组引用（{{ refs.map(g => g.name).join('、') }}），需先在这些组的认证流程中移除它。</span>
                  </template>
                </div>
              </div>
              <NButton type="error" secondary :disabled="refs.length > 0" @click="remove">删除</NButton>
            </div>
          </RlPanel>
        </div>
      </template>
      <div v-else class="h-320px" />
    </NSpin>
  </div>
</template>

<style scoped>
.pv-back {
  display: inline-flex; align-items: center; gap: 4px; margin-top: 20px; padding: 0; border: 0; background: none;
  font: inherit; font-size: 13px; color: var(--color-text-secondary); cursor: pointer;
}
.pv-back:hover { color: var(--color-text-primary); }
.pv-tile {
  display: inline-flex; align-items: center; justify-content: center; width: 28px; height: 28px;
  border-radius: 8px; background: var(--color-fill); font-size: 16px; color: var(--color-text-primary);
}
.pv-h2 { margin: 0; font-size: 18px; font-weight: 600; color: var(--color-text-primary); }
.pv-alert { background: color-mix(in srgb, var(--color-text-warning) 10%, transparent); }

.pv-facts { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); margin: 0; }
.pv-fact { min-width: 0; padding: 14px 20px; border-bottom: 1px solid var(--color-border-default); }
.pv-fact--wide { grid-column: 1 / -1; border-bottom: 0; }
.pv-fact dt { font-size: 12px; color: var(--color-text-tertiary); }
.pv-fact dd { margin: 4px 0 0; font-size: 14px; color: var(--color-text-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.pv-fact--wide dd { white-space: normal; }
.pv-callbacks { padding: 16px 20px 20px; border-top: 1px solid var(--color-border-default); }

.pv-savebar {
  position: sticky; bottom: 0; z-index: 10; display: flex; align-items: center; gap: 10px;
  margin-top: 16px; padding: 12px 0; background: var(--color-body);
  border-top: 1px solid var(--color-border-default);
}
.pv-savebar--dirty { border-top-color: var(--color-border-strong); }

.pv-danger { display: flex; flex-wrap: wrap; align-items: center; gap: 12px 24px; padding: 16px 20px; }
.pv-danger + .pv-danger { border-top: 1px solid var(--color-border-default); }
</style>
