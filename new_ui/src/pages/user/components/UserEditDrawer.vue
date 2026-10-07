<script setup lang="ts">
// 新建 / 编辑用户抽屉
import { computed, ref, watch } from 'vue'
import {
  NButton, NCheckbox, NCheckboxGroup, NDatePicker, NDivider, NDrawer, NDrawerContent, NForm, NFormItem,
  NInput, NInputNumber, NModal, NRadioButton, NRadioGroup, NSelect, NSpin, NSwitch, type FormInst, type FormRules,
} from 'naive-ui'
import KeyIcon from '~icons/fluent/key-20-regular'
import Copy from '~icons/fluent/copy-20-regular'
import dayjs from 'dayjs'
import { errMsg, get, post } from '@/api/http'
import { ui } from '@/utils/ui'
import { isExternalType, typeLabel } from './shared'

const props = defineProps<{ userId: number; groupNames: string[]; policyList: { id: number; name: string }[] }>()
const show = defineModel<boolean>('show', { required: true })
const emit = defineEmits<{ saved: [] }>()

interface UserForm {
  id: number
  type: string
  username: string
  nickname: string
  email: string
  phone: string
  pin_code: string
  limittime: number | null
  disable_otp: boolean
  otp_secret: string
  change_pwd: boolean
  send_email: boolean
  status: number
  mtu: number
  policy_id: number
  groups: string[]
  [k: string]: any
}

const defaultForm = (): UserForm => ({
  id: 0, type: 'local', username: '', nickname: '', email: '', phone: '', pin_code: '',
  limittime: null, disable_otp: false, otp_secret: '', change_pwd: true, send_email: true,
  status: 1, mtu: 0, policy_id: 0, groups: [],
})

const form = ref<UserForm>(defaultForm())
const formRef = ref<FormInst | null>(null)
const loading = ref(false)
const saving = ref(false)
const isExternal = computed(() => isExternalType(form.value.type))

const rules: FormRules = {
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' },
    { max: 50, message: '长度不超过 50 个字符', trigger: 'blur' },
  ],
  nickname: [{ required: true, message: '请输入用户姓名', trigger: 'blur' }],
  email: [
    { required: true, message: '请输入用户邮箱', trigger: 'blur' },
    { type: 'email', message: '请输入正确的邮箱地址', trigger: ['blur', 'input'] },
  ],
  pin_code: [{ min: 6, message: 'PIN 码至少 6 个字符', trigger: 'blur' }],
  groups: [{ type: 'array', required: true, message: '请至少选择一个组', trigger: 'change' }],
}

// 打开时加载：新建用默认值，编辑拉详情（不默认发邮件）
watch(show, async v => {
  if (!v) return
  formRef.value?.restoreValidation()
  if (!props.userId) {
    form.value = defaultForm()
    return
  }
  loading.value = true
  try {
    const d = await get('/api/v1/user/detail', { id: props.userId })
    form.value = {
      ...defaultForm(),
      ...d,
      groups: d.groups || [],
      send_email: false,
      limittime: d.limittime ? new Date(d.limittime).getTime() : null,
    }
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    loading.value = false
  }
})

const policyOptions = computed(() => [
  { label: '跟随组策略（默认）', value: 0 },
  ...props.policyList.map(p => ({ label: p.name, value: p.id })),
])

const otpEnabled = computed({
  get: () => !form.value.disable_otp,
  set: v => { form.value.disable_otp = !v },
})

// 系统生成的初始密码（仅显示一次）
const generated = ref<{ username: string; password: string; mailed: boolean } | null>(null)
async function copyPassword() {
  try {
    await navigator.clipboard.writeText(generated.value?.password || '')
    ui.message.success('已复制')
  } catch {
    ui.message.error('复制失败，请手动选择密码复制')
  }
}

const isPast = (ts: number) => ts < dayjs().startOf('day').valueOf()

async function submit() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  if (saving.value) return
  saving.value = true
  try {
    const f = form.value
    const payload = { ...f, limittime: f.limittime ? new Date(f.limittime).toISOString() : null }
    const d = await post('/api/v1/user/set', { ...payload, mtu: payload.mtu ?? 0 })
    show.value = false
    emit('saved')
    // 密码留空时由系统生成，只在这次返回明文，弹窗展示一次供复制
    if (d?.generated_password) {
      generated.value = { username: f.username, password: d.generated_password, mailed: f.send_email && !!f.email }
    } else {
      ui.message.success('保存成功')
    }
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <NDrawer v-model:show="show" :width="640" :mask-closable="false">
    <NDrawerContent :title="form.id ? '编辑用户' : '新建用户'" closable :native-scrollbar="false">
      <NSpin :show="loading">
        <NForm ref="formRef" :model="form" :rules="rules" label-placement="top" require-mark-placement="right-hanging">
          <div class="mb-12px text-xl font-semibold">基本信息</div>
          <div class="grid grid-cols-2 gap-x-16px">
            <NFormItem label="用户名" path="username">
              <NInput v-model:value="form.username" :disabled="form.id > 0" placeholder="登录用户名" />
            </NFormItem>
            <NFormItem label="认证类型">
              <NInput :value="typeLabel(form.type) + (form.id ? ` · ID ${form.id}` : '')" disabled />
            </NFormItem>
            <NFormItem label="姓名" path="nickname">
              <NInput v-model:value="form.nickname" :disabled="isExternal" placeholder="请输入姓名" />
            </NFormItem>
            <NFormItem label="密码" path="pin_code">
              <NInput v-model:value="form.pin_code" :disabled="isExternal" placeholder="留空由系统自动生成" />
            </NFormItem>
            <NFormItem label="邮箱" path="email">
              <NInput v-model:value="form.email" placeholder="name@example.com" />
            </NFormItem>
            <NFormItem label="手机号" path="phone">
              <NInput v-model:value="form.phone" placeholder="用于短信接收 OTP 验证码" />
            </NFormItem>
          </div>

          <NDivider class="my-8px!" />
          <div class="mb-12px text-xl font-semibold">安全与有效期</div>
          <NFormItem label="OTP 双因素认证" :show-feedback="false" class="mb-16px">
            <div class="w-full">
              <div class="flex items-center gap-10px">
                <NSwitch v-model:value="otpEnabled" />
                <NInput v-if="otpEnabled" v-model:value="form.otp_secret" placeholder="OTP 密钥，留空由系统自动生成" class="flex-1 rl-mono" />
                <span v-else class="text-md text-secondary">已禁用</span>
              </div>
              <div class="mt-4px text-sm text-tertiary">开启后，用户登录密码为「密码 + 动态码」</div>
            </div>
          </NFormItem>
          <NFormItem label="下次登录强制改密" :show-feedback="false" class="mb-16px">
            <div class="flex items-center gap-10px">
              <NSwitch v-model:value="form.change_pwd" :disabled="isExternal" />
              <span v-if="isExternal" class="text-sm text-tertiary">外部用户无需本地密码</span>
              <span v-else-if="form.change_pwd" class="text-sm text-warning">用户下次登录时必须先修改密码</span>
              <span v-else class="text-sm text-tertiary">仅本地用户生效</span>
            </div>
          </NFormItem>
          <div class="grid grid-cols-2 gap-x-16px">
            <NFormItem label="过期时间" path="limittime">
              <NDatePicker
                v-model:value="form.limittime" type="datetime" format="yyyy-MM-dd HH:mm" clearable
                placeholder="不过期" :is-date-disabled="isPast" :disabled="isExternal" class="w-full"
              />
            </NFormItem>
            <NFormItem label="MTU" path="mtu">
              <NInputNumber v-model:value="form.mtu" :min="0" :max="1500" class="w-full" placeholder="0 使用全局默认值">
                <template #suffix><span class="text-sm text-tertiary">0 = 全局默认</span></template>
              </NInputNumber>
            </NFormItem>
            <NFormItem label="状态" path="status">
              <NRadioGroup v-model:value="form.status" :disabled="isExternal">
                <NRadioButton :value="1">启用</NRadioButton>
                <NRadioButton :value="0">停用</NRadioButton>
              </NRadioGroup>
            </NFormItem>
            <NFormItem label="保存后发送账号邮件" path="send_email">
              <NSwitch v-model:value="form.send_email" />
            </NFormItem>
          </div>

          <NDivider class="my-8px!" />
          <div class="mb-12px text-xl font-semibold">用户组与策略</div>
          <NFormItem path="groups">
            <template #label>
              用户组 <span class="ml-4px text-sm text-tertiary">已选 {{ form.groups.length }} / {{ groupNames.length }}</span>
            </template>
            <div class="w-full rounded-8px b-1px b-solid b-[var(--color-border-default)] p-10px">
              <NCheckboxGroup v-if="groupNames.length" v-model:value="form.groups" :disabled="isExternal">
                <div class="grid max-h-180px grid-cols-3 gap-x-12px gap-y-8px overflow-y-auto">
                  <NCheckbox v-for="g in groupNames" :key="g" :value="g" :label="g" class="min-w-0" :title="g" />
                </div>
              </NCheckboxGroup>
              <div v-else class="py-10px text-center text-sm text-secondary">暂无可用用户组，请先创建</div>
            </div>
          </NFormItem>
          <NFormItem label="个人策略" path="policy_id">
            <div class="w-full">
              <NSelect v-model:value="form.policy_id" :options="policyOptions" filterable />
              <div class="mt-4px text-sm text-tertiary">选择后将覆盖用户组的策略；默认跟随组策略</div>
            </div>
          </NFormItem>
        </NForm>
      </NSpin>
      <template #footer>
        <div class="flex gap-sm">
          <NButton @click="show = false">取消</NButton>
          <NButton type="primary" :loading="saving" :disabled="loading" @click="submit">保存</NButton>
        </div>
      </template>
    </NDrawerContent>
  </NDrawer>

  <!-- 系统生成的初始密码：只显示这一次 -->
  <NModal :show="!!generated" :mask-closable="false" :close-on-esc="false">
    <div v-if="generated" class="rl-panel w-[calc(100vw-32px)] max-w-440px p-24px">
      <div class="mb-16px h-40px w-40px flex items-center justify-center rounded-10px bg-fill text-xl text-success"><KeyIcon /></div>
      <div class="text-xl font-semibold text-primary">用户 {{ generated.username }} 已创建</div>
      <p class="m-0 mt-6px text-md text-secondary">
        系统已生成初始密码，<strong class="text-primary">只显示这一次</strong>，关闭后无法再查看。{{ generated.mailed ? '账号邮件也已发送给该用户。' : '请复制后通过安全渠道告知用户。' }}
      </p>
      <div class="mt-16px flex items-center gap-8px rounded-10px bg-fill px-14px py-10px">
        <span class="rl-mono flex-1 select-all break-all text-lg text-primary">{{ generated.password }}</span>
        <button class="rl-icon-btn" title="复制" @click="copyPassword"><Copy /></button>
      </div>
      <NButton type="primary" block class="mt-20px" @click="generated = null">我已保存密码</NButton>
    </div>
  </NModal>
</template>
