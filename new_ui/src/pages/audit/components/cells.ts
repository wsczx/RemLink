// 事件流表格的通用单元格
import { h } from 'vue'
import dayjs from 'dayjs'
import CountryFlag from '@/components/CountryFlag.vue'
import { hostOf, isPrivateIp } from '@/utils/net'
import { relTime } from './shared'

export const dash = () => h('span', { class: 'text-quaternary' }, '—')

// 时间：绝对时间（等宽）+ 相对时间
export const timeCell = (v?: string) => v
  ? h('div', { class: 'min-w-0 leading-18px', title: dayjs(v).format('YYYY-MM-DD HH:mm:ss') }, [
      h('div', { class: 'rl-mono whitespace-nowrap' }, dayjs(v).format('MM-DD HH:mm:ss')),
      h('div', { class: 'text-sm text-tertiary' }, relTime(v)),
    ])
  : dash()

// 主信息 + 副信息两行
export const twoLine = (main: any, sub?: any, mainCls = 'font-semibold') => h('div', { class: 'min-w-0 leading-18px' }, [
  h('div', { class: ['truncate', mainCls] }, main),
  sub === undefined ? null : h('div', { class: 'truncate text-sm text-secondary' }, sub || '—'),
])

// 来源地址的国旗 code：内网传 LAN，否则未知
export const flagCode = (addr?: string, cc?: string) => cc || (isPrivateIp(hostOf(addr)) ? 'LAN' : '')

// 国旗 + IP
export const ipCell = (addr?: string, cc?: string) => addr
  ? h('div', { class: 'flex min-w-0 items-center gap-8px' }, [
      h(CountryFlag, { code: flagCode(addr, cc) }),
      h('span', { class: 'rl-mono truncate', title: addr }, addr),
    ])
  : dash()

export const userLabel = (u: string, n?: string) => (n && n !== u ? `${u}（${n}）` : u)
