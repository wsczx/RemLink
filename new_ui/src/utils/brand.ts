// 品牌配置：后端在 index.html 内联注入 window.__BRAND__，登录后再从接口拉取
declare global {
  interface Window { __BRAND__?: { title?: string; favicon?: string } }
}

export interface Brand { title: string; logo: string; favicon: string; [k: string]: any }

export function inlineBrand() {
  const b = window.__BRAND__ || {}
  // 未经后端注入（vite dev）时占位符原样保留，视为空
  const clean = (s?: string) => (s && !s.startsWith('{{') ? s : '')
  return { title: clean(b.title), favicon: clean(b.favicon) }
}

export function normalizeImageSrc(src?: string) {
  if (!src) return ''
  const s = String(src).trim()
  if (s.startsWith('<svg') || s.startsWith('<?xml')) return 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(s)
  return s
}

export function applyBrandToDocument(brand?: Partial<Brand>) {
  if (!brand) return
  document.title = brand.title || inlineBrand().title || 'RemLink'
  const href = normalizeImageSrc(brand.favicon)
  let link = document.querySelector<HTMLLinkElement>("link[rel='icon']:not([media])")
  if (!link) {
    link = document.createElement('link')
    link.rel = 'icon'
    document.head.appendChild(link)
  }
  link.href = href || import.meta.env.BASE_URL + 'favicon.svg'
}
