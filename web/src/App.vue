<template>
  <div id="app">
    <router-view></router-view>
  </div>
</template>

<script>
export default {
  name: 'app',
  components: {},
  data() {
    return {}
  },
  created() {
    // 暗色模式初始化（所有页面统一生效，包括 Portal/WebAuth/Login）
    const saved = localStorage.getItem('dark-mode');
    let isDark;
    if (saved !== null) {
      isDark = saved === 'true';
    } else {
      isDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    }
    if (isDark) {
      document.documentElement.classList.add('dark');
    }
    // 监听系统主题变化（用户未手动设置时跟随）
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
      if (localStorage.getItem('dark-mode') === null) {
        document.documentElement.classList.toggle('dark', e.matches);
      }
    });
  },
}
</script>

<style>
/* ========== CSS 变量（全局设计令牌）========== */
:root {
  /* 主题色 */
  --color-primary: #409EFF;
  --color-primary-bg: #ecf5ff;
  --color-primary-light: #66b1ff;
  --color-primary-dark: #337ecc;
  --color-success: #67c23a;
  --color-warning: #e6a23c;
  --color-danger: #f56c6c;
  --color-info: #909399;

  /* 状态色浅背景 */
  --success-bg: #f0f9eb;
  --warning-bg: #fdf6ec;
  --danger-bg: #fef0f0;
  --info-bg: #f4f4f5;

  /* 侧边栏 */
  --sidebar-bg: #1b2138;
  --sidebar-bg-hover: #252d47;
  --sidebar-bg-active: rgba(64, 158, 255, 0.15);
  --sidebar-text: #a3b1cc;
  --sidebar-text-hover: #d0d5e8;
  --sidebar-text-active: #fff;
  --sidebar-width: 220px;
  --sidebar-collapse-width: 64px;

  /* 布局 */
  --header-bg: #fff;
  --header-height: 56px;
  --header-shadow: 0 1px 4px rgba(0, 0, 0, 0.06);
  --footer-bg: #fff;

  /* 背景 */
  --bg-page: #f0f2f5;
  --bg-card: #fff;
  --bg-hover: #f5f7fa;
  --bg-header: #fafafa;
  --bg-stripe: #fafbfc;
  --bg-overlay: rgba(0, 0, 0, 0.5);

  /* 卡片 */
  --card-radius: 8px;
  --card-shadow: 0 1px 8px rgba(0, 0, 0, 0.05);

  /* 文字 */
  --text-primary: #303133;
  --text-regular: #606266;
  --text-secondary: #909399;
  --text-placeholder: #c0c4cc;
  --text-inverse: #fff;

  /* 边框 */
  --border-base: #dcdfe6;
  --border-color: #e4e7ed;
  --border-color-light: #ebeef5;

  /* 过渡 */
  --transition-fast: 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  --transition-normal: 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

/* ========== 暗色模式 ========== */
html.dark {
  --color-primary-bg: #182234;
  --color-primary-light: #4a9eff;

  --success-bg: #14291a;
  --warning-bg: #2e2618;
  --danger-bg: #2e1818;
  --info-bg: #1e1e28;

  --header-bg: #161b2e;
  --header-shadow: none;
  --footer-bg: #161b2e;

  --bg-page: #0d1117;
  --bg-card: #161b2e;
  --bg-hover: #1c2238;
  --bg-header: #1c2238;
  --bg-stripe: #1c2238;
  --bg-overlay: rgba(0, 0, 0, 0.7);

  --card-shadow: none;

  --text-primary: #e6edf3;
  --text-regular: #b1bac4;
  --text-secondary: #7d8590;
  --text-placeholder: #484f58;
  --text-inverse: #0d1117;

  --border-base: #30363d;
  --border-color: #30363d;
  --border-color-light: #21262d;

  color-scheme: dark;
}

html,
body {
  height: 100%;
  margin: 0;
  background: var(--bg-page);
}

#app {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", "Helvetica Neue", Arial, sans-serif;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  color: var(--text-primary);
  height: 100%;
}

/* ========== 通用工具类 ========== */
.hide {
  display: none;
}

.sh-10 {
  height: 10px;
}

.sh-20 {
  height: 20px;
}

.sw-10 {
  height: 1px;
  width: 10px;
  display: inline-block;
}

.sw-20 {
  height: 1px;
  width: 20px;
  display: inline-block;
}

.m-left-10 {
  margin-left: 10px;
}

/* ========== 全局 Element UI 覆盖 ========== */

/* 表格美化 */
.el-table {
  border-radius: var(--card-radius);
  overflow: hidden;
}

.el-table th.el-table__cell {
  background: var(--bg-header);
  color: var(--text-primary);
  font-weight: 600;
  border-bottom: 2px solid var(--border-color-light);
}

.el-table--striped .el-table__body tr.el-table__row--striped td.el-table__cell {
  background: var(--bg-stripe);
}

.el-table__body tr:hover>td.el-table__cell {
  background: var(--color-primary-bg) !important;
}

/* 卡片美化 */
.el-card {
  border-radius: var(--card-radius) !important;
  border: 1px solid var(--border-color-light) !important;
  box-shadow: var(--card-shadow) !important;
}

.el-card__header {
  border-bottom: 1px solid var(--border-color-light);
  padding: 16px 20px;
  font-weight: 600;
  font-size: 15px;
  color: var(--text-primary);
  background: var(--bg-header);
  border-radius: var(--card-radius) var(--card-radius) 0 0 !important;
}

.el-card__body {
  overflow-x: auto;
}

/* 表单美化 */
.el-form-item__label {
  color: var(--text-regular);
  font-weight: 500;
}

.el-input__inner,
.el-textarea__inner {
  border-radius: 6px;
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
}

.el-input__inner:focus,
.el-textarea__inner:focus {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px rgba(64, 158, 255, 0.15);
}

/* 按钮美化 */
.el-button {
  border-radius: 6px;
  font-weight: 500;
  letter-spacing: 0.3px;
  transition: all var(--transition-fast);
}

.el-button--primary {
  background: var(--color-primary);
  border-color: var(--color-primary);
  box-shadow: 0 2px 6px rgba(64, 158, 255, 0.25);
}

.el-button--primary:hover {
  background: var(--color-primary-light);
  border-color: var(--color-primary-light);
  box-shadow: 0 4px 12px rgba(64, 158, 255, 0.35);
  transform: translateY(-1px);
}

/* 分页美化 */
.el-pagination {
  margin-top: 16px;
  font-weight: 500;
}

.el-pager li {
  border-radius: 4px;
}

/* 对话框美化 */
.el-dialog {
  border-radius: 12px;
  box-shadow: 0 12px 48px rgba(0, 0, 0, 0.15);
}

.el-dialog__header {
  padding: 20px 24px 16px;
  border-bottom: 1px solid var(--border-color-light);
}

.el-dialog__title {
  font-weight: 600;
  font-size: 16px;
}

.el-dialog__body {
  padding: 24px;
}

/* MessageBox 与 Dialog 使用同一套弹窗视觉 */
.el-message-box {
  border-radius: 12px;
  border: 1px solid var(--border-color-light);
  box-shadow: 0 12px 48px rgba(0, 0, 0, 0.15);
  padding-bottom: 0;
}

.el-message-box__header {
  padding: 20px 24px 16px;
  border-bottom: 1px solid var(--border-color-light);
}

.el-message-box__title {
  color: var(--text-primary);
  font-size: 16px;
  font-weight: 600;
}

.el-message-box__headerbtn {
  top: 20px;
  right: 20px;
}

.el-message-box__content {
  padding: 22px 24px;
  color: var(--text-regular);
}

.el-message-box__status {
  top: 23px;
}

.el-message-box__message {
  line-height: 1.7;
}

.el-message-box__btns {
  padding: 0 24px 20px;
}

.system-warning-message-box {
  border-left: 4px solid var(--color-warning);
}

.system-warning-message-box .el-message-box__status.el-icon-error {
  color: var(--color-danger);
}

.system-warning-message-box .el-message-box__status.el-icon-warning {
  color: var(--color-warning);
}

/* 标签/Tag 美化 */
.el-tag {
  border-radius: 4px;
  font-weight: 500;
}

/* 下拉菜单美化（与 el-select 弹层风格统一） */
.el-dropdown-menu {
  border-radius: 8px;
  border: 1px solid var(--border-color-light);
  box-shadow: 0 6px 24px rgba(0, 0, 0, 0.12);
  padding: 6px 0;
}

.el-dropdown-menu__item {
  color: var(--text-regular);
  line-height: 34px;
  padding: 0 16px;
}

.el-dropdown-menu__item:hover {
  background: var(--color-primary-bg);
  color: var(--color-primary);
}

.el-dropdown-menu__item--divided:before {
  margin: 0 8px;
  background-color: var(--bg-card);
}

/* 折叠侧栏弹出的 Element UI 菜单挂在 body 下，需要全局样式覆盖透明背景 */
.el-menu--vertical .el-menu--popup {
  background-color: var(--sidebar-bg) !important;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 8px;
  box-shadow: 0 4px 24px rgba(0, 0, 0, 0.4);
  padding: 4px 0;
  min-width: 160px;
}

.el-menu--vertical .el-menu--popup .el-menu-item {
  background: transparent;
  color: var(--sidebar-text);
  height: 40px;
  line-height: 40px;
  font-size: 13px;
  margin: 0;
  border-radius: 0;
  padding-left: 20px !important;
}

.el-menu--vertical .el-menu--popup .el-menu-item:hover {
  background: var(--sidebar-bg-hover) !important;
  color: var(--sidebar-text-hover) !important;
}

.el-menu--vertical .el-menu--popup .el-menu-item.is-active {
  background: var(--sidebar-bg-active) !important;
  color: var(--sidebar-text-active) !important;
}

/* 面包屑 */
.el-breadcrumb__inner {
  color: var(--text-secondary);
  font-weight: 500;
}

.el-breadcrumb__item:last-child .el-breadcrumb__inner {
  color: var(--text-primary);
  font-weight: 600;
}

/* 滚动条美化 */
::-webkit-scrollbar {
  width: 6px;
  height: 6px;
}

::-webkit-scrollbar-track {
  background: transparent;
}

::-webkit-scrollbar-thumb {
  background: var(--text-placeholder);
  border-radius: 3px;
}

::-webkit-scrollbar-thumb:hover {
  background: var(--text-secondary);
}

/* 消息提示 */
.el-message {
  border-radius: 8px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.1);
}

/* 统计数字动画 */
.count-to-text {
  font-weight: 700;
}

/* ========== 统计卡片（全局共享）========== */
.stats-row {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  margin-bottom: 16px;
}

.stat-card {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 18px 20px;
  background: var(--bg-card);
  border-radius: var(--card-radius);
  box-shadow: var(--card-shadow);
  cursor: default;
}

.stat-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 22px;
  flex-shrink: 0;
}

.stat-body {
  display: flex;
  flex-direction: column;
}

.stat-value {
  font-size: 26px;
  font-weight: 700;
  color: var(--text-primary);
  line-height: 1.2;
}

.stat-label {
  font-size: 13px;
  color: var(--text-secondary);
  margin-top: 2px;
}

/* ========== 表格卡片（全局共享）========== */
.table-card {
  border-radius: var(--card-radius);
  overflow: hidden;
  border: 1px solid var(--border-color-light);
}

.table-card .el-card__header {
  padding: 16px 20px;
  background: var(--bg-header);
  border-bottom: 1px solid var(--border-color-light);
}

/* ========== Tab 统一风格 ========== */
.el-tabs__header {
  margin-bottom: 0;
  padding: 0 20px;
  background: var(--bg-header);
  border-bottom: 1px solid var(--border-color-light);
}

.el-tabs__item {
  height: 44px;
  line-height: 44px;
  font-size: 14px;
  font-weight: 500;
}

.el-tabs__item i {
  margin-right: 6px;
  font-size: 14px;
}

.el-tabs__nav-wrap::after {
  display: none;
}

.el-tabs__content {
  padding: 20px;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.card-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary);
}

.card-title i {
  margin-right: 6px;
  color: var(--color-primary);
}

.card-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.search-input {
  width: 200px;
}

/* 响应式 */
@media (max-width: 1200px) {
  .stats-row {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 768px) {
  .stats-row {
    grid-template-columns: repeat(2, 1fr);
    gap: 10px;
  }

  .stat-card {
    padding: 14px 16px;
  }

  .stat-value {
    font-size: 22px;
  }

  .stat-label {
    font-size: 12px;
  }

  .stat-icon {
    width: 42px;
    height: 42px;
    font-size: 18px;
  }

  /* 表格横向滚动 */
  .el-table {
    font-size: 13px;
  }

  /* 搜索框铺满 */
  .search-input {
    width: 100% !important;
  }

  /* 卡片标题区在移动端堆叠 */
  .card-header {
    flex-direction: column;
    align-items: flex-start;
    gap: 10px;
  }

  .card-actions {
    flex-wrap: wrap;
    width: 100%;
  }

  .card-actions .el-button {
    margin-left: 0;
  }

  /* 对话框宽度适配 */
  .el-dialog {
    width: 92% !important;
    margin: 10px auto !important;
    border-radius: 10px;
  }

  .el-dialog__header {
    padding: 16px 20px 14px;
  }

  .el-dialog__body {
    padding: 16px 20px;
  }

  .el-dialog__footer {
    padding: 12px 20px 16px;
  }

  /* MessageBox 适配 */
  .el-message-box {
    width: 90% !important;
    max-width: 420px;
  }

  /* 分页居中 */
  .el-pagination {
    text-align: center;
  }

  .el-pagination .el-pagination__total,
  .el-pagination .el-pagination__sizes {
    float: none;
    display: block;
    margin-bottom: 6px;
  }

  /* 按钮组不换行时缩小间距 */
  .el-button+.el-button {
    margin-left: 8px;
  }

  /* 表单在窄屏下 label 置顶 */
  .el-form-item--small .el-form-item__label {
    float: none;
    display: block;
    text-align: left;
    padding-bottom: 6px;
  }

  .el-form-item--small .el-form-item__content {
    margin-left: 0 !important;
  }
}

/* 超小屏 (≤480px) */
@media (max-width: 480px) {
  .stats-row {
    grid-template-columns: 1fr;
  }

  .el-dialog {
    width: 96% !important;
  }

  .el-message-box {
    width: 92% !important;
  }
}

/* ========== 暗色模式：Element UI 组件覆盖 ========== */
html.dark .el-card {
  background: var(--bg-card) !important;
  border-color: var(--border-color-light) !important;
}

html.dark .el-table th.el-table__cell {
  background: var(--bg-card) !important;
  color: var(--text-primary) !important;
}

html.dark .el-table,
html.dark .el-table tr,
html.dark .el-table td.el-table__cell {
  background: var(--bg-card) !important;
  color: var(--text-regular) !important;
  border-color: var(--border-color-light) !important;
}

html.dark .el-table--striped .el-table__body tr.el-table__row--striped td.el-table__cell {
  background: var(--bg-stripe) !important;
}

html.dark .el-table__body tr:hover>td.el-table__cell {
  background: var(--bg-hover) !important;
}

html.dark .el-input__inner,
html.dark .el-textarea__inner,
html.dark .el-input-number,
html.dark .el-select .el-input__inner {
  background: var(--bg-hover) !important;
  border-color: var(--border-color) !important;
  color: var(--text-primary) !important;
}

html.dark .el-input__inner::placeholder,
html.dark .el-textarea__inner::placeholder {
  color: var(--text-placeholder) !important;
}

html.dark .el-dialog,
html.dark .el-message-box {
  background: var(--bg-card) !important;
  color: var(--text-primary) !important;
}

html.dark .el-dialog__header,
html.dark .el-dialog__body,
html.dark .el-message-box__header,
html.dark .el-message-box__content {
  color: var(--text-primary) !important;
}

html.dark .el-dialog__header,
html.dark .el-dialog__headerbtn .el-dialog__close,
html.dark .el-message-box__headerbtn .el-message-box__close {
  color: var(--text-regular) !important;
}

html.dark .el-collapse-item__header,
html.dark .el-collapse-item__wrap,
html.dark .el-collapse-item__content {
  background: var(--bg-card) !important;
  color: var(--text-primary) !important;
  border-color: var(--border-color-light) !important;
}

html.dark .el-collapse-item__header {
  border-bottom-color: var(--border-color-light) !important;
}

html.dark .el-tag--info {
  background: var(--info-bg) !important;
  border-color: var(--border-color) !important;
  color: var(--text-secondary) !important;
}

html.dark .el-select-dropdown {
  background: var(--bg-card) !important;
  border-color: var(--border-color) !important;
}

html.dark .el-select-dropdown__item {
  color: var(--text-regular) !important;
}

html.dark .el-select-dropdown__item.hover,
html.dark .el-select-dropdown__item:hover {
  background: var(--bg-hover) !important;
}

html.dark .el-select-dropdown__item.selected {
  color: var(--color-primary) !important;
}

html.dark .el-dropdown-menu {
  background: var(--bg-card) !important;
  border-color: var(--border-color) !important;
  border-radius: 8px !important;
  box-shadow: 0 6px 24px rgba(0, 0, 0, 0.4) !important;
  padding: 6px 0 !important;
}

html.dark .el-dropdown-menu__item {
  color: var(--text-regular) !important;
  line-height: 34px !important;
  padding: 0 16px !important;
}

html.dark .el-dropdown-menu__item:hover {
  background: var(--bg-hover) !important;
  color: var(--color-primary) !important;
}

html.dark .el-dropdown-menu__item--divided:before {
  margin: 0 8px !important;
  background-color: var(--bg-card) !important;
}


html.dark .el-popper[x-placement^="bottom"] .popper__arrow,
html.dark .el-popper[x-placement^="top"] .popper__arrow {
  border-bottom-color: var(--border-color) !important;
}

html.dark .el-switch__core {
  background-color: var(--border-color) !important;
  border-color: var(--border-color) !important;
}

html.dark .el-radio-button__inner {
  background: var(--bg-hover) !important;
  color: var(--text-regular) !important;
  border-color: var(--border-color) !important;
}

html.dark .el-radio-button__orig-radio:checked+.el-radio-button__inner {
  background: var(--color-primary) !important;
  color: var(--text-inverse) !important;
  border-color: var(--color-primary) !important;
}

html.dark .el-loading-mask {
  background: rgba(20, 24, 40, 0.7) !important;
}

html.dark .el-form-item__label {
  color: var(--text-regular) !important;
}

html.dark .el-button--default {
  background: var(--bg-card) !important;
  color: var(--text-regular) !important;
  border-color: var(--border-color) !important;
}

html.dark .el-button--default:hover {
  background: var(--bg-hover) !important;
  color: var(--color-primary) !important;
  border-color: var(--color-primary) !important;
}

html.dark .el-divider {
  background-color: var(--border-color-light) !important;
}

html.dark .el-divider__text {
  background: var(--bg-card) !important;
  color: var(--text-secondary) !important;
}

html.dark .el-tooltip__popper {
  background: var(--bg-hover) !important;
  color: var(--text-primary) !important;
}

html.dark .el-tabs__item {
  color: var(--text-secondary) !important;
}

html.dark .el-tabs__item:hover {
  color: var(--text-primary) !important;
}

html.dark .el-tabs__item.is-active {
  color: var(--color-primary) !important;
}

html.dark .el-tabs__active-bar {
  background-color: var(--color-primary) !important;
}

html.dark .el-tabs__nav-wrap::after {
  background-color: var(--border-color-light) !important;
}

html.dark .el-tabs__header {
  border-bottom-color: var(--border-color-light) !important;
}

/* ========== 自定义页脚徽章（管理员在「页脚内容」中填的 HTML 使用 .github-badge 即可） ==========
   放在全局样式，因为页脚 HTML 由 v-html 注入，scoped 样式无法命中 */
.login-footer .github-badge,
.portal-footer .github-badge,
.portal-login-footer .github-badge,
.webauth-footer .github-badge,
.layout-footer .github-badge {
  display: inline-flex;
  align-items: stretch;
  vertical-align: middle;
  margin: 0 2px;
  border-radius: 3px;
  overflow: hidden;
  background: #fff;
  box-shadow: 0 0 1px rgba(0, 0, 0, 0.15);
  font-size: 11px;
  line-height: 18px;
  text-decoration: none;
}

.login-footer .github-badge a,
.portal-footer .github-badge a,
.portal-login-footer .github-badge a,
.webauth-footer .github-badge a,
.layout-footer .github-badge a {
  display: inline-flex;
  align-items: center;
  text-decoration: none;
  color: inherit;
}

.login-footer .badge-subject,
.portal-footer .badge-subject,
.portal-login-footer .badge-subject,
.webauth-footer .badge-subject,
.layout-footer .badge-subject {
  display: inline-block;
  padding: 0 6px;
  background: #555;
  color: #fff;
  border-radius: 3px 0 0 3px;
}

.login-footer .badge-value,
.portal-footer .badge-value,
.portal-login-footer .badge-value,
.webauth-footer .badge-value,
.layout-footer .badge-value {
  display: inline-block;
  padding: 0 6px;
  color: #fff;
  border-radius: 0 3px 3px 0;
}

.login-footer .bg-blue,
.portal-footer .bg-blue,
.portal-login-footer .bg-blue,
.webauth-footer .bg-blue,
.layout-footer .bg-blue {
  background: #007ec6;
}

.login-footer .bg-green,
.portal-footer .bg-green,
.portal-login-footer .bg-green,
.webauth-footer .bg-green,
.layout-footer .bg-green {
  background: #4c1;
}

.login-footer .bg-orange,
.portal-footer .bg-orange,
.portal-login-footer .bg-orange,
.webauth-footer .bg-orange,
.layout-footer .bg-orange {
  background: #f60;
}

.login-footer .bg-grey,
.portal-footer .bg-grey,
.portal-login-footer .bg-grey,
.webauth-footer .bg-grey,
.layout-footer .bg-grey {
  background: #888;
}
</style>
