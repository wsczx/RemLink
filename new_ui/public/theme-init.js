// 首帧前确定主题，避免闪烁（外部脚本，满足 CSP script-src 'self'）
(function () {
  try {
    var s = localStorage.getItem('dark-mode')
    var dark = s !== null ? s === 'true' : matchMedia('(prefers-color-scheme: dark)').matches
    if (dark) document.documentElement.dataset.theme = 'dark'
  } catch (e) { /* 忽略 */ }
})()
