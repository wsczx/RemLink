<template>

  <div class="layout-header">
    <!-- 左侧：折叠按钮 + 面包屑 -->
    <div class="header-left">
      <i @click="toggleClick"
        :class="toggleIconClass"
        class="toggle-icon"></i>
      <el-breadcrumb separator-class="el-icon-arrow-right" class="app-breadcrumb">
        <el-breadcrumb-item v-for="(item, index) in route_name" :key="index">{{ item }}</el-breadcrumb-item>
      </el-breadcrumb>
    </div>

    <!-- 右侧：暗色切换 + 用户信息 + 下拉菜单 -->
    <div class="header-right">
      <div class="theme-toggle" @click="toggleDarkMode" :title="isDark ? '切换亮色' : '切换暗色'">
        <i :class="isDark ? 'el-icon-sunny' : 'el-icon-moon'"></i>
      </div>

      <el-dropdown trigger="click" @command="handleCommand">
        <span class="user-info">
          <i class="el-icon-user-solid user-avatar"></i>
          <span class="user-name">{{ admin_user }}</span>
          <i class="el-icon-arrow-down el-icon--right"></i>
        </span>
        <el-dropdown-menu slot="dropdown">
          <el-dropdown-item command="security" icon="el-icon-setting">安全设置</el-dropdown-item>
          <el-dropdown-item command="logout" icon="el-icon-switch-button" divided>退出登录</el-dropdown-item>
        </el-dropdown-menu>
      </el-dropdown>
    </div>

  </div>

</template>

<script>
import {getUser, logout as doLogout} from "@/plugins/auth";

export default {
  name: "Layoutheader",
  props: ['is_active', 'route_name', 'is_mobile'],
  data() {
    return {
      isDark: false,
      mqListener: null,
    }
  },
  computed: {
    admin_user() {
      return getUser();
    },
    toggleIconClass() {
      if (this.is_mobile) {
        return 'el-icon-s-unfold'
      }
      return this.is_active ? 'el-icon-s-fold' : 'el-icon-s-unfold'
    },
  },
  mounted() {
    this.isDark = document.documentElement.classList.contains('dark');
    // 监听系统主题变化（用户未手动设置时跟随）
    this.mqListener = (e) => {
      if (localStorage.getItem('dark-mode') === null) {
        this.isDark = e.matches;
        this.applyDarkMode();
      }
    };
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', this.mqListener);
  },
  beforeDestroy() {
    if (this.mqListener) {
      window.matchMedia('(prefers-color-scheme: dark)').removeEventListener('change', this.mqListener);
    }
  },
  methods: {
    toggleClick() {
      if (this.is_mobile) {
        this.$emit('toggle')
      } else {
        this.$emit('update:is_active', !this.is_active)
      }
    },
    toggleDarkMode() {
      this.isDark = !this.isDark;
      localStorage.setItem('dark-mode', this.isDark);
      this.applyDarkMode();
    },
    applyDarkMode() {
      if (this.isDark) {
        document.documentElement.classList.add('dark');
      } else {
        document.documentElement.classList.remove('dark');
      }
    },
    handleCommand(cmd) {
      if (cmd === 'security') {
        this.$router.push("/admin/set/security")
      } else if (cmd === 'logout') {
        this.$confirm('确定要退出登录吗？', '退出确认', {
          confirmButtonText: '退出',
          cancelButtonText: '取消',
          type: 'warning'
        }).then(() => {
          doLogout()
          this.$router.push("/login");
        }).catch(() => {})
      }
    },
  }
}
</script>

<style scoped>
.layout-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  height: 100%;
}

.header-left {
  display: flex;
  align-items: center;
  overflow: hidden;
  flex: 1;
  min-width: 0;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
  margin-left: 10px;
}

.theme-toggle {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 6px;
  cursor: pointer;
  transition: all var(--transition-fast);
  color: var(--text-regular);
}
.theme-toggle:hover {
  background: var(--bg-hover);
  color: var(--color-primary);
}
.theme-toggle i {
  font-size: 18px;
}

.toggle-icon {
  font-size: 22px;
  cursor: pointer;
  color: var(--text-regular);
  padding: 6px 8px;
  border-radius: 6px;
  transition: all var(--transition-fast);
  flex-shrink: 0;
}
.toggle-icon:hover {
  background: var(--bg-hover);
  color: var(--color-primary);
}

.app-breadcrumb {
  display: inline-block;
  font-size: 14px;
  margin-left: 16px;
  padding-left: 16px;
  border-left: 1px solid var(--border-color-light);
  overflow: hidden;
  white-space: nowrap;
}

.user-info {
  display: flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  padding: 4px 12px;
  border-radius: 6px;
  transition: all var(--transition-fast);
  font-size: 13px;
  color: var(--text-regular);
  white-space: nowrap;
}
.user-info:hover {
  background: var(--bg-hover);
  color: var(--color-primary);
}
.user-avatar {
  font-size: 18px;
  color: var(--color-primary);
}
.user-name {
  font-weight: 500;
}

/* 移动端适配 */
@media (max-width: 768px) {
  .app-breadcrumb {
    margin-left: 10px;
    padding-left: 10px;
    font-size: 13px;
  }

  .user-name {
    display: none;
  }

  .user-info {
    padding: 4px 8px;
  }
}
</style>
