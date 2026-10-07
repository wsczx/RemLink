<script setup lang="ts">
// 新建 / 编辑 IP 映射抽屉
import { ref, watch } from 'vue'
import {
  NButton, NDrawer, NDrawerContent, NForm, NFormItem, NInput, NSpin, NSwitch, type FormInst, type FormRules,
} from 'naive-ui'
import { errMsg, get, post } from '@/api/http'
import { ui } from '@/utils/ui'

const props = defineProps<{ mapId: number }>()
const show = defineModel<boolean>('show', { required: true })
const emit = defineEmits<{ saved: [] }>()

interface IpMapForm { id?: number; ip_addr: string; mac_addr: string; username: string; note: string; keep: boolean; [k: string]: any }
const blank = (): IpMapForm => ({ ip_addr: '', mac_addr: '', username: '', note: '', keep: false })

const form = ref<IpMapForm>(blank())
const formRef = ref<FormInst | null>(null)
const loading = ref(false)
const saving = ref(false)

const rules: FormRules = {
  ip_addr: [{ required: true, message: '请输入 IP 地址', trigger: 'blur' }],
  mac_addr: [{ required: true, message: '请输入 MAC 地址', trigger: 'blur' }],
  username: [{ max: 50, message: '长度不超过 50 个字符', trigger: 'blur' }],
}

watch(show, async v => {
  if (!v) return
  formRef.value?.restoreValidation()
  if (!props.mapId) {
    form.value = blank()
    return
  }
  loading.value = true
  try {
    // 保留详情里的其余字段（组 / IPv6 / 唯一 MAC 等）原样回传
    form.value = { ...blank(), ...(await get('/api/v1/user/ip_map/detail', { id: props.mapId })) }
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    loading.value = false
  }
})

async function submit() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  saving.value = true
  try {
    await post('/api/v1/user/ip_map/set', form.value)
    ui.message.success('保存成功')
    show.value = false
    emit('saved')
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <NDrawer v-model:show="show" :width="560" :mask-closable="false">
    <NDrawerContent :title="form.id ? '编辑 IP 映射' : '新建 IP 映射'" closable>
      <NSpin :show="loading">
        <NForm ref="formRef" :model="form" :rules="rules" label-placement="top" require-mark-placement="right-hanging">
          <NFormItem v-if="form.id" label="ID">
            <NInput :value="String(form.id)" disabled />
          </NFormItem>
          <NFormItem label="IP 地址" path="ip_addr">
            <NInput v-model:value="form.ip_addr" placeholder="如 192.168.1.100" class="rl-mono" />
          </NFormItem>
          <NFormItem label="MAC 地址" path="mac_addr">
            <NInput v-model:value="form.mac_addr" placeholder="如 00:11:22:33:44:55" class="rl-mono" />
          </NFormItem>
          <NFormItem label="绑定用户" path="username">
            <NInput v-model:value="form.username" placeholder="选填，绑定的用户名" />
          </NFormItem>
          <NFormItem label="备注" path="note">
            <NInput v-model:value="form.note" placeholder="备注信息" />
          </NFormItem>
          <NFormItem label="IP 保留" path="keep">
            <div class="flex items-center gap-10px">
              <NSwitch v-model:value="form.keep" />
              <span class="text-sm text-tertiary">开启后该 IP 将保留，不再分配给其他设备</span>
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
</template>
