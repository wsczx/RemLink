<script setup lang="ts">
// 按用户组批量生成客户端证书
import { computed, ref, watch } from 'vue'
import { NAlert, NButton, NCheckbox, NCheckboxGroup, NDrawer, NDrawerContent, NForm, NFormItem, NInput, NInputNumber, NSelect, NSwitch } from 'naive-ui'
import Search from '~icons/fluent/search-20-regular'
import { errMsg, post } from '@/api/http'
import { ui } from '@/utils/ui'
import { type CertUser, checkCertAuth, userLabel } from './client'

const props = defineProps<{ users: CertUser[]; groups: string[] }>()
const show = defineModel<boolean>('show', { required: true })
const emit = defineEmits<{ done: [] }>()

const blank = () => ({ groupName: '', usernames: [] as string[], allUsers: false, maxDevices: 3, deviceBindingEnabled: false })
const form = ref(blank())
const kw = ref('')
const saving = ref(false)

watch(show, v => { if (v) { form.value = blank(); kw.value = '' } })

const groupUsers = computed(() => (form.value.groupName ? props.users.filter(u => (u.groups || u.Groups || []).includes(form.value.groupName)) : []))
const filtered = computed(() => {
  const s = kw.value.trim().toLowerCase()
  if (!s) return groupUsers.value
  return groupUsers.value.filter(u => u.username?.toLowerCase().includes(s) || u.nickname?.toLowerCase().includes(s))
})
// 未勾选具体用户 = 全组
const targetCount = computed(() => (form.value.allUsers || !form.value.usernames.length ? groupUsers.value.length : form.value.usernames.length))

function onGroup(g: string) {
  form.value.usernames = []
  form.value.allUsers = false
  kw.value = ''
  checkCertAuth(g, '建议先配置 TLS 证书认证步骤')
}

async function submit() {
  const f = form.value
  if (!f.groupName) return ui.message.error('请选择目标用户组')
  const usernames = f.allUsers || !f.usernames.length ? groupUsers.value.map(u => u.username) : f.usernames
  if (!usernames.length) return ui.message.error('该组下没有可生成证书的用户，请确认用户已加入该组')
  const fd = new FormData()
  fd.append('usernames', usernames.join('\n'))
  fd.append('group_name', f.groupName)
  fd.append('device_binding_enabled', String(f.deviceBindingEnabled))
  fd.append('max_devices', String(f.maxDevices))
  saving.value = true
  try {
    const d = await post('/api/v1/set/client_cert/batch_generate', fd)
    const failed: string[] = d?.failed || []
    let msg = `成功生成 ${d?.success || 0} 个证书`
    if (failed.length) msg += `，失败 ${failed.length} 个：${failed.join('；')}`
    ui.message[d?.success ? 'success' : 'warning'](msg, { duration: 5000 })
    show.value = false
    emit('done')
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <NDrawer v-model:show="show" :width="600">
    <NDrawerContent title="按用户组批量签发" closable>
      <NForm label-placement="top" :show-feedback="false" class="flex flex-col gap-16px">
        <NFormItem label="用户组" required>
          <div class="w-full">
            <NSelect v-model:value="form.groupName" :options="groups.map(g => ({ label: g, value: g }))" filterable placeholder="选择目标用户组" @update:value="onGroup" />
            <div class="mt-4px text-sm text-secondary">所选组下的用户需已存在，非该组成员将被自动跳过。</div>
          </div>
        </NFormItem>
        <NFormItem label="用户">
          <div class="w-full">
            <div class="flex items-center justify-between gap-sm">
              <NCheckbox v-model:checked="form.allUsers" @update:checked="v => v && (form.usernames = [])">为该组全部用户生成</NCheckbox>
              <span class="text-sm text-secondary">将生成 {{ targetCount }} 个</span>
            </div>
            <div class="mt-sm overflow-hidden rounded-8px b-1px b-solid b-[var(--color-border-default)]">
              <div class="b-b-1px b-b-solid b-[var(--color-border-default)] p-8px">
                <NInput v-model:value="kw" size="small" clearable placeholder="搜索用户名 / 昵称" :disabled="form.allUsers || !form.groupName">
                  <template #prefix><Search class="text-secondary" /></template>
                </NInput>
              </div>
              <div class="max-h-280px overflow-auto px-12px py-8px">
                <div v-if="!form.groupName" class="py-md text-center text-sm text-tertiary">请先选择用户组</div>
                <div v-else-if="form.allUsers" class="py-md text-center text-sm text-secondary">将为「{{ form.groupName }}」下全部用户生成</div>
                <NCheckboxGroup v-else-if="filtered.length" v-model:value="form.usernames">
                  <div class="flex flex-col gap-6px">
                    <NCheckbox v-for="u in filtered" :key="u.username" :value="u.username" :label="userLabel(u.username, u.nickname)" />
                  </div>
                </NCheckboxGroup>
                <div v-else class="py-md text-center text-sm text-tertiary">该组下暂无用户，或没有匹配的用户</div>
              </div>
            </div>
            <div class="mt-4px text-sm text-secondary">不勾选任何用户时，默认为该组全部用户生成。</div>
          </div>
        </NFormItem>
        <div class="grid gap-12px rounded-10px bg-fill p-14px">
          <div class="flex items-center justify-between gap-16px">
            <div>
              <div class="text-md font-medium">设备绑定</div>
              <div class="text-sm text-secondary">证书首次使用时绑定设备</div>
            </div>
            <NSwitch v-model:value="form.deviceBindingEnabled" />
          </div>
          <NAlert v-if="form.deviceBindingEnabled" type="warning" :bordered="false">开启后仅 Cisco AnyConnect 客户端支持证书认证。</NAlert>
          <div class="flex items-center justify-between gap-16px">
            <div class="text-md font-medium">最大设备数</div>
            <NInputNumber v-model:value="form.maxDevices" :min="1" :max="10" class="w-120px" />
          </div>
        </div>
      </NForm>
      <template #footer>
        <div class="flex justify-end gap-sm">
          <NButton @click="show = false">取消</NButton>
          <NButton type="primary" :loading="saving" @click="submit">签发 {{ targetCount }} 个证书</NButton>
        </div>
      </template>
    </NDrawerContent>
  </NDrawer>
</template>
