// 「通知与品牌」各设置页共用的表单状态：加载、快照比对（脏检查）、保存、重置、离开前确认
import { computed, onMounted, ref, type Ref } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { errMsg } from '@/api/http'
import { confirm, ui } from '@/utils/ui'

interface Options<T> {
  fetch: () => Promise<T>
  submit: (v: T) => Promise<unknown>
  validate?: (v: T) => string | null
  successText?: string
}

export function useSettingForm<T extends object>(initial: () => T, opts: Options<T>) {
  const form = ref(initial()) as Ref<T>
  const snap = ref('')
  const loading = ref(false)
  const saving = ref(false)
  const loaded = ref(false)

  const dirty = computed(() => loaded.value && JSON.stringify(form.value) !== snap.value)
  // 已保存（服务端）状态，供顶部状态卡展示，避免编辑中的值误导
  const saved = computed<T>(() => (snap.value ? JSON.parse(snap.value) : initial()))

  // 把当前表单记为已保存
  const commit = () => { snap.value = JSON.stringify(form.value) }

  async function load() {
    loading.value = true
    try {
      form.value = await opts.fetch()
      commit()
      loaded.value = true
    } catch (e) {
      ui.message.error(errMsg(e))
    } finally {
      loading.value = false
    }
  }

  async function save() {
    const err = opts.validate?.(form.value)
    if (err) {
      ui.message.warning(err)
      return false
    }
    saving.value = true
    try {
      await opts.submit(form.value)
      commit()
      ui.message.success(opts.successText || '已保存')
      return true
    } catch (e) {
      ui.message.error(errMsg(e, '保存失败'))
      return false
    } finally {
      saving.value = false
    }
  }

  function reset() {
    if (snap.value) form.value = JSON.parse(snap.value)
  }

  onBeforeRouteLeave(async () => (dirty.value ? confirm('当前页面有未保存的修改，离开后将丢失。', '放弃修改？') : true))
  onMounted(load)

  return { form, saved, dirty, loading, saving, loaded, load, save, reset, commit }
}

export function parseJSON<T>(s: unknown, fallback: T): T {
  if (!s || typeof s !== 'string') return fallback
  try { return JSON.parse(s) ?? fallback } catch { return fallback }
}
