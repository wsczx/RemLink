<script setup lang="ts">
// 启用加密向导：选择方式（生成新密钥 / 导入已有密钥）→ 确认或输入密钥 → 完成并确认已备份密钥文件
import { computed, ref, watch } from 'vue'
import { NButton, NCheckbox, NDrawer, NDrawerContent, NInput, NStep, NSteps } from 'naive-ui'
import KeyAdd from '~icons/fluent/key-multiple-20-regular'
import KeyImport from '~icons/fluent/arrow-import-20-regular'
import Copy from '~icons/fluent/copy-20-regular'
import CheckmarkCircle from '~icons/fluent/checkmark-circle-20-filled'
import Warning from '~icons/fluent/warning-20-filled'
import { errMsg, get, post } from '@/api/http'
import { ui } from '@/utils/ui'

const show = defineModel<boolean>('show', { required: true })
const emit = defineEmits<{ done: [] }>()

type Mode = 'generate' | 'import'
const step = ref(0)
const mode = ref<Mode>('generate')
const ack = ref(false)
const key = ref('')
const busy = ref(false)
const keyPath = ref('')
const encrypted = ref(0) // 本次加密的记录数
const backedUp = ref(false)

watch(show, v => {
  if (!v) return
  step.value = 0
  mode.value = 'generate'
  ack.value = backedUp.value = false
  key.value = keyPath.value = ''
  encrypted.value = 0
})

const modes = [
  { value: 'generate' as Mode, icon: KeyAdd, title: '生成新密钥', badge: '推荐', desc: '首次启用加密时选择。系统随机生成 AES-256 密钥，并保存到服务器工作目录。' },
  { value: 'import' as Mode, icon: KeyImport, title: '导入已有密钥', desc: '迁移服务器或恢复数据库时选择，需提供原服务器上的同一把密钥（64 位十六进制）。' },
]
const keyTrim = computed(() => key.value.trim())
const keyValid = computed(() => /^[0-9a-fA-F]{64}$/.test(keyTrim.value))
const keyError = computed(() => (keyTrim.value && !keyValid.value ? (keyTrim.value.length !== 64 ? `需要 64 个字符，当前 ${keyTrim.value.length} 个` : '只能包含 0-9、a-f') : ''))
const canRun = computed(() => (mode.value === 'generate' ? ack.value : keyValid.value && ack.value))

const sumStats = (s: any) => Object.values(s || {}).reduce((a: number, n: any) => a + (+n || 0), 0)

async function run() {
  busy.value = true
  try {
    if (mode.value === 'generate') {
      const d = await post('/api/v1/set/secret/enable')
      keyPath.value = d?.key_path || ''
      encrypted.value = sumStats(d?.stats)
    } else {
      // 导入接口直接返回统计 map
      encrypted.value = sumStats(await post('/api/v1/set/secret/upload', { key: keyTrim.value }))
      key.value = ''
      try { keyPath.value = (await get('/api/v1/set/secret/status'))?.key_path || '' } catch { /* 仅用于展示 */ }
    }
    step.value = 2
    emit('done')
    ui.message.success(mode.value === 'generate' ? '加密已启用，请立即备份密钥文件' : '密钥导入成功，加密已启用')
  } catch (e) {
    ui.message.error(errMsg(e, mode.value === 'generate' ? '启用加密失败' : '导入密钥失败'))
  } finally {
    busy.value = false
  }
}

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    ui.message.success('已复制')
  } catch {
    ui.message.warning('复制失败，请手动选择复制')
  }
}
// 完成页必须确认已备份才能关闭
const locked = computed(() => busy.value || step.value === 2)
</script>

<template>
  <NDrawer v-model:show="show" :width="600" :mask-closable="!locked" :close-on-esc="!locked">
    <NDrawerContent title="启用数据加密" :closable="!locked">
      <NSteps :current="step + 1" size="small" class="mb-24px">
        <NStep title="选择方式" />
        <NStep :title="mode === 'generate' ? '确认' : '输入密钥'" />
        <NStep title="备份密钥" />
      </NSteps>

      <!-- 0 选择方式 -->
      <div v-if="step === 0" class="grid gap-10px">
        <button
          v-for="m in modes" :key="m.value" type="button" class="enc-opt"
          :class="mode === m.value && 'enc-opt--on'" @click="mode = m.value"
        >
          <span class="enc-opt__icon"><component :is="m.icon" /></span>
          <span class="min-w-0 flex-1 text-left">
            <span class="flex items-center gap-6px text-md font-semibold">
              {{ m.title }}<span v-if="m.badge" class="ax-badge text-info">{{ m.badge }}</span>
            </span>
            <span class="mt-4px block text-sm leading-20px text-secondary">{{ m.desc }}</span>
          </span>
          <span class="enc-opt__radio" />
        </button>
      </div>

      <!-- 1 确认 / 输入密钥 -->
      <div v-else-if="step === 1" class="grid gap-16px">
        <template v-if="mode === 'import'">
          <div>
            <div class="mb-6px text-md font-semibold">密钥（十六进制）</div>
            <NInput
              v-model:value="key" type="textarea" :autosize="{ minRows: 2, maxRows: 3 }" :maxlength="64" show-count
              placeholder="a1b2c3d4…（共 64 个字符）" class="rl-mono" :status="keyError ? 'error' : undefined"
            />
            <div v-if="keyError" class="mt-4px text-sm text-error">{{ keyError }}</div>
            <div v-else class="mt-4px text-sm text-tertiary">即原服务器密钥文件中的内容。密钥不一致将无法解密已有数据。</div>
          </div>
        </template>
        <div class="enc-facts">
          <div class="text-md font-semibold">执行后会发生什么</div>
          <ul class="m-0 mt-8px grid gap-6px pl-18px text-md text-secondary">
            <li v-if="mode === 'generate'">生成一把随机的 <b class="text-primary">AES-256</b> 密钥，写入服务器工作目录（可用环境变量 <span class="rl-kbd">REMLINK_ENCRYPTION_KEY_DIR</span> 指定目录）</li>
            <li v-else>保存你提供的密钥，写入服务器工作目录</li>
            <li>立即加密数据库中已有的所有敏感字段（密码、密钥、证书等），之后写入的数据也会自动加密</li>
            <li>数据量较大时需要一些时间，期间请勿关闭页面</li>
          </ul>
        </div>
        <NCheckbox v-model:checked="ack">
          我了解：<b>密钥丢失后，加密数据将永久无法恢复</b>，我会在下一步备份密钥文件
        </NCheckbox>
      </div>

      <!-- 2 完成 -->
      <div v-else class="grid gap-16px">
        <div class="flex items-center gap-10px">
          <CheckmarkCircle class="text-[28px] text-success" />
          <div>
            <div class="text-lg font-semibold">加密已启用</div>
            <div class="text-sm text-secondary">数据库中的敏感字段现已以密文存储<template v-if="encrypted">，本次加密 {{ encrypted }} 条记录</template></div>
          </div>
        </div>
        <div class="enc-warn">
          <Warning class="mt-2px flex-shrink-0 text-error" />
          <div class="text-md">
            <div class="font-semibold">现在就把密钥文件复制到安全的位置</div>
            <div class="mt-2px text-secondary">迁移服务器或密钥文件损坏时，必须使用同一把密钥才能解密数据。不要只把它留在这台服务器上。</div>
          </div>
        </div>
        <div>
          <div class="mb-6px text-sm text-secondary">密钥文件路径</div>
          <div class="flex items-center gap-xs rounded-8px bg-fill py-4px pl-12px pr-4px">
            <code class="rl-mono min-w-0 flex-1 select-all break-all">{{ keyPath || '—' }}</code>
            <button v-if="keyPath" class="rl-icon-btn flex-shrink-0" title="复制路径" @click="copy(keyPath)"><Copy /></button>
          </div>
        </div>
        <NCheckbox v-model:checked="backedUp">我已将密钥文件备份到服务器以外的安全位置</NCheckbox>
      </div>

      <template #footer>
        <div class="flex gap-sm">
          <template v-if="step < 2">
            <NButton :disabled="busy" @click="show = false">取消</NButton>
            <NButton v-if="step === 1" :disabled="busy" @click="step = 0">上一步</NButton>
            <NButton v-if="step === 0" type="primary" @click="step = 1">下一步</NButton>
            <NButton v-else type="primary" :loading="busy" :disabled="!canRun" @click="run">
              {{ mode === 'generate' ? '生成密钥并启用' : '导入并启用' }}
            </NButton>
          </template>
          <NButton v-else type="primary" :disabled="!backedUp" @click="show = false">完成</NButton>
        </div>
      </template>
    </NDrawerContent>
  </NDrawer>
</template>

<style scoped>
.enc-opt {
  display: flex; align-items: flex-start; gap: 12px; width: 100%; padding: 14px 16px;
  border: 1px solid var(--color-border-default); border-radius: 10px; background: var(--color-primary);
  color: inherit; font: inherit; cursor: pointer; transition: border-color .15s;
}
.enc-opt:hover { border-color: var(--color-border-strong); }
.enc-opt--on { border-color: var(--color-action); box-shadow: 0 0 0 1px var(--color-action); }
.enc-opt__icon {
  display: flex; flex-shrink: 0; align-items: center; justify-content: center; width: 36px; height: 36px;
  border-radius: 10px; background: var(--color-fill); font-size: 20px; color: var(--color-text-secondary);
}
.enc-opt__radio { flex-shrink: 0; width: 16px; height: 16px; margin-top: 2px; border-radius: 50%; border: 1px solid var(--color-border-strong); }
.enc-opt--on .enc-opt__radio { border: 5px solid var(--color-action); }
.enc-facts { padding: 14px 16px; border-radius: 10px; background: var(--color-secondary); }
.enc-warn { display: flex; gap: 10px; padding: 12px 14px; border-radius: 10px; border: 1px solid var(--color-border-strong); }
</style>
