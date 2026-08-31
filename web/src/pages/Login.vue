<template>

  <div class="login-page">
    <!-- 暗色模式切换 -->
    <div class="theme-toggle-fixed" @click="toggleDarkMode" :title="isDark ? '切换亮色' : '切换暗色'">
      <i :class="isDark ? 'el-icon-sunny' : 'el-icon-moon'"></i>
    </div>

    <!-- 背景装饰 -->
    <div class="login-bg">
      <div class="bg-circle bg-circle-1"></div>
      <div class="bg-circle bg-circle-2"></div>
      <div class="bg-circle bg-circle-3"></div>
    </div>

    <!-- 登录卡片 -->
    <div class="login-card">
      <div class="login-header">
        <img v-if="brand.logo" :src="brand.logo" class="login-logo-img" alt="logo" />
        <img v-else :src="baseUrl + (isDark ? 'logo-dark' : 'logo') + '.svg'" class="login-logo-img" alt="logo" />
        <h1 class="login-title">{{ brand.title || 'RemLink' }}</h1>
        <p class="login-subtitle">{{ brand.desc || '企业级安全远程接入网关管理后台' }}</p>
      </div>

      <!-- 密码输入步骤 -->
      <el-form v-show="!otpStep" :model="ruleForm" status-icon :rules="rules" ref="ruleForm" class="login-form"
        @submit.native.prevent>
        <el-form-item prop="admin_user">
          <el-input v-model="ruleForm.admin_user" placeholder="请输入管理用户名" prefix-icon="el-icon-user-solid"
            @keydown.enter.native.prevent="submitForm('ruleForm')">
          </el-input>
        </el-form-item>
        <el-form-item prop="admin_pass">
          <el-input type="password" v-model="ruleForm.admin_pass" autocomplete="off" placeholder="请输入管理密码"
            prefix-icon="el-icon-lock" @keydown.enter.native.prevent="submitForm('ruleForm')">
          </el-input>
        </el-form-item>
        <el-form-item class="login-actions">
          <el-button type="primary" :loading="isLoading" @click="submitForm('ruleForm')" class="btn-login-full"
            native-type="button">
            登 录
          </el-button>
        </el-form-item>
      </el-form>

      <!-- OTP 输入步骤 -->
      <div v-show="otpStep" class="otp-section">
        <div class="otp-header">
          <i class="el-icon-mobile-phone otp-icon"></i>
          <p class="otp-title">请输入动态验证码</p>
          <p class="otp-desc">已启用二次认证，请输入 6 位 TOTP 动态码</p>
        </div>
        <el-form :model="otpForm" ref="otpForm" class="otp-form" @submit.native.prevent>
          <el-form-item>
            <el-input v-model="otpForm.otp_code" placeholder="6位动态验证码" maxlength="6" prefix-icon="el-icon-key"
              @keydown.enter.native.prevent="submitOtp" class="otp-input">
            </el-input>
          </el-form-item>
          <el-form-item class="otp-actions">
            <el-button type="primary" :loading="otpLoading" @click="submitOtp" class="btn-otp-confirm"
              native-type="button">
              验 证
            </el-button>
            <el-button @click="otpStep = false" class="btn-otp-back" native-type="button">
              返 回
            </el-button>
          </el-form-item>
        </el-form>
      </div>

      <!-- 底部提示 -->
      <div class="login-footer">
        <div v-if="brand.footer" class="login-footer-text" v-html="brand.footer"></div>
        <p>忘记管理员密码？请在服务器停止服务后执行密码重置命令，再重启服务：</p>
        <code class="reset-cmd">remlink --reset-admin-password</code>
      </div>
    </div>
  </div>

</template>

<script>
import axios from "axios";
import qs from "qs";
import { applyBrandToDocument } from "../plugins/brand";
import { setUser, checkAuth } from "@/plugins/auth";

export default {
  name: "Login",
  data() {
    return {
      ruleForm: {},
      isDark: false,
      brand: { title: "", logo: "" },
      isLoading: false,
      otpStep: false,
      otpLoading: false,
      otpToken: '',
      otpForm: { otp_code: '' },
      rules: {
        admin_user: [
          { required: true, message: '请输入用户名', trigger: 'blur' },
          { max: 50, message: '长度小于 50 个字符', trigger: 'blur' }
        ],
        admin_pass: [
          { required: true, message: '请输入密码', trigger: 'blur' },
          { min: 6, message: '长度大于 6 个字符', trigger: 'blur' }
        ],
      },
    }
  },
  mounted() {
    this.isDark = document.documentElement.classList.contains('dark');
    this.loadBrand();
  },
  methods: {
    toggleDarkMode() {
      this.isDark = !this.isDark;
      localStorage.setItem('dark-mode', this.isDark);
      document.documentElement.classList.toggle('dark', this.isDark);
    },
    loadBrand() {
      axios.get('/portal/api/login-config').then(resp => {
        if (resp.data && resp.data.data) {
          this.brand = Object.assign({ title: "", logo: "", favicon: "" }, resp.data.data)
          applyBrandToDocument(this.brand)
        }
      }).catch(() => { })
    },
    async handleLoginSuccess(rdata) {
      this.$message.success('登录成功');
      setUser(rdata.admin_user);

      // 先验证 JWT cookie 已生效，防止导航后首个 API 请求 401 被拦截跳回登录页
      const ok = await checkAuth();
      if (!ok) {
        this.$message.error('登录验证失败，请重试');
        return;
      }

      if (rdata.admin_temp) {
        this.$alert('当前管理员密码是首次生成或命令重置后的临时密码。请立即修改密码，否则退出后台后将无法再次查看该临时密码。', '请修改管理员密码', {
          confirmButtonText: '去修改',
          type: 'warning',
          callback: () => {
            this.$router.push('/admin/set/security')
          }
        })
        return
      }
      this.$router.push("/admin/home");
    },
    submitForm(formName) {
      if (this.isLoading) return; // 防止重复提交
      this.$refs[formName].validate((valid) => {
        if (!valid) {
          return false;
        }
        this.isLoading = true

        axios.post('/base/login', qs.stringify(this.ruleForm)).then(resp => {
          var rdata = resp.data
          if (rdata.code === 0) {
            var d = rdata.data
            // OTP 已启用，进入第二步
            if (d.otp_required) {
              this.otpToken = d.otp_token
              this.otpForm.otp_code = ''
              this.otpStep = true
              this.$nextTick(() => {
                var input = document.querySelector('.otp-input input')
                if (input) input.focus()
              })
              return
            }
            // 无需 OTP，直接登录
            this.handleLoginSuccess(d)
          } else {
            this.$message.error(rdata.msg);
          }
        }).catch(() => {
          this.$message.error('请求出错，请检查网络连接');
        }).finally(() => {
          this.isLoading = false
        });
      });
    },
    submitOtp() {
      if (this.otpLoading) return; // 防止重复提交
      if (!this.otpForm.otp_code || this.otpForm.otp_code.length !== 6) {
        this.$message.warning('请输入 6 位动态验证码');
        return;
      }
      this.otpLoading = true
      axios.post('/base/login/otp', {
        otp_token: this.otpToken,
        otp_code: this.otpForm.otp_code,
      }).then(resp => {
        var rdata = resp.data
        if (rdata.code === 0) {
          var d = rdata.data
          this.handleLoginSuccess(d)
        } else {
          this.$message.error(rdata.msg);
        }
      }).catch(() => {
        this.$message.error('请求出错，请检查网络连接');
      }).finally(() => {
        this.otpLoading = false
      });
    },
  },
}
</script>

<style scoped>
.login-page {
  height: 100%;
  display: flex;
  justify-content: center;
  align-items: center;
  background: linear-gradient(135deg, #1b2138 0%, #2a3a5c 40%, #1a3668 100%);
  position: relative;
  overflow: hidden;
}

/* 背景装饰圆 */
.login-bg {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

.bg-circle {
  position: absolute;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.03);
}

.bg-circle-1 {
  width: 500px;
  height: 500px;
  top: -150px;
  right: -100px;
}

.bg-circle-2 {
  width: 300px;
  height: 300px;
  bottom: -80px;
  left: -60px;
}

.bg-circle-3 {
  width: 200px;
  height: 200px;
  bottom: 30%;
  right: 15%;
  background: rgba(64, 158, 255, 0.06);
}

/* 登录卡片 */
.login-card {
  position: relative;
  z-index: 1;
  width: 420px;
  padding: 48px 40px 36px;
  background: var(--bg-card);
  border-radius: 16px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3), 0 0 0 1px rgba(255, 255, 255, 0.08);
}

.login-header {
  text-align: center;
  margin-bottom: 36px;
}

.login-logo {
  font-size: 44px;
  color: var(--color-primary);
  margin-bottom: 12px;
}

.login-logo-img {
  width: 48px;
  height: 48px;
  object-fit: contain;
  margin-bottom: 12px;
}

.login-title {
  font-size: 28px;
  font-weight: 700;
  color: var(--text-primary);
  margin: 0 0 8px 0;
  letter-spacing: 2px;
}

.login-subtitle {
  font-size: 14px;
  color: var(--text-secondary);
  margin: 0;
}

/* 表单 */
.login-form {
  margin-top: 8px;
}

.login-form /deep/ .el-input__inner {
  height: 44px;
  line-height: 44px;
  font-size: 14px;
  padding-left: 40px;
}

.login-form /deep/ .el-input__prefix {
  left: 12px;
}

.login-form /deep/ .el-input__prefix .el-icon-user-solid,
.login-form /deep/ .el-input__prefix .el-icon-lock {
  font-size: 18px;
  color: var(--text-secondary);
}

.login-actions {
  margin-top: 28px;
}

.btn-login-full {
  width: 100%;
  height: 44px;
  font-size: 15px;
  letter-spacing: 4px;
  font-weight: 600;
}

/* OTP 验证步骤 */
.otp-section {
  text-align: center;
}

.otp-header {
  margin-bottom: 28px;
}

.otp-icon {
  font-size: 48px;
  color: var(--color-primary);
  margin-bottom: 12px;
}

.otp-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0 0 8px 0;
}

.otp-desc {
  font-size: 13px;
  color: var(--text-secondary);
  margin: 0;
}

.otp-form {
  margin-top: 8px;
}

.otp-form /deep/ .el-input__inner {
  height: 48px;
  line-height: 48px;
  font-size: 20px;
  text-align: center;
  letter-spacing: 8px;
  padding-left: 40px;
}

.otp-actions {
  margin-top: 24px;
}

.btn-otp-confirm,
.btn-otp-back {
  width: calc(50% - 6px);
  height: 44px;
  font-size: 15px;
  letter-spacing: 4px;
  font-weight: 600;
}

.btn-otp-confirm {
  float: left;
}

.btn-otp-back {
  float: right;
}

/* 底部提示 */
.login-footer {
  margin-top: 24px;
  padding-top: 20px;
  border-top: 1px solid var(--border-color-light);
  text-align: center;
}

.login-footer p {
  font-size: 12px;
  color: var(--text-secondary);
  margin: 0 0 8px 0;
}

.login-footer-text {
  font-size: 12px;
  color: var(--text-secondary);
  margin: 0 0 10px 0;
}

.reset-cmd {
  display: inline-block;
  padding: 4px 12px;
  background: var(--bg-hover);
  border: 1px solid #e4e7ed;
  border-radius: 4px;
  font-size: 12px;
  color: var(--text-primary);
  font-family: Menlo, Monaco, Consolas, monospace;
  user-select: all;
}

/* 移动端适配 */
@media (max-width: 480px) {
  .login-card {
    width: 90%;
    padding: 36px 24px 28px;
  }

  .login-title {
    font-size: 24px;
  }

  .login-subtitle {
    font-size: 13px;
  }

  .login-form /deep/ .el-input__inner {
    height: 42px;
    line-height: 42px;
    font-size: 13px;
  }

  .btn-login-full {
    height: 42px;
    font-size: 14px;
  }

  .reset-cmd {
    font-size: 11px;
    padding: 3px 8px;
  }
}
</style>

<style>
/* ========== Login 暗色模式 ========== */
.theme-toggle-fixed {
  position: fixed;
  top: 20px;
  right: 20px;
  z-index: 100;
  width: 38px;
  height: 38px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  background: rgba(255, 255, 255, 0.12);
  backdrop-filter: blur(8px);
  color: rgba(255, 255, 255, 0.8);
  font-size: 18px;
  transition: all var(--transition-fast);
}

.theme-toggle-fixed:hover {
  background: rgba(255, 255, 255, 0.2);
  color: var(--text-inverse);
}

html.dark .login-card {
  background: var(--bg-card) !important;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5) !important;
}

html.dark .login-card .el-input__inner {
  background: var(--bg-hover) !important;
  border-color: var(--border-color) !important;
  color: var(--text-primary) !important;
}

html.dark .login-title,
html.dark .otp-title {
  color: var(--text-primary) !important;
}

html.dark .login-subtitle,
html.dark .otp-desc {
  color: var(--text-secondary) !important;
}
</style>
