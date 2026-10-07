<script setup lang="ts">
// 为单个用户签发客户端证书（服务端生成 P12 / 上传 CSR 签发）
import { computed, ref, watch } from 'vue'
import { NAlert, NButton, NDrawer, NDrawerContent, NForm, NFormItem, NInput, NInputNumber, NSelect, NSwitch } from 'naive-ui'
import Checkmark from '~icons/fluent/checkmark-circle-20-filled'
import { errMsg, post } from '@/api/http'
import { ui } from '@/utils/ui'
import { type CertUser, checkCertAuth, userLabel } from './client'

const props = defineProps<{ users: CertUser[] }>()
const show = defineModel<boolean>('show', { required: true })
const emit = defineEmits<{ done: [] }>()

const blank = () => ({ username: '', groupName: '', maxDevices: 3, deviceBindingEnabled: false, generateType: 'server' as 'server' | 'csr', csrData: '' })
const form = ref(blank())
const saving = ref(false)

const userOptions = computed(() => props.users.map(u => ({ label: userLabel(u.username, u.nickname), value: u.username })))
const userGroups = computed(() => props.users.find(u => u.username === form.value.username)?.groups || [])

watch(show, v => { if (v) form.value = blank() })

function onUser() {
  form.value.groupName = userGroups.value.length === 1 ? userGroups.value[0] : ''
  if (form.value.groupName) checkGroup(form.value.groupName)
}

const MODES = [
  { value: 'server' as const, title: '服务端生成', desc: '生成 P12（含私钥），下载或邮件发给用户' },
  { value: 'csr' as const, title: '用户提交 CSR', desc: '用户自行保管私钥，仅签发证书（PEM）' },
]

const checkGroup = (g: string) => checkCertAuth(g, '建议先在「用户组 > 编辑 > 认证方式」中添加 TLS 证书认证步骤')

async function submit() {
  const f = form.value
  if (!f.username) return ui.message.error('请选择或输入用户名')
  if (userGroups.value.length && !f.groupName) return ui.message.error('请选择用户组')
  if (f.generateType === 'csr' && !f.csrData.trim()) return ui.message.error('请粘贴 CSR 内容')
  const fd = new FormData()
  fd.append('username', f.username)
  if (f.groupName) fd.append('group_name', f.groupName)
  fd.append('device_binding_enabled', String(f.deviceBindingEnabled))
  fd.append('max_devices', String(f.maxDevices))
  if (f.generateType === 'csr') fd.append('csr', f.csrData)
  saving.value = true
  try {
    await post('/api/v1/set/client_cert/generate', fd)
    ui.message.success('证书生成成功')
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
  <NDrawer v-model:show="show" :width="560">
    <NDrawerContent title="签发客户端证书" closable>
      <NForm label-placement="top" :show-feedback="false" class="flex flex-col gap-16px">
        <NFormItem label="用户名" required>
          <NSelect v-model:value="form.username" :options="userOptions" filterable tag clearable placeholder="输入或选择用户名" @update:value="onUser" />
        </NFormItem>
        <NFormItem v-if="userGroups.length" label="用户组" required>
          <NSelect v-model:value="form.groupName" :options="userGroups.map(g => ({ label: g, value: g }))" placeholder="选择用户组" @update:value="checkGroup" />
        </NFormItem>
        <NFormItem label="私钥由谁生成">
          <div class="grid w-full gap-8px sm:grid-cols-2">
            <button
              v-for="m in MODES" :key="m.value" type="button"
              class="gen-opt rounded-10px bg-primary p-12px text-left" :class="form.generateType === m.value && 'gen-opt--on'"
              @click="form.generateType = m.value"
            >
              <div class="flex items-center justify-between gap-6px">
                <span class="text-md font-semibold">{{ m.title }}</span>
                <Checkmark v-if="form.generateType === m.value" class="text-lg" />
              </div>
              <div class="mt-2px text-sm text-secondary">{{ m.desc }}</div>
            </button>
          </div>
        </NFormItem>
        <NFormItem v-if="form.generateType === 'csr'" label="CSR 内容" required>
          <NInput v-model:value="form.csrData" type="textarea" :rows="8" placeholder="粘贴 PEM 格式的 CSR（-----BEGIN CERTIFICATE REQUEST-----）" class="rl-mono" />
        </NFormItem>
        <div class="grid gap-12px rounded-10px bg-fill p-14px">
          <div class="flex items-center justify-between gap-16px">
            <div>
              <div class="text-md font-medium">设备绑定</div>
              <div class="text-sm text-secondary">证书首次使用时绑定设备，超出上限的设备无法登录</div>
            </div>
            <NSwitch v-model:value="form.deviceBindingEnabled" />
          </div>
          <NAlert v-if="form.deviceBindingEnabled" type="warning" :bordered="false">开启后仅 Cisco AnyConnect 客户端支持证书认证，后续可手动关闭。</NAlert>
          <div class="flex items-center justify-between gap-16px">
            <div>
              <div class="text-md font-medium">最大设备数</div>
              <div class="text-sm text-secondary">该证书允许绑定的最大设备数</div>
            </div>
            <NInputNumber v-model:value="form.maxDevices" :min="1" :max="10" class="w-120px" />
          </div>
        </div>
      </NForm>
      <template #footer>
        <div class="flex justify-end gap-sm">
          <NButton @click="show = false">取消</NButton>
          <NButton type="primary" :loading="saving" @click="submit">生成</NButton>
        </div>
      </template>
    </NDrawerContent>
  </NDrawer>
</template>

<style scoped>
.gen-opt { border: 1px solid var(--color-border-default); cursor: pointer; font: inherit; color: inherit; transition: border-color .15s; }
.gen-opt:hover { border-color: var(--color-border-strong); }
.gen-opt--on { border-color: var(--color-text-primary); box-shadow: inset 0 0 0 1px var(--color-text-primary); }
</style>
