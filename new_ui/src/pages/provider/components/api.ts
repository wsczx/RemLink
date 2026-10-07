// 认证源页面共用的数据加载
import { get } from '@/api/http'
import type { ProviderRow } from './types'

// 拉取全部认证源（后端分页，认证源数量很少，逐页拉完）
export async function loadAllProviders(): Promise<{ rows: ProviderRow[]; callbackBase: string; active: number }> {
  const first = await get('/api/v1/provider/list', { page: 1 })
  const rows: ProviderRow[] = [...(first?.datas || [])]
  const count = first?.count || 0
  const size = first?.page_size || rows.length || 1
  const pages = Math.ceil(count / size)
  for (let p = 2; p <= pages && p <= 50; p++) {
    const d = await get('/api/v1/provider/list', { page: p })
    rows.push(...(d?.datas || []))
  }
  return { rows, callbackBase: first?.callbackBase || '', active: first?.stats_active || 0 }
}

// 拉取全部用户组（用于统计认证源被哪些组引用），失败时返回空数组不影响主流程
export async function loadAllGroups(): Promise<any[]> {
  try {
    const d = await get('/api/v1/group/list', { page: 1, page_size: 1000 })
    return d?.datas || []
  } catch {
    return []
  }
}
