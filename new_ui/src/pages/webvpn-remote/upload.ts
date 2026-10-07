// 上传队列：分片续传（stat 取已传大小 → offset 续传）、并发、进度/速度、失败重试、取消
import { computed, reactive } from 'vue'
import { RemoteError, friendlyError, joinPath, remoteApi } from './api'

export const CHUNK_SIZE = 8 << 20 // 8MB 分片
const CONCURRENCY = 2

export type TaskStatus = 'pending' | 'uploading' | 'done' | 'error' | 'canceled'

export interface UploadTask {
  id: number
  file: File
  dir: string      // 上传时所在目录（相对根）
  relpath: string  // 相对 dir 的路径，可含子目录（文件夹上传）
  status: TaskStatus
  loaded: number   // 已在远端的字节数
  speed: number    // B/s
  error: string
  errorCode: string
  resumed: boolean // 是否从已有部分续传
  overwrite: boolean
  xhr?: XMLHttpRequest
}

export interface PendingFile { file: File; relpath: string }

let seq = 0

export function useUploader(opts: { sid: () => string; maxSize: () => number; onFileDone: (dir: string) => void }) {
  const tasks = reactive<UploadTask[]>([])

  const active = computed(() => tasks.filter(t => t.status === 'pending' || t.status === 'uploading'))
  const summary = computed(() => {
    const total = tasks.reduce((s, t) => s + t.file.size, 0)
    const loaded = tasks.reduce((s, t) => s + (t.status === 'done' ? t.file.size : t.loaded), 0)
    const speed = tasks.reduce((s, t) => s + (t.status === 'uploading' ? t.speed : 0), 0)
    return {
      total, loaded, speed,
      done: tasks.filter(t => t.status === 'done').length,
      failed: tasks.filter(t => t.status === 'error').length,
      running: active.value.length,
    }
  })

  function add(dir: string, files: PendingFile[]) {
    for (const f of files) {
      tasks.push({
        id: ++seq, file: f.file, dir, relpath: f.relpath || f.file.name, status: 'pending',
        loaded: 0, speed: 0, error: '', errorCode: '', resumed: false, overwrite: false,
      })
    }
    pump()
  }

  function pump() {
    let running = tasks.filter(t => t.status === 'uploading').length
    for (const t of tasks) {
      if (running >= CONCURRENCY) break
      if (t.status !== 'pending') continue
      running++
      run(t)
    }
  }

  async function run(t: UploadTask) {
    t.status = 'uploading'
    t.error = ''
    t.errorCode = ''
    t.speed = 0
    try {
      const max = opts.maxSize()
      if (max > 0 && t.file.size > max) throw new RemoteError('超出管理员设置的文件大小上限', 413, 'too_large')
      const target = joinPath(t.dir, t.relpath)
      let offset = 0
      // 大文件：查询远端已有大小，小于本地则续传
      if (t.file.size > CHUNK_SIZE && !t.overwrite) {
        const st = await remoteApi.stat(opts.sid(), target)
        if (st.exists && st.entry && !st.entry.is_dir) {
          if (st.entry.size > 0 && st.entry.size < t.file.size) {
            offset = st.entry.size
            t.resumed = true
          } else {
            throw new RemoteError('文件已存在', 409, 'exists')
          }
        }
      }
      t.loaded = offset
      // 空文件也要发一次
      do {
        if (t.status !== 'uploading') return
        const end = Math.min(offset + CHUNK_SIZE, t.file.size)
        try {
          const res = await sendChunk(t, offset, end)
          offset = res.total ?? end
        } catch (e) {
          // 偏移不一致：按远端实际大小续传
          if (e instanceof RemoteError && e.code === 'offset_mismatch' && typeof e.data?.total === 'number' && e.data.total <= t.file.size) {
            offset = e.data.total
            continue
          }
          throw e
        }
        t.loaded = offset
      } while (offset < t.file.size)
      t.status = 'done'
      t.speed = 0
      opts.onFileDone(t.dir)
    } catch (e) {
      if ((t.status as TaskStatus) === 'canceled') return
      t.status = 'error'
      t.errorCode = e instanceof RemoteError ? e.code : ''
      t.error = friendlyError(e)
      t.speed = 0
    } finally {
      t.xhr = undefined
      pump()
    }
  }

  // 用 XHR 以获取上传进度并支持中止；字段顺序 relpath → offset → size → file 与后端流式解析一致
  function sendChunk(t: UploadTask, start: number, end: number): Promise<{ total: number }> {
    return new Promise((resolve, reject) => {
      const fd = new FormData()
      fd.append('relpath', t.relpath)
      fd.append('offset', String(start))
      // 整个文件大小：后端据此只在最后一片写完时记一条文件日志
      fd.append('size', String(t.file.size))
      fd.append('file', t.file.slice(start, end), t.file.name)
      const xhr = new XMLHttpRequest()
      t.xhr = xhr
      xhr.open('POST', remoteApi.uploadUrl(opts.sid(), t.dir, t.overwrite && start === 0))
      xhr.withCredentials = true
      let lastT = performance.now()
      let lastL = 0
      xhr.upload.onprogress = ev => {
        const now = performance.now()
        const dt = (now - lastT) / 1000
        if (dt >= 0.3) {
          const inst = (ev.loaded - lastL) / dt
          t.speed = t.speed ? t.speed * 0.6 + inst * 0.4 : inst
          lastT = now
          lastL = ev.loaded
        }
        // 进度按文件内容比例换算（multipart 头部开销忽略）
        const ratio = ev.total ? Math.min(1, ev.loaded / ev.total) : 0
        t.loaded = start + Math.round((end - start) * ratio)
      }
      xhr.onload = () => {
        let j: any = null
        try { j = JSON.parse(xhr.responseText) } catch { /* 非 JSON */ }
        if (xhr.status >= 200 && xhr.status < 300 && j?.code === 0) {
          const r = j.data?.files?.[0]
          if (r?.code) return reject(new RemoteError(r.error || '上传失败', xhr.status, r.code, r))
          return resolve({ total: r?.total ?? end })
        }
        reject(new RemoteError(j?.msg || `上传失败（${xhr.status}）`, xhr.status, j?.error || 'http_' + xhr.status, j?.data))
      }
      xhr.onerror = () => reject(new RemoteError('网络错误，上传中断', 0, 'network'))
      xhr.onabort = () => reject(new RemoteError('已取消', 0, 'canceled'))
      xhr.send(fd)
    })
  }

  function cancel(t: UploadTask) {
    if (t.status !== 'pending' && t.status !== 'uploading') return
    t.status = 'canceled'
    t.speed = 0
    t.xhr?.abort()
  }
  function retry(t: UploadTask, overwrite = false) {
    if (t.status === 'uploading') return
    t.overwrite = overwrite
    t.status = 'pending'
    pump()
  }
  function retryFailed() {
    tasks.filter(t => t.status === 'error').forEach(t => retry(t, t.overwrite))
  }
  function cancelAll() { [...active.value].forEach(cancel) }
  function clearFinished() {
    for (let i = tasks.length - 1; i >= 0; i--) {
      if (tasks[i].status === 'done' || tasks[i].status === 'canceled') tasks.splice(i, 1)
    }
  }

  return { tasks, active, summary, add, cancel, retry, retryFailed, cancelAll, clearFinished }
}

// ---------- 拖拽文件夹：webkitGetAsEntry 递归遍历 ----------

function readEntries(reader: any): Promise<any[]> {
  return new Promise((resolve, reject) => reader.readEntries(resolve, reject))
}
function entryFile(entry: any): Promise<File> {
  return new Promise((resolve, reject) => entry.file(resolve, reject))
}

// 返回待上传文件与需要创建的空目录（均为相对路径，不以 / 开头）
export async function collectDropped(dt: DataTransfer): Promise<{ files: PendingFile[]; dirs: string[] }> {
  const files: PendingFile[] = []
  const dirs: string[] = []
  const entries = Array.from(dt.items || [])
    .filter(i => i.kind === 'file')
    .map(i => (i as any).webkitGetAsEntry?.())
  if (!entries.length || entries.some(e => !e)) {
    // 不支持 Entry API：退化为平铺文件
    return { files: Array.from(dt.files).map(f => ({ file: f, relpath: f.name })), dirs }
  }
  async function walk(entry: any, prefix: string) {
    if (entry.isFile) {
      files.push({ file: await entryFile(entry), relpath: prefix + entry.name })
    } else if (entry.isDirectory) {
      const reader = entry.createReader()
      const children: any[] = []
      // readEntries 每次最多返回 100 个，需循环读到空
      for (;;) {
        const batch = await readEntries(reader)
        if (!batch.length) break
        children.push(...batch)
      }
      if (!children.length) dirs.push(prefix + entry.name)
      for (const c of children) await walk(c, prefix + entry.name + '/')
    }
  }
  for (const e of entries) await walk(e, '')
  return { files, dirs }
}
