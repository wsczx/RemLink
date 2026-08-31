<template>

  <div class="layout-aside">
    <!-- 品牌区域 -->
    <div class="aside-brand" :class="{ collapsed: !is_active }">
      <img v-if="brand.logo" :src="brand.logo" class="brand-logo-img" alt="logo" />
      <img v-else :src="baseUrl + 'logo.svg'" class="brand-logo-img" alt="logo" />
      <span v-show="is_active" class="brand-title">{{ brand.title || 'RemLink' }}</span>
    </div>

    <!-- 菜单 -->
    <el-menu :collapse="!is_active" :default-active="$route.path" class="layout-menu" :collapse-transition="false"
      background-color="transparent" text-color="#969db8" active-text-color="#fff" router>

      <!-- 仪表盘 -->
      <el-menu-item index="/admin/home">
        <i class="el-icon-s-home"></i>
        <span slot="title">仪表盘</span>
      </el-menu-item>

      <!-- 用户管理 -->
      <el-submenu index="user">
        <template slot="title">
          <i class="el-icon-user-solid"></i>
          <span slot="title">用户管理</span>
        </template>
        <el-menu-item index="/admin/user/list">用户列表</el-menu-item>
        <el-menu-item index="/admin/user/online">在线用户</el-menu-item>
        <el-menu-item index="/admin/user/lockmanager">锁定管理</el-menu-item>
        <el-menu-item index="/admin/user/ip_map">IP 映射</el-menu-item>
      </el-submenu>

      <!-- 访问控制 -->
      <el-submenu index="access">
        <template slot="title">
          <i class="el-icon-s-grid"></i>
          <span slot="title">访问控制</span>
        </template>
        <el-menu-item index="/admin/group/list">用户组管理</el-menu-item>
        <el-menu-item index="/admin/policy/list">策略管理</el-menu-item>
        <el-menu-item index="/admin/provider/list">认证源管理</el-menu-item>
      </el-submenu>

      <!-- WebVPN -->
      <el-submenu index="webvpn">
        <template slot="title">
          <i class="el-icon-connection"></i>
          <span slot="title">WebVPN</span>
        </template>
        <el-menu-item index="/admin/webvpn">WebVPN 应用</el-menu-item>
        <el-menu-item index="/admin/webvpn/audit">WebVPN 审计</el-menu-item>
      </el-submenu>

      <!-- 系统设置 -->
      <el-submenu index="system">
        <template slot="title">
          <i class="el-icon-setting"></i>
          <span slot="title">系统设置</span>
        </template>
        <el-menu-item index="/admin/set/system">系统信息</el-menu-item>
        <el-menu-item index="/admin/set/soft">软件配置</el-menu-item>
        <el-menu-item index="/admin/set/security">安全设置</el-menu-item>
        <el-menu-item index="/admin/set/cert">证书设置</el-menu-item>
        <el-menu-item index="/admin/set/other">其他设置</el-menu-item>
      </el-submenu>

      <!-- 日志审计 -->
      <el-submenu index="audit">
        <template slot="title">
          <i class="el-icon-s-data"></i>
          <span slot="title">日志审计</span>
        </template>
        <el-menu-item index="/admin/set/syslog">系统日志</el-menu-item>
        <el-menu-item index="/admin/set/audit">安全审计</el-menu-item>
        <el-menu-item>
          <span class="debug-tool-link" @click="openDebugTool('/debug/pprof/')">诊断工具 pprof</span>
        </el-menu-item>
        <el-menu-item>
          <span class="debug-tool-link" @click="openDebugTool('/debug/statsviz/')">诊断工具 statsviz</span>
        </el-menu-item>
      </el-submenu>

    </el-menu>
  </div>

</template>

<script>
import axios from "axios";
import { applyBrandToDocument } from "../plugins/brand";

export default {
  name: "LayoutAside",
  data() {
    return {
      brand: { title: "", logo: "" },
    }
  },
  props: ['is_active'],
  mounted() {
    this.loadBrand();
    // 监听品牌更新事件：管理后台保存品牌配置后，立即重新加载并应用（无需刷新页面）
    this._brandHandler = () => { this.loadBrand() };
    window.addEventListener('remlink:brand-updated', this._brandHandler);
  },
  beforeDestroy() {
    if (this._brandHandler) {
      window.removeEventListener('remlink:brand-updated', this._brandHandler);
    }
  },
  methods: {
    loadBrand() {
      axios.get('/portal/api/login-config').then(resp => {
        if (resp.data && resp.data.data) {
          this.brand = Object.assign({ title: "", logo: "" }, resp.data.data)
          applyBrandToDocument(this.brand)
        }
      }).catch(() => { })
    },
    // 打开诊断工具：未启用时在当前页报错，而不是跳转新页面显示 403
    openDebugTool(url) {
      axios.get(url, { validateStatus: () => true })
        .then((resp) => {
          if (resp.status === 200) {
            window.open(url, '_blank')
          } else {
            this.$message.error('诊断工具未启用，请在服务端配置中开启 pprof 后重启服务')
          }
        })
        .catch(() => {
          this.$message.error('无法访问诊断工具，请确认服务状态')
        })
    },
  },
}
</script>

<style scoped>
.debug-tool-link {
  cursor: pointer;
}

.layout-aside {
  height: 100%;
  width: 100%;
  display: flex;
  flex-direction: column;
  background: var(--sidebar-bg);
  overflow: hidden;
}

/* 品牌区域 */
.aside-brand {
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  overflow: hidden;
  white-space: nowrap;
}

.aside-brand.collapsed {
  padding: 0;
  justify-content: center;
}

.brand-icon {
  font-size: 26px;
  color: var(--color-primary);
  flex-shrink: 0;
}

.brand-logo-img {
  width: 26px;
  height: 26px;
  object-fit: contain;
  flex-shrink: 0;
}

.brand-title {
  margin-left: 10px;
  font-size: 18px;
  font-weight: 700;
  color: var(--text-inverse);
  letter-spacing: 1px;
  overflow: hidden;
}

/* 菜单容器 — 强制撑满父容器宽度，防止子菜单开合导致宽度变化 */
.layout-menu {
  flex: 1 1 0;
  min-width: 0;
  border-right: none;
  overflow-y: auto;
  overflow-x: hidden;
}

/* 覆盖 el-menu 自身的宽度计算，始终占满父容器 */
.layout-aside ::v-deep>.el-menu {
  width: 100% !important;
  box-sizing: border-box;
}

/* 菜单项 */
.layout-menu /deep/ .el-menu-item {
  height: 48px;
  line-height: 48px;
  margin: 2px 8px;
  border-radius: 8px;
  font-size: 14px;
  transition: all var(--transition-fast);
}

.layout-menu /deep/ .el-menu-item:hover {
  background: var(--sidebar-bg-hover) !important;
  color: var(--sidebar-text-hover) !important;
}

.layout-menu /deep/ .el-menu-item.is-active {
  background: var(--sidebar-bg-active) !important;
  color: var(--sidebar-text-active) !important;
  font-weight: 600;
  position: relative;
}

.layout-menu /deep/ .el-menu-item.is-active::before {
  content: '';
  position: absolute;
  left: 0;
  top: 50%;
  transform: translateY(-50%);
  width: 3px;
  height: 20px;
  background: var(--color-primary);
  border-radius: 0 2px 2px 0;
}

.layout-menu /deep/ .el-menu-item i {
  color: inherit;
}

/* 子菜单标题 */
.layout-menu /deep/ .el-submenu__title {
  height: 48px;
  line-height: 48px;
  margin: 2px 8px;
  border-radius: 8px;
  font-size: 14px;
  font-weight: 500;
  transition: all var(--transition-fast);
}

.layout-menu /deep/ .el-submenu__title:hover {
  background: var(--sidebar-bg-hover) !important;
  color: var(--sidebar-text-hover) !important;
}

.layout-menu /deep/ .el-submenu__title i {
  color: inherit;
}

/* 子菜单内的项 */
.layout-menu /deep/ .el-menu--inline .el-menu-item {
  padding-left: 56px !important;
  height: 40px;
  line-height: 40px;
  font-size: 13px;
}

/* 折叠状态 — 让 Element UI 原生 collapse 逻辑处理居中，只微调间距和图标大小 */
.layout-menu.el-menu--collapse /deep/ .el-menu-item,
.layout-menu.el-menu--collapse /deep/ .el-submenu__title {
  margin: 2px 8px;
}

.layout-menu.el-menu--collapse /deep/ .el-menu-item i,
.layout-menu.el-menu--collapse /deep/ .el-submenu__title i {
  font-size: 20px;
  vertical-align: middle;
  margin-right: 0;
}

/* 折叠后弹出的子菜单 — 补上深色背景（menu 的 background-color="transparent" 会传递到 popup 导致透明） */
.layout-menu /deep/ .el-menu--popup {
  background-color: var(--sidebar-bg) !important;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 8px;
  box-shadow: 0 4px 24px rgba(0, 0, 0, 0.4);
  padding: 4px 0;
  min-width: 160px;
}

.layout-menu /deep/ .el-menu--popup .el-menu-item {
  background: transparent;
  color: var(--sidebar-text);
  height: 40px;
  line-height: 40px;
  font-size: 13px;
  margin: 0;
  border-radius: 0;
  padding-left: 20px !important;
}

.layout-menu /deep/ .el-menu--popup .el-menu-item:hover {
  background: var(--sidebar-bg-hover) !important;
  color: var(--sidebar-text-hover) !important;
}

.layout-menu /deep/ .el-menu--popup .el-menu-item.is-active {
  background: var(--sidebar-bg-active) !important;
  color: var(--sidebar-text-active) !important;
}

/* 外部链接 */
.el-menu-item a {
  display: block;
  color: inherit;
  text-decoration: none;
}
</style>
