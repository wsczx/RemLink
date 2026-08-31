<template>
  <el-container style="height: 100%;">
    <!-- 移动端遮罩 -->
    <div v-show="mobile_open" class="aside-overlay" @click="closeMobileMenu"></div>

    <!-- 侧边栏菜单 -->
    <el-aside :width="asideWidth" class="layout-aside-wrap" :class="asideClasses">
      <LayoutAside :is_active="is_active" />
    </el-aside>

    <el-container class="layout-main">
      <!-- 顶部栏 -->
      <el-header class="layout-header">
        <LayoutHeader :is_active.sync="is_active" :route_name="route_name" :is_mobile="is_mobile"
          @toggle="handleToggle" />
      </el-header>

      <!-- 内容区域 -->
      <el-main class="layout-content">
        <router-view :route_name.sync="route_name"></router-view>
      </el-main>

      <!-- 底部 -->
      <el-footer class="layout-footer">
        <span v-if="brand.footer" class="footer-text" v-html="brand.footer"></span>
        <span v-if="brand.footer" class="footer-divider">|</span>
        <span class="footer-text">RemLink 企业级安全远程接入网关</span>
        <span class="footer-divider">|</span>
        <a href="https://github.com/wsczx/RemLink" target="_blank" class="footer-link">RemLink</a>
        <span class="footer-divider">|</span>
        <span class="footer-text">&copy; 2020-present</span>
      </el-footer>
    </el-container>
  </el-container>
</template>

<script>
import LayoutAside from "@/layout/LayoutAside";
import LayoutHeader from "@/layout/LayoutHeader";
import axios from "axios";
import { applyBrandToDocument } from "@/plugins/brand";

export default {
  name: "Layout",
  components: { LayoutHeader, LayoutAside },
  data() {
    return {
      is_active: true,
      mobile_open: false,
      is_mobile: false,
      route_name: ['仪表盘'],
      brand: { footer: "" },
      system_warning_prompt_shown: false,
      system_warning_checking: false,
      upgrade_checking: false,
    }
  },
  computed: {
    asideWidth() {
      if (this.is_mobile) return '220px'
      return this.is_active ? '220px' : '64px'
    },
    asideClasses() {
      return {
        'aside-mobile': this.is_mobile,
        'aside-mobile-show': this.mobile_open
      }
    }
  },
  methods: {
    handleToggle() {
      if (this.is_mobile) {
        this.mobile_open = !this.mobile_open
      } else {
        this.is_active = !this.is_active
      }
    },
    closeMobileMenu() {
      this.mobile_open = false
    },
    handleResize() {
      this.is_mobile = window.innerWidth <= 768
      if (!this.is_mobile) {
        this.mobile_open = false
      }
    },
    checkSystemWarnings() {
      if (this.system_warning_prompt_shown || this.system_warning_checking) {
        return
      }
      this.system_warning_checking = true
      axios.get('/set/soft/status', {}).then(resp => {
        const warnings = (resp.data.data && resp.data.data.warnings) || []
        const warning = this.pickGlobalWarning(warnings)
        if (!warning) {
          return
        }
        this.system_warning_prompt_shown = true
        this.$confirm(warning.message, warning.title, {
          confirmButtonText: '去配置',
          cancelButtonText: '稍后',
          type: warning.level || 'warning',
          customClass: 'system-warning-message-box'
        }).then(() => {
          if (this.$route.path !== warning.action_path) {
            this.$router.push(warning.action_path)
          }
        }).catch(() => { })
      }).catch(() => {
      }).finally(() => {
        this.system_warning_checking = false
      })
    },
    pickGlobalWarning(warnings) {
      const globalWarnings = [
        {
          code: 'admin_temp_password',
          title: '安全风险',
          action_path: '/admin/set/security',
        },
        {
          code: 'nat_interface_missing',
          title: '网络配置错误',
          action_path: '/admin/set/soft',
        },
        {
          code: 'initial_setup',
          title: '初始化配置',
          action_path: '/admin/set/soft',
        },
      ]

      for (const item of globalWarnings) {
        const warning = warnings.find(w => this.warningMatches(w, item.code))
        if (warning) {
          return this.normalizeWarning(warning, item)
        }
      }
      return null
    },
    warningMatches(warning, code) {
      if (!warning) {
        return false
      }
      if (typeof warning === 'string') {
        return code === 'initial_setup' && warning.indexOf('默认初始化配置') !== -1
      }
      return warning.code === code
    },
    checkDailyUpgrade() {
      // 每日最多检查一次；失败不记录，下次路由切换重试
      const today = new Date().toISOString().slice(0, 10)
      if (localStorage.getItem('upgrade-check-date') === today || this.upgrade_checking) {
        return
      }
      this.upgrade_checking = true
      axios.get('/set/upgrade/check').then(resp => {
        localStorage.setItem('upgrade-check-date', today)
        const data = resp.data.data
        if (!data || !data.need_upgrade || !data.latest) {
          return
        }
        this.$confirm(
          `发现新版本 ${data.latest.version}（当前 ${data.current_version}），是否前往升级？`,
          '版本更新',
          {
            confirmButtonText: '去升级',
            cancelButtonText: '稍后',
            type: 'info',
          }
        ).then(() => {
          if (this.$route.path !== '/admin/set/system') {
            this.$router.push('/admin/set/system')
          }
        }).catch(() => { })
      }).catch(() => { }).finally(() => {
        this.upgrade_checking = false
      })
    },
    normalizeWarning(warning, fallback) {
      if (typeof warning === 'string') {
        return {
          title: fallback.title,
          level: 'warning',
          message: warning,
          action_path: fallback.action_path,
        }
      }
      return {
        title: fallback.title,
        level: warning.level || 'warning',
        message: warning.message || '',
        action_path: warning.action_path || fallback.action_path,
      }
    },
  },
  watch: {
    '$route.path': {
      immediate: true,
      handler() {
        // 移动端切换路由后自动关闭菜单
        if (this.is_mobile) {
          this.mobile_open = false
        }
        this.$nextTick(this.checkSystemWarnings)
        this.$nextTick(this.checkDailyUpgrade)
      },
    },
  },
  created() {
    this.handleResize()
    window.addEventListener('resize', this.handleResize)
    // 加载品牌配置（含自定义页脚）
    axios.get('/portal/api/login-config').then(resp => {
      if (resp.data && resp.data.data) {
        this.brand = Object.assign({ footer: "" }, resp.data.data)
        applyBrandToDocument(this.brand)
      }
    }).catch(() => { })
    // 页面长期不切路由时（跨天）也能触发每日检查
    this._upgradeTimer = setInterval(this.checkDailyUpgrade, 60 * 60 * 1000)
  },
  beforeDestroy() {
    window.removeEventListener('resize', this.handleResize)
    clearInterval(this._upgradeTimer)
  },
}
</script>

<style>
.layout-aside-wrap {
  flex: 0 0 auto;
  transition: width var(--transition-normal);
  overflow: hidden;
}

.layout-main {
  flex-direction: column;
  overflow: hidden;
  min-width: 0;
}

.layout-header {
  background: var(--header-bg);
  color: var(--text-primary);
  line-height: var(--header-height);
  height: var(--header-height) !important;
  padding: 0 20px;
  border-bottom: 1px solid var(--border-color-light);
  box-shadow: var(--header-shadow);
  z-index: 10;
  flex-shrink: 0;
}

.layout-content {
  background: var(--bg-page);
  padding: 20px;
  flex: 1;
  overflow: auto;
  min-width: 0;
}

.layout-footer {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 6px 8px;
  min-height: 40px;
  height: auto !important;
  padding: 8px 12px;
  line-height: 1.5;
  font-size: 12px;
  color: var(--text-secondary);
  background: var(--footer-bg);
  border-top: 1px solid var(--border-color-light);
  flex-shrink: 0;
  text-align: center;
}

.footer-divider {
  color: var(--border-color);
}

/* 自定义页脚（v-html 注入）内的徽章组在小屏可换行居中 */
.layout-footer .footer-text {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 6px 8px;
}

/* 窄屏页脚已换行，竖线分隔符无意义，隐藏避免孤立竖线 */
@media (max-width: 480px) {
  .layout-footer .footer-divider {
    display: none;
  }
}

.footer-link {
  color: var(--color-primary);
  text-decoration: none;
  font-weight: 500;
}

.footer-link:hover {
  text-decoration: underline;
}

/* ========== 移动端侧边栏 overlay ========== */
.aside-overlay {
  display: none;
}

@media (max-width: 768px) {
  .aside-overlay {
    display: block;
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    z-index: 998;
    transition: opacity var(--transition-normal);
  }

  .layout-aside-wrap.aside-mobile {
    position: fixed;
    left: 0;
    top: 0;
    bottom: 0;
    z-index: 999;
    transform: translateX(-100%);
    transition: transform var(--transition-normal);
    width: 220px !important;
  }

  .layout-aside-wrap.aside-mobile.aside-mobile-show {
    transform: translateX(0);
    box-shadow: 2px 0 12px rgba(0, 0, 0, 0.15);
  }

  .layout-header {
    padding: 0 12px;
  }

  .layout-content {
    padding: 12px 8px;
  }

  .layout-footer {
    font-size: 11px;
    gap: 4px;
  }
}
</style>
