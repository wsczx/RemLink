<template>
  <div class="webauth-page">
    <!-- 暗色模式切换 -->
    <div class="theme-toggle-fixed" @click="toggleDarkMode" :title="isDark ? '切换亮色' : '切换暗色'">
      <i :class="isDark ? 'el-icon-sunny' : 'el-icon-moon'"></i>
    </div>

    <!-- 背景装饰 -->
    <div class="webauth-bg">
      <div class="bg-circle bg-circle-1"></div>
      <div class="bg-circle bg-circle-2"></div>
      <div class="bg-circle bg-circle-3"></div>
    </div>

    <div class="webauth-card">
      <!-- 品牌头部 -->
      <div class="webauth-header">
        <div class="brand-icon-wrap">
          <img v-if="brand.logo" :src="brand.logo" class="brand-logo-img" alt="logo" />
          <img v-else :src="baseUrl + (isDark ? 'logo-dark' : 'logo') + '.svg'" class="brand-logo-img" alt="logo" />
        </div>
        <h1 class="brand-name">{{ brand.title || 'RemLink' }}</h1>
        <p class="brand-desc">{{ brand.desc || '企业级安全远程接入网关认证' }}</p>
      </div>

      <!-- 步骤-1：输入用户名以过滤组 -->
      <div v-show="step === 'identify'" class="step-body">
        <p class="step-desc">请输入用户名以加载您可接入的用户组</p>
        <el-form :model="identifyForm" class="auth-form" @submit.native.prevent>
          <el-form-item>
            <el-input v-model="identifyForm.username" placeholder="用户名" prefix-icon="el-icon-user-solid"
              @keydown.enter.native.prevent="submitIdentify" />
          </el-form-item>
        </el-form>
        <div class="step-actions">
          <el-button type="primary" :loading="loading" :disabled="!identifyForm.username" @click="submitIdentify"
            class="btn-full" native-type="button">继续</el-button>
        </div>
      </div>

      <!-- 步骤0：组选择 -->
      <div v-show="step === 'select_group'" class="step-body">
        <p class="step-desc">请选择您所属的用户组以继续身份认证</p>
        <div class="group-grid">
          <div v-for="(g, idx) in groups" :key="g" class="group-card" :class="{ active: selectedGroup === g }"
            :style="{ animationDelay: `${idx * 0.06}s` }" @click="selectedGroup = g">
            <div class="group-card-icon" :class="`icon-${idx % 5}`">
              <i :class="groupIcon(idx)"></i>
            </div>
            <div class="group-card-body">
              <span class="group-card-name">{{ g }}</span>
              <span class="group-card-hint">点击选择此组</span>
            </div>
            <div v-if="selectedGroup === g" class="group-card-check">
              <i class="el-icon-check"></i>
            </div>
          </div>
        </div>
        <div class="step-actions">
          <el-button type="primary" :disabled="!selectedGroup" :loading="loading" @click="submitGroup"
            class="btn-full">下一步</el-button>
        </div>
      </div>

      <!-- 步骤1：手机号输入（仅 SMS 首步） -->
      <div v-show="step === 'sms_phone'" class="step-body">
        <p class="step-desc">{{ hint || '请输入手机号以接收短信验证码' }}</p>
        <el-form :model="phoneForm" class="auth-form" @submit.native.prevent>
          <el-form-item>
            <el-input v-model="phoneForm.phone" placeholder="手机号" maxlength="11" prefix-icon="el-icon-phone-outline"
              class="phone-input" @keydown.enter.native.prevent="submitSmsPhone" />
          </el-form-item>
        </el-form>
        <div class="step-actions">
          <el-button type="primary" :loading="loading" :disabled="!isPhoneValid" @click="submitSmsPhone"
            class="btn-full" native-type="button">获取验证码</el-button>
        </div>
      </div>

      <!-- 步骤2：凭据输入（local/ldap 用户名密码） -->
      <div v-show="step === 'credentials'" class="step-body">
        <p class="step-desc">{{ hint || '请输入登录凭据' }}</p>
        <el-form :model="credForm" ref="credForm" class="auth-form" @submit.native.prevent>
          <el-form-item>
            <el-input v-model="credForm.username" placeholder="用户名" prefix-icon="el-icon-user-solid"
              :disabled="!!usernameField" @keydown.enter.native.prevent="submitCredentials" />
          </el-form-item>
          <el-form-item v-if="!noPassword">
            <el-input v-model="credForm.password" type="password" placeholder="密码" prefix-icon="el-icon-lock"
              show-password @keydown.enter.native.prevent="submitCredentials" />
          </el-form-item>
        </el-form>
        <div class="step-actions">
          <el-button type="primary" :loading="loading" :disabled="noPassword ? !credForm.username : !credForm.password"
            @click="submitCredentials" class="btn-full" native-type="button">继 续</el-button>
        </div>
      </div>

      <!-- 步骤2：OTP 输入 -->
      <div v-show="step === 'otp'" class="step-body">
        <div class="otp-header">
          <i class="el-icon-mobile-phone otp-icon"></i>
          <p class="otp-title">{{ hint || '请输入 6 位动态验证码' }}</p>
          <p class="otp-desc">已启用 OTP 二次认证，请输入手机令牌动态码</p>
        </div>
        <el-form :model="otpForm" ref="otpForm" class="auth-form" @submit.native.prevent>
          <el-form-item>
            <el-input v-model="otpForm.code" placeholder="6位动态验证码" maxlength="6" prefix-icon="el-icon-key"
              @keydown.enter.native.prevent="submitOtp" class="otp-input" />
          </el-form-item>
        </el-form>
        <div class="step-actions">
          <el-button type="primary" :loading="loading" :disabled="otpForm.code.length < 6" @click="submitOtp"
            class="btn-full" native-type="button">验 证</el-button>
        </div>
      </div>

      <!-- 步骤2.5：SMS 短信验证码 -->
      <div v-show="step === 'sms'" class="step-body">
        <div class="otp-header">
          <i class="el-icon-mobile-phone otp-icon"></i>
          <p class="otp-title">{{ hint || '请输入短信验证码' }}</p>
          <p class="otp-desc">验证码已发送至 {{ phoneMasked }}</p>
        </div>
        <el-form :model="smsForm" class="auth-form" @submit.native.prevent>
          <el-form-item>
            <el-input v-model="smsForm.code" placeholder="6位验证码" maxlength="6" prefix-icon="el-icon-edit-outline"
              class="otp-input" @keydown.enter.native.prevent="submitSmsCode" />
          </el-form-item>
        </el-form>
        <div class="step-actions">
          <el-button type="primary" :loading="loading" :disabled="smsForm.code.length < 6" @click="submitSmsCode"
            class="btn-full" native-type="button">验 证</el-button>
        </div>
        <div class="sms-resend-wrap">
          <a class="sms-resend-link" :class="{ 'sms-resend-disabled': smsCountdown > 0 }" @click="resendSmsCode">{{
            smsCountdown > 0 ? smsCountdown + 's 后重新发送' : '重新发送验证码' }}</a>
        </div>
      </div>

      <!-- 步骤3：RADIUS 二次验证 -->
      <div v-show="step === 'radius'" class="step-body">
        <div class="otp-header">
          <i class="el-icon-lock otp-icon"></i>
          <p class="otp-title">{{ challengeMsg || '请输入二次验证码' }}</p>
          <p class="otp-desc">已启用 RADIUS 二次认证，请输入动态验证码</p>
        </div>
        <el-form :model="credForm" ref="radiusForm" class="auth-form" @submit.native.prevent>
          <el-form-item>
            <el-input v-model="credForm.password" type="password" placeholder="二次验证码" prefix-icon="el-icon-lock"
              show-password @keydown.enter.native.prevent="submitCredentials" />
          </el-form-item>
        </el-form>
        <div class="step-actions">
          <el-button type="primary" :loading="loading" :disabled="!credForm.password" @click="submitCredentials"
            class="btn-full" native-type="button">提 交</el-button>
        </div>
      </div>

      <!-- 步骤2.6：强制改密（首次登录） -->
      <div v-show="step === 'change_pwd'" class="step-body">
        <div class="otp-header">
          <i class="el-icon-warning-outline otp-icon"></i>
          <p class="otp-title">{{ hint || '首次登录，请修改密码' }}</p>
          <p class="otp-desc">新密码规则：至少8位且须包含字母和数字</p>
        </div>
        <el-form :model="changePwdForm" class="auth-form" @submit.native.prevent>
          <el-form-item>
            <el-input v-model="changePwdForm.new_password" type="password" placeholder="新密码" prefix-icon="el-icon-lock"
              show-password @keydown.enter.native.prevent="submitChangePwd" />
          </el-form-item>
          <el-form-item>
            <el-input v-model="changePwdForm.confirm_password" type="password" placeholder="确认新密码"
              prefix-icon="el-icon-lock" show-password @keydown.enter.native.prevent="submitChangePwd" />
          </el-form-item>
        </el-form>
        <div class="step-actions">
          <el-button type="primary" :loading="loading"
            :disabled="!changePwdForm.new_password || !changePwdForm.confirm_password" @click="submitChangePwd"
            class="btn-full" native-type="button">修改并继续</el-button>
        </div>
      </div>

      <!-- 完成 -->
      <div v-show="step === 'done'" class="step-body done-box">
        <i class="el-icon-circle-check done-icon"></i>
        <p class="done-text">认证成功</p>
        <p class="done-sub">即将跳转到用户自助门户</p>
        <el-button type="primary" @click="goPortal" class="btn-full">进入门户</el-button>
      </div>

      <!-- 错误提示 -->
      <div v-if="error" class="step-error">
        <i class="el-icon-warning"></i>
        <span>{{ error }}</span>
      </div>

      <!-- 加载提示 -->
      <div v-if="loadingText" class="step-loading">
        <i class="el-icon-loading"></i>
        <span>{{ loadingText }}</span>
      </div>

      <!-- 自定义页脚（品牌设置中的页脚文本） -->
      <div class="webauth-footer" v-if="brand.footer" v-html="brand.footer">
      </div>
    </div>
  </div>
</template>

<script>
import axios from 'axios'
import { applyBrandToDocument } from "../plugins/brand"

export default {
  name: 'WebAuth',
  data() {
    return {
      step: 'select_group',
      isDark: false,
      brand: { title: "", logo: "" },
      state: '',
      groups: [],
      fallbackGroups: [],
      selectedGroup: '',
      identifyForm: { username: '' },
      hint: '',
      challengeMsg: '',
      usernameField: '',
      noPassword: false,
      loading: false,
      loadingText: '',
      error: '',
      portalUrl: '',
      credForm: { username: '', password: '' },
      changePwdForm: { new_password: '', confirm_password: '' },
      phoneForm: { phone: '' },
      otpForm: { code: '' },
      smsForm: { code: '' },
      phoneMasked: '',
      smsCountdown: 0,
      smsCountdownTimer: null,
    }
  },
  computed: {
    isPhoneValid() {
      return /^1[3-9]\d{9}$/.test(this.phoneForm.phone)
    },
  },
  created() {
    this.state = this.$route.query.state || ''
    if (!this.state) {
      this.error = '缺少认证参数，请重新发起连接'
      return
    }
    // SSO 回调后继续（整页跳转方式的兼容入口）
    if (this.$route.path === '/web-auth/continue') {
      this.continueAfterSSO()
      return
    }
    this.loadBrand()
    this.initAuth()
  },
  beforeDestroy() {
    clearInterval(this.smsCountdownTimer)
  },
  mounted() {
    this.isDark = document.documentElement.classList.contains('dark');
  },
  methods: {
    toggleDarkMode() {
      this.isDark = !this.isDark;
      localStorage.setItem('dark-mode', this.isDark);
      document.documentElement.classList.toggle('dark', this.isDark);
    },
    apiPath(name) {
      return `/web-auth/${name}?state=${encodeURIComponent(this.state)}`
    },
    loadBrand() {
      axios.get('/portal/api/login-config').then(resp => {
        if (resp.data && resp.data.data) {
          this.brand = Object.assign({ title: "", logo: "", favicon: "" }, resp.data.data)
          applyBrandToDocument(this.brand)
        }
      }).catch(() => { })
    },
    async initAuth() {
      this.error = ''
      this.loadingText = '正在检测证书信息...'
      try {
        const { data } = await axios.get(this.apiPath('start'), { withCredentials: false })
        this.loadingText = ''
        switch (data.status) {
          case 'select_group':
            if (data.require_identify) {
              // 开启组过滤：先收集用户名，再按权限过滤组清单
              this.fallbackGroups = data.groups || []
              this.step = 'identify'
            } else {
              // 关闭组过滤（默认）：直接展示全量启用组，保持旧模式。
              this.groups = data.groups || []
              this.step = 'select_group'
              // 只有一个组时无需人工选择，直接提交
              if (this.groups.length === 1) {
                this.selectedGroup = this.groups[0]
                this.submitGroup()
              }
            }
            return
          case 'identify':
            this.step = 'identify'
            return
          case 'credentials':
            this.enterCredentials(data)
            return
          case 'otp':
            this.enterOtp(data)
            return
          case 'radius':
            this.enterRadius(data)
            return
          case 'sso':
            window.location.href = data.redirect_url
            return
          case 'sms_phone':
            this.enterSmsPhone(data)
            return
          case 'sms':
            this.enterSms(data)
            return
          case 'change_pwd':
            this.enterChangePwd(data)
            return
          case 'done':
            this.onDone(data)
            return
          default:
            this.error = data.message || '未知响应'
        }
      } catch (e) {
        this.loadingText = ''
        this.error = '认证服务异常，请重试'
      }
    },
    enterCredentials(data) {
      this.step = 'credentials'
      this.hint = data.hint || ''
      this.usernameField = data.username || ''
      this.noPassword = data.no_password === true
      if (data.username) {
        this.credForm.username = data.username
      }
    },
    enterSmsPhone(data) {
      this.step = 'sms_phone'
      this.hint = data.hint || ''
      this.phoneForm.phone = ''
    },
    async submitSmsPhone() {
      if (!this.isPhoneValid || this.loading) return
      this.loading = true
      this.error = ''
      try {
        const { data } = await axios.post(this.apiPath('step'), {
          phone: this.phoneForm.phone,
        }, { withCredentials: false })
        this.handleStepResponse(data)
      } catch (e) {
        this.error = '请求失败，请重试'
      } finally {
        this.loading = false
      }
    },
    enterOtp(data) {
      this.step = 'otp'
      this.hint = data.hint || ''
      this.otpForm.code = ''
    },
    enterRadius(data) {
      this.step = 'radius'
      this.challengeMsg = data.challenge_msg || ''
      this.credForm.password = ''
    },
    enterChangePwd(data) {
      this.step = 'change_pwd'
      this.hint = data.hint || ''
      this.changePwdForm.new_password = ''
      this.changePwdForm.confirm_password = ''
    },
    async submitChangePwd() {
      if (this.loading) return
      if (!this.changePwdForm.new_password || !this.changePwdForm.confirm_password) return
      this.loading = true
      this.error = ''
      try {
        const { data } = await axios.post(this.apiPath('change_password'), {
          new_password: this.changePwdForm.new_password,
          new_password_confirm: this.changePwdForm.confirm_password,
        }, { withCredentials: false })
        this.handleStepResponse(data)
      } catch (e) {
        this.error = '修改失败，请重试'
      } finally {
        this.loading = false
      }
    },
    onDone(data) {
      // 重定向到服务端完成端点：设置 acSamlv2Token Cookie 供客户端读取
      if (data.complete_url) {
        window.location.href = data.complete_url
        return
      }
      this.step = 'done'
      this.portalUrl = data.portal_url || '/ui/#/portal'
      setTimeout(() => this.goPortal(), 2000)
    },
    async submitGroup() {
      if (!this.selectedGroup || this.loading) return
      this.loading = true
      this.error = ''
      try {
        const { data } = await axios.post(this.apiPath('groups'), {
          group: this.selectedGroup,
          username: this.credForm.username || '',
        }, { withCredentials: false })
        this.handleStepResponse(data)
      } catch (e) {
        this.error = '请求失败，请重试'
      } finally {
        this.loading = false
      }
    },
    async submitCredentials() {
      if (this.loading) return
      if (!this.noPassword && !this.credForm.password) return
      if (this.noPassword && !this.credForm.username) return
      this.loading = true
      this.error = ''
      try {
        const { data } = await axios.post(this.apiPath('step'), {
          username: this.credForm.username || '',
          password: this.credForm.password,
        }, { withCredentials: false })
        this.handleStepResponse(data)
      } catch (e) {
        this.error = '认证失败，请重试'
      } finally {
        this.loading = false
      }
    },
    async submitOtp() {
      if (this.otpForm.code.length < 6 || this.loading) return
      this.loading = true
      this.error = ''
      try {
        const { data } = await axios.post(this.apiPath('step'), {
          otp_code: this.otpForm.code,
        }, { withCredentials: false })
        this.handleStepResponse(data)
      } catch (e) {
        this.error = '验证失败，请重试'
      } finally {
        this.loading = false
      }
    },
    enterSms(data) {
      this.step = 'sms'
      this.hint = data.hint || '请输入短信验证码'
      this.phoneMasked = data.phone_masked || ''
      this.smsForm.code = ''
      this.startSmsCountdown()
    },
    async submitSmsCode() {
      if (this.smsForm.code.length < 6 || this.loading) return
      this.loading = true
      this.error = ''
      try {
        const { data } = await axios.post(this.apiPath('step'), {
          sms_code: this.smsForm.code,
        }, { withCredentials: false })
        this.handleStepResponse(data)
      } catch (e) {
        this.error = '验证失败，请重试'
      } finally {
        this.loading = false
      }
    },
    async resendSmsCode() {
      if (this.smsCountdown > 0 || this.loading) return
      this.loading = true
      this.error = ''
      try {
        const { data } = await axios.post(this.apiPath('sms/resend'), {}, { withCredentials: false })
        if (data.status === 'ok') {
          this.startSmsCountdown()
        } else {
          this.error = data.message || '重发失败'
        }
      } catch (e) {
        this.error = '重发失败，请重试'
      } finally {
        this.loading = false
      }
    },
    startSmsCountdown() {
      this.smsCountdown = 60
      clearInterval(this.smsCountdownTimer)
      this.smsCountdownTimer = setInterval(() => {
        this.smsCountdown--
        if (this.smsCountdown <= 0) {
          clearInterval(this.smsCountdownTimer)
          this.smsCountdownTimer = null
        }
      }, 1000)
    },
    handleStepResponse(data) {
      switch (data.status) {
        case 'credentials':
          this.credForm.password = ''
          this.enterCredentials(data)
          break
        case 'sms_phone':
          this.enterSmsPhone(data)
          break
        case 'otp':
          this.enterOtp(data)
          break
        case 'sms':
          this.enterSms(data)
          break
        case 'radius':
          this.enterRadius(data)
          break
        case 'change_pwd':
          this.enterChangePwd(data)
          break
        case 'sso':
          window.location.href = data.redirect_url
          break
        case 'done':
          this.onDone(data)
          break
        case 'error':
          this.error = data.message || '认证失败'
          break
        default:
          this.error = data.message || '未知响应'
      }
    },
    async continueAfterSSO() {
      this.loading = true
      this.loadingText = '正在恢复认证流程...'
      this.error = ''
      try {
        const { data } = await axios.post(this.apiPath('continue'), {}, { withCredentials: false })
        this.loadingText = ''
        this.loading = false
        this.handleStepResponse(data)
      } catch (e) {
        this.loading = false
        this.loadingText = ''
        this.error = '恢复认证失败，请重新发起连接'
      }
    },
    goPortal() {
      window.location.href = this.portalUrl
    },
    async submitIdentify() {
      if (!this.identifyForm.username || this.loading) return
      this.loading = true
      this.error = ''
      try {
        const { data } = await axios.post(this.apiPath('identify'), {
          username: this.identifyForm.username,
        }, { withCredentials: false })
        this.loading = false
        if (data.status === 'select_group') {
          this.groups = data.groups || []
          this.step = 'select_group'
          if (this.groups.length === 1) {
            this.selectedGroup = this.groups[0]
            this.submitGroup()
          }
        } else if (data.status === 'error') {
          this.error = data.message || '加载组失败'
        } else {
          this.error = data.message || '未知响应'
        }
      } catch (e) {
        // 旧版服务端无 /web-auth/identify：回退展示 /web-auth/start 返回的全量组
        this.loading = false
        if (this.fallbackGroups && this.fallbackGroups.length) {
          this.groups = this.fallbackGroups
          this.step = 'select_group'
        } else {
          this.error = '请求失败，请重试'
        }
      }
    },
    groupIcon(idx) {
      const icons = ['el-icon-s-grid', 'el-icon-s-platform', 'el-icon-s-cooperation', 'el-icon-s-order', 'el-icon-s-flag']
      return icons[idx % icons.length]
    },

  },
}
</script>

<style scoped>
/* 页面容器 — 深色渐变背景，与门户/管理后台一致 */
.webauth-page {
  min-height: 100vh;
  min-height: 100dvh;
  display: flex;
  justify-content: center;
  align-items: center;
  background: linear-gradient(135deg, #1b2138 0%, #2a3a5c 40%, #1a3668 100%);
  position: relative;
  overflow-x: hidden;
  /* 确保 WebView 中背景铺满 */
  width: 100%;
  box-sizing: border-box;
  padding: 16px;
}

/* 背景装饰圆 */
.webauth-bg {
  position: fixed;
  inset: 0;
  z-index: 0;
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

/* 认证卡片 */
.webauth-card {
  position: relative;
  z-index: 1;
  width: 420px;
  max-width: 100%;
  padding: 40px 40px 32px;
  background: var(--bg-card);
  border-radius: 16px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3), 0 0 0 1px rgba(255, 255, 255, 0.08);
  box-sizing: border-box;
}

/* 自定义页脚（品牌设置中的页脚文本） */
.webauth-footer {
  margin-top: 18px;
  padding-top: 14px;
  border-top: 1px solid var(--border-color-light);
  text-align: center;
  font-size: 12px;
  color: var(--text-secondary);
  line-height: 1.6;
}

/* 品牌头部 */
.webauth-header {
  text-align: center;
  margin-bottom: 32px;
}

.brand-icon-wrap {
  width: 56px;
  height: 56px;
  margin: 0 auto 14px;
  border-radius: 14px;
  background: transparent;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: none;
}

.brand-icon-wrap i {
  font-size: 26px;
  color: var(--text-inverse);
}

.brand-logo-img {
  width: 48px;
  height: 48px;
  object-fit: contain;
}

.brand-name {
  margin: 0;
  font-size: 22px;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: 1px;
}

.brand-desc {
  margin: 6px 0 0;
  font-size: 13px;
  color: var(--text-secondary);
}

/* 步骤内容 */
.step-body {
  min-height: 80px;
}

.step-desc {
  text-align: center;
  font-size: 13px;
  color: var(--text-secondary);
  margin-bottom: 20px;
}

/* 组选择网格 */
.group-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
  margin-bottom: 20px;
  max-height: 300px;
  overflow-y: auto;
  padding: 2px;
}

.group-card {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 18px 12px 14px;
  border: 2px solid var(--border-color-light);
  border-radius: 12px;
  cursor: pointer;
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
  background: var(--bg-card);
  animation: cardIn 0.4s ease-out both;
}

.group-card:hover {
  border-color: var(--color-primary-light);
  background: var(--color-primary-bg);
  transform: translateY(-2px);
  box-shadow: 0 6px 20px rgba(64, 158, 255, 0.1);
}

.group-card.active {
  border-color: var(--color-primary);
  background: linear-gradient(135deg, var(--color-primary-bg) 0%, var(--color-primary-bg) 100%);
  transform: translateY(-2px);
  box-shadow: 0 6px 24px rgba(64, 158, 255, 0.18);
}

.group-card-icon {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.25s;
}

.group-card-icon i {
  font-size: 22px;
  color: var(--text-inverse);
}

.group-card.active .group-card-icon {
  transform: scale(1.08);
  box-shadow: 0 4px 12px rgba(64, 158, 255, 0.3);
}

/* 不同颜色方案 */
.group-card-icon.icon-0 {
  background: linear-gradient(135deg, var(--color-primary), #66b1ff);
}

.group-card-icon.icon-1 {
  background: linear-gradient(135deg, var(--color-success), #85ce61);
}

.group-card-icon.icon-2 {
  background: linear-gradient(135deg, var(--color-warning), #ebb563);
}

.group-card-icon.icon-3 {
  background: linear-gradient(135deg, var(--color-danger), #f89898);
}

.group-card-icon.icon-4 {
  background: linear-gradient(135deg, #6366f1, #818cf8);
}

.group-card-body {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
}

.group-card-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  text-align: center;
  word-break: break-all;
  line-height: 1.3;
}

.group-card.active .group-card-name {
  color: var(--color-primary);
}

.group-card-hint {
  font-size: 11px;
  color: var(--text-placeholder);
}

.group-card.active .group-card-hint {
  color: var(--color-primary);
  opacity: 0.7;
}

.group-card-check {
  position: absolute;
  top: -6px;
  right: -6px;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--color-primary);
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 2px 8px rgba(64, 158, 255, 0.35);
  animation: popIn 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.group-card-check i {
  font-size: 12px;
  color: var(--text-inverse);
  font-weight: bold;
}

@keyframes cardIn {
  from {
    opacity: 0;
    transform: translateY(12px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes popIn {
  from {
    transform: scale(0);
  }

  to {
    transform: scale(1);
  }
}

/* 表单样式 — 与登录页一致 */
.auth-form {
  margin-bottom: 8px;
}

.auth-form /deep/ .el-input__inner {
  height: 44px;
  line-height: 44px;
  font-size: 14px;
  padding-left: 40px;
}

.auth-form /deep/ .el-input__prefix {
  left: 12px;
}

.auth-form /deep/ .el-input__prefix i {
  font-size: 18px;
  color: var(--text-secondary);
}

/* OTP 头部 */
.otp-header {
  text-align: center;
  margin-bottom: 24px;
}

.otp-icon {
  font-size: 48px;
  color: var(--color-primary);
  margin-bottom: 12px;
}

.otp-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0 0 8px 0;
}

.otp-desc {
  font-size: 13px;
  color: var(--text-secondary);
  margin: 0;
}

/* OTP 输入框 — 大号居中 */
.otp-input /deep/ .el-input__inner {
  height: 48px;
  line-height: 48px;
  font-size: 20px;
  text-align: center;
  letter-spacing: 8px;
  padding-left: 40px;
}

/* 手机号输入框 */
.phone-input /deep/ .el-input__inner {
  height: 48px;
  line-height: 48px;
  font-size: 20px;
  text-align: center;
  letter-spacing: 2px;
}

/* SMS 重发链接 */
.sms-resend-wrap {
  text-align: center;
  margin-top: 12px;
}

.sms-resend-link {
  font-size: 13px;
  color: var(--color-primary);
  cursor: pointer;
  text-decoration: none;
}

.sms-resend-link:hover {
  color: #66b1ff;
}

.sms-resend-disabled {
  color: var(--text-placeholder);
  cursor: not-allowed;
}

/* 操作区 */
.step-actions {
  margin-top: 20px;
}

.btn-full {
  width: 100%;
  height: 44px;
  font-size: 15px;
  letter-spacing: 4px;
  font-weight: 600;
}

/* 完成状态 */
.done-box {
  text-align: center;
}

.done-icon {
  font-size: 56px;
  color: var(--color-success);
  margin-bottom: 14px;
}

.done-text {
  font-size: 20px;
  color: var(--text-primary);
  font-weight: 600;
  margin-bottom: 6px;
}

.done-sub {
  font-size: 13px;
  color: var(--text-secondary);
  margin-bottom: 24px;
}

/* 错误提示 */
.step-error {
  margin-top: 16px;
  padding: 10px 14px;
  background: var(--danger-bg);
  border: 1px solid #fde2e2;
  border-radius: 6px;
  color: var(--color-danger);
  font-size: 13px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.step-error i {
  font-size: 16px;
  flex-shrink: 0;
}

/* 加载提示 */
.step-loading {
  margin-top: 16px;
  text-align: center;
  color: var(--text-secondary);
  font-size: 13px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
}

.step-loading i {
  animation: el-rotate 2s linear infinite;
}

@keyframes el-rotate {
  from {
    transform: rotate(0deg);
  }

  to {
    transform: rotate(360deg);
  }
}

/* 移动端适配 — Cisco AnyConnect 内置浏览器 */
@media (max-width: 480px) {
  .webauth-page {
    padding: 0;
    align-items: stretch;
  }

  .webauth-card {
    width: 100%;
    max-width: 100%;
    border-radius: 0;
    padding: 28px 20px 24px;
    min-height: 100vh;
    min-height: 100dvh;
    display: flex;
    flex-direction: column;
    justify-content: center;
    box-shadow: none;
    position: relative;
  }

  /* 移动端卡片全屏白色，切换按钮移到卡片内部 */
  .theme-toggle-fixed {
    position: absolute;
    background: transparent;
    backdrop-filter: none;
    color: var(--text-secondary);
  }

  .theme-toggle-fixed:hover {
    background: var(--bg-hover);
    color: var(--color-primary);
  }

  .webauth-header {
    margin-bottom: 24px;
  }

  .brand-icon-wrap {
    width: 52px;
    height: 52px;
    border-radius: 12px;
  }

  .brand-icon-wrap i {
    font-size: 22px;
  }

  .brand-name {
    font-size: 20px;
  }

  .group-grid {
    grid-template-columns: 1fr;
    gap: 8px;
    max-height: none;
  }

  .group-card {
    flex-direction: row;
    padding: 14px 16px;
    gap: 12px;
  }

  .group-card-body {
    flex-direction: row;
    align-items: center;
    flex: 1;
    justify-content: space-between;
  }

  .group-card-hint {
    display: none;
  }

  .auth-form /deep/ .el-input__inner {
    height: 48px;
    line-height: 48px;
    font-size: 16px;
  }

  .btn-full {
    height: 48px;
    font-size: 16px;
  }
}
</style>

<style>
/* ========== WebAuth 暗色模式 ========== */
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

html.dark .webauth-card {
  background: var(--bg-card) !important;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5) !important;
}

html.dark .webauth-card .el-input__inner {
  background: var(--bg-hover) !important;
  border-color: var(--border-color) !important;
  color: var(--text-primary) !important;
}

html.dark .webauth-card .el-input__inner::placeholder {
  color: var(--text-placeholder) !important;
}

html.dark .step-desc,
html.dark .brand-name,
html.dark .group-card-name,
html.dark .otp-title,
html.dark .success-title {
  color: var(--text-primary) !important;
}

html.dark .brand-desc,
html.dark .group-card-desc,
html.dark .otp-desc,
html.dark .success-desc,
html.dark .step-hint {
  color: var(--text-secondary) !important;
}

html.dark .group-card {
  background: var(--bg-hover) !important;
  border-color: var(--border-color-light) !important;
}

html.dark .group-card:hover {
  border-color: var(--color-primary) !important;
  background: var(--color-primary-bg) !important;
}

html.dark .group-card.active {
  border-color: var(--color-primary) !important;
  background: var(--color-primary-bg) !important;
}

html.dark .otp-input .el-input__inner {
  background: var(--bg-hover) !important;
  border-color: var(--border-color) !important;
  color: var(--text-primary) !important;
}

html.dark .webauth-card .el-button--default {
  background: var(--bg-card) !important;
  color: var(--text-regular) !important;
  border-color: var(--border-color) !important;
}
</style>
