<template>
  <div class="portal-page" :class="{ 'portal-logged-in': loggedIn }">
    <!-- 校验登录态中：显示加载占位，避免刷新瞬间闪现登录卡片 -->
    <template v-if="checking">
      <div
        style="min-height:100vh;display:flex;align-items:center;justify-content:center;font-size:32px;color:var(--color-primary,#409eff);">
        <i class="el-icon-loading"></i>
      </div>
    </template>
    <!-- 登录页 -->
    <template v-else-if="!loggedIn">
      <!-- 暗色模式切换（登录页） -->
      <div class="theme-toggle-fixed" @click="toggleDarkMode" :title="isDark ? '切换亮色' : '切换暗色'">
        <i :class="isDark ? 'el-icon-sunny' : 'el-icon-moon'"></i>
      </div>
      <div class="portal-login-bg">
        <div class="bg-circle bg-circle-1"></div>
        <div class="bg-circle bg-circle-2"></div>
        <div class="bg-circle bg-circle-3"></div>
      </div>

      <div class="portal-login-card">
        <div class="portal-brand">
          <div class="brand-icon-wrap">
            <img v-if="brand.logo" :src="brand.logo" class="brand-logo-img" alt="logo" />
            <img v-else :src="baseUrl + (isDark ? 'logo-dark' : 'logo') + '.svg'" class="brand-logo-img" alt="logo" />
          </div>
          <h1 class="brand-name">{{ brand.title || 'RemLink' }}</h1>
          <p class="brand-desc">{{ brand.desc || '企业级安全远程接入网关用户门户' }}</p>
        </div>

        <div class="portal-features" v-if="showFeatures">
          <div class="feat-item" v-for="(f, i) in displayFeatures" :key="i">
            <div class="feat-icon" :class="featureIcons[i].cls"><i :class="featureIcons[i].icon"></i></div>
            <div class="feat-text">
              <span class="feat-label">{{ f.label }}</span>
              <span class="feat-desc">{{ f.desc }}</span>
            </div>
          </div>
        </div>

        <el-form v-if="loginMode === 'login'" :model="loginForm" :rules="loginRules" ref="loginForm"
          @submit.native.prevent class="portal-login-form">
          <el-form-item prop="username">
            <el-input v-model="loginForm.username" prefix-icon="el-icon-user-solid" placeholder="用户名"
              @keydown.enter.native.prevent="submitLogin" />
          </el-form-item>
          <el-form-item prop="password">
            <el-input v-model="loginForm.password" type="password" prefix-icon="el-icon-lock" placeholder="密码"
              autocomplete="current-password" @keydown.enter.native.prevent="submitLogin" />
          </el-form-item>
          <el-button type="primary" class="login-submit-btn" :loading="loginLoading" @click="submitLogin"
            native-type="button">登
            录</el-button>
          <div class="forgot-link-wrap">
            <a class="forgot-link" @click="loginMode = 'forgot'">忘记密码？</a>
            <template v-if="brand.sms_enabled">
              <span class="forgot-sep">|</span>
              <a class="forgot-link" @click="switchToSms">短信验证码登录</a>
            </template>
          </div>
        </el-form>

        <!-- 短信登录 -->
        <div v-if="loginMode === 'sms'" class="sms-login-form">
          <div v-if="!smsCodeSent" class="sms-step-phone">
            <el-form :model="smsForm" ref="smsForm" @submit.native.prevent>
              <el-form-item>
                <el-input v-model="smsForm.phone" prefix-icon="el-icon-mobile-phone" placeholder="请输入手机号" maxlength="11"
                  @keydown.enter.native.prevent="sendSmsCode" />
              </el-form-item>
              <el-button type="primary" class="login-submit-btn" :loading="smsSending" :disabled="!smsForm.phone"
                @click="sendSmsCode" native-type="button">发 送 验 证 码</el-button>
            </el-form>
            <div class="sms-switch-link">
              <a @click="switchToLogin">← 返回密码登录</a>
            </div>
          </div>
          <div v-else class="sms-step-code">
            <div class="sms-code-header">
              <i class="el-icon-mobile-phone sms-icon"></i>
              <p class="sms-code-title">请输入短信验证码</p>
              <p class="sms-code-desc">验证码已发送至 ****{{ phoneTail }}</p>
            </div>
            <el-form :model="smsForm" @submit.native.prevent>
              <el-form-item>
                <el-input v-model="smsForm.code" placeholder="6位验证码" maxlength="6" prefix-icon="el-icon-edit-outline"
                  class="sms-code-input" @keydown.enter.native.prevent="submitSmsLogin" ref="smsCodeInput">
                </el-input>
              </el-form-item>
              <el-button type="primary" class="login-submit-btn" :loading="smsVerifying"
                :disabled="smsForm.code.length !== 6" @click="submitSmsLogin" native-type="button">登 录</el-button>
            </el-form>
            <div class="sms-actions">
              <a class="sms-resend" :class="{ 'sms-resend-disabled': smsCountdown > 0 }" @click="sendSmsCode">
                {{ smsCountdown > 0 ? smsCountdown + 's 后重新发送' : '重新发送验证码' }}
              </a>
              <span class="sms-actions-sep">|</span>
              <a @click="switchToLogin">返回密码登录</a>
            </div>
          </div>
        </div>

        <div v-if="loginMode === 'challenge'" class="otp-section">
          <div class="otp-header">
            <i class="el-icon-mobile-phone otp-icon"></i>
            <p class="otp-title">{{ challengeType === 'sms' ? '请输入短信验证码' : '请输入动态验证码' }}</p>
            <p class="otp-desc">{{ challengeType === 'sms' ? '验证码已发送至您的手机' : challengeType === 'radius' ? '请输入动态验证码' :
              '已启用二次认证，请输入 6 位 TOTP 动态码' }}</p>
          </div>
          <el-form :model="challengeForm" class="otp-form" @submit.native.prevent>
            <el-form-item>
              <el-input v-model="challengeCode"
                :placeholder="challengeType === 'sms' ? '请输入短信验证码' : challengeType === 'radius' ? '请输入二次验证码' : '6位动态验证码'"
                :maxlength="challengeType === 'radius' ? 0 : 6" prefix-icon="el-icon-key"
                @keydown.enter.native.prevent="submitChallenge" class="otp-input">
              </el-input>
            </el-form-item>
            <el-form-item class="otp-actions">
              <el-button type="primary" :loading="challengeLoading" @click="submitChallenge" class="btn-otp-confirm"
                native-type="button">
                验 证
              </el-button>
              <el-button @click="cancelChallenge" class="btn-otp-back" native-type="button">
                返 回
              </el-button>
            </el-form-item>
          </el-form>
        </div>

        <!-- 首次登录强制改密（内联，不进独立页面） -->
        <div v-if="loginMode === 'change_pwd'" class="force-pwd-section">
          <div class="force-pwd-header">
            <i class="el-icon-warning-outline force-pwd-icon"></i>
            <p class="force-pwd-title">首次登录，请修改密码</p>
            <p class="force-pwd-desc">新密码规则：至少8位且须包含字母和数字</p>
          </div>
          <el-form :model="forcePwdForm" class="force-pwd-form" @submit.native.prevent>
            <el-form-item>
              <el-input v-model="forcePwdForm.new_password" type="password" prefix-icon="el-icon-lock" placeholder="新密码"
                autocomplete="new-password" @input="calcForcePwdStrength"
                @keydown.enter.native.prevent="submitForceChange" />
            </el-form-item>
            <el-form-item>
              <el-input v-model="forcePwdForm.confirm_password" type="password" prefix-icon="el-icon-lock"
                placeholder="确认新密码" autocomplete="new-password" @keydown.enter.native.prevent="submitForceChange" />
            </el-form-item>
            <div class="pwd-strength" v-if="forcePwdForm.new_password.length">
              <div class="strength-bar">
                <div class="strength-fill" :class="'level-' + forcePwdLevel"></div>
              </div>
              <span class="strength-text" :class="'text-level-' + forcePwdLevel">{{ forcePwdLevelText }}</span>
            </div>
            <el-button type="primary" class="login-submit-btn" :loading="forcePwdLoading" @click="submitForceChange"
              native-type="button">修改并继续</el-button>
          </el-form>
        </div>

        <div v-if="loginMode === 'forgot'" class="forgot-form">
          <div class="form-title">找回密码</div>
          <el-form :model="forgotForm" ref="forgotForm" @submit.native.prevent>
            <el-form-item prop="username">
              <el-input v-model="forgotForm.username" prefix-icon="el-icon-user-solid" placeholder="用户名" />
            </el-form-item>
            <el-form-item prop="email">
              <el-input v-model="forgotForm.email" prefix-icon="el-icon-message" placeholder="注册邮箱" />
            </el-form-item>
            <el-button type="primary" class="login-submit-btn" :loading="forgotLoading" @click="submitForgot"
              native-type="button">发 送 重 置 邮
              件</el-button>
          </el-form>
          <div v-if="forgotSent" class="forgot-success">
            <i class="el-icon-success"></i>
            <p>重置邮件已发送，请检查邮箱</p>
          </div>
          <div class="forgot-back">
            <a @click="switchToLogin">← 返回登录</a>
          </div>
        </div>

        <div v-if="loginMode === 'reset'" class="forgot-form">
          <div class="form-title">重置密码</div>
          <div class="reset-username">用户：{{ resetUsername }}</div>
          <el-form :model="resetForm" ref="resetForm" @submit.native.prevent>
            <el-form-item prop="new_password">
              <el-input v-model="resetForm.new_password" type="password" prefix-icon="el-icon-lock"
                placeholder="新密码（至少8位，含字母和数字）" autocomplete="new-password" @input="calcResetPwdStrength" />
            </el-form-item>
            <el-form-item prop="confirm_password">
              <el-input v-model="resetForm.confirm_password" type="password" prefix-icon="el-icon-lock"
                placeholder="确认新密码" autocomplete="new-password" />
            </el-form-item>
            <div class="pwd-strength" v-if="resetPwdLevel > 0">
              <div class="strength-bar">
                <div class="strength-fill" :class="'level-' + resetPwdLevel"></div>
              </div>
              <span class="strength-text" :class="'text-level-' + resetPwdLevel">{{ ['', '弱', '中', '强',
                '很强'][resetPwdLevel]
                }}</span>
            </div>
            <el-button type="primary" class="login-submit-btn" :loading="resetLoading" @click="submitReset"
              native-type="button">重 置 密
              码</el-button>
          </el-form>
          <div class="forgot-back">
            <a @click="switchToLogin">← 返回登录</a>
          </div>
        </div>

        <div v-if="loginMode === 'login' && hasSso" class="sso-section">
          <div class="sso-divider"><span>第三方登录</span></div>
          <div class="sso-buttons" :class="'sso-count-' + ssoCount">
            <el-button v-if="brand.sso_types && brand.sso_types.includes('wxwork')" class="sso-btn sso-wxwork"
              @click="startSSO('wxwork')">
              <i class="el-icon-s-platform"></i> 企微
            </el-button>
            <el-button v-if="brand.sso_types && brand.sso_types.includes('feishu')" class="sso-btn sso-feishu"
              @click="startSSO('feishu')">
              <i class="el-icon-s-cooperation"></i> 飞书
            </el-button>
            <el-button v-if="brand.sso_types && brand.sso_types.includes('dingtalk')" class="sso-btn sso-dingtalk"
              @click="startSSO('dingtalk')">
              <i class="el-icon-s-grid"></i> 钉钉
            </el-button>
          </div>
        </div>

        <!-- 登录框自定义页脚（品牌设置中的页脚文本） -->
        <div class="portal-login-footer" v-if="brand.footer" v-html="brand.footer">
        </div>
      </div>
    </template>

    <!-- 门户首页 -->
    <template v-else>
      <header class="portal-dash-header">
        <div class="header-brand">
          <img v-if="brand.logo" :src="brand.logo" class="header-logo-img" alt="logo" />
          <img v-else :src="baseUrl + (isDark ? 'logo-dark' : 'logo') + '.svg'" class="header-logo-img" alt="logo" />
          <span class="header-title">{{ brand.title ? brand.title + ' 用户门户' : 'RemLink 用户门户' }}</span>
        </div>
        <div class="header-actions">
          <span class="header-greeting">{{ displayName }}</span>
          <el-tag :type="user.status === 1 ? 'success' : 'warning'" size="small" effect="plain">
            {{ user.status === 1 ? '正常' : '异常' }}
          </el-tag>
          <div class="theme-toggle-inline" @click="toggleDarkMode" :title="isDark ? '切换亮色' : '切换暗色'">
            <i :class="isDark ? 'el-icon-sunny' : 'el-icon-moon'"></i>
          </div>
          <el-button type="danger" size="small" plain @click="logout">
            <i class="el-icon-switch-button"></i> 退出
          </el-button>
        </div>
      </header>

      <main class="portal-dash-main">
        <el-alert v-if="showAnnouncement" class="portal-announcement" :type="announcementType" :closable="false"
          show-icon>
          <div class="announcement-content" v-html="dashboard.announcement"></div>
        </el-alert>

        <div class="welcome-banner">
          <div class="welcome-body">
            <h2 class="welcome-greeting">👋 你好，{{ displayName }}</h2>
            <div class="welcome-meta">
              <span class="meta-item"><i class="el-icon-circle-check"
                  :class="user.status === 1 ? 'text-success' : 'text-danger'"></i> {{ statusLabel }}</span>
              <span class="meta-divider">·</span>
              <span class="meta-item"><i class="el-icon-time"></i> {{ expireLabel }}</span>
              <span class="meta-divider">·</span>
              <span class="meta-item"><i class="el-icon-key"></i> {{ typeLabel }}</span>
            </div>
            <div class="welcome-groups" v-if="(user.groups || []).length">
              <i class="el-icon-folder-opened"></i>
              <span v-for="(g, i) in user.groups" :key="g" class="frag">
                <el-tag size="mini" type="info" effect="plain">{{ g }}</el-tag>
                <span v-if="i < user.groups.length - 1" class="group-sep"></span>
              </span>
            </div>
          </div>
        </div>

        <el-card shadow="never" class="quicklinks-card" v-if="showQuickLinks">
          <div slot="header" class="card-header">
            <span class="card-title"><i class="el-icon-link"></i> 快捷链接</span>
          </div>
          <div class="quicklinks-body">
            <a v-for="(l, i) in quickLinks" :key="i" class="quicklink-item" :href="l.url" target="_blank"
              rel="noopener">
              <img v-if="isIconImage(l.icon)" :src="l.icon" class="ql-icon-img" alt="" />
              <i v-else-if="isIconClass(l.icon)" :class="l.icon"></i>
              <span v-else-if="l.icon" class="ql-icon-text">{{ l.icon }}</span>
              <span>{{ l.label }}</span>
            </a>
          </div>
        </el-card>

        <!-- WebVPN 应用：单点登录后可一键跳转内网 Web 应用（受门户卡片显隐开关 cards_visible.webvpn 控制） -->
        <el-card shadow="never" class="webvpn-card" v-if="cardVisible('webvpn') && webvpnApps.length">
          <div slot="header" class="card-header">
            <span class="card-title"><i class="el-icon-connection"></i> WebVPN 应用</span>
            <el-button type="text" size="mini" @click="loadWebVpnApps" :loading="webvpnLoading">
              <i class="el-icon-refresh"></i>
            </el-button>
          </div>
          <div class="webvpn-body">
            <a v-for="a in webvpnApps" :key="a.name" class="webvpn-item" :href="a.url" target="_blank" rel="noopener"
              :title="a.note || a.backend">
              <i class="el-icon-s-promotion webvpn-icon"></i>
              <span class="webvpn-name">{{ a.note || a.name }}</span>
              <span class="webvpn-host">{{ a.name }}.{{ webvpnDomain }}</span>
            </a>
          </div>
        </el-card>

        <!-- 在线设备：与统计卡片同级，醒目展示 -->
        <el-card shadow="never" class="devices-card"
          v-if="(devices.length > 0 || devicesLoading) && cardVisible('devices')">
          <div slot="header" class="card-header">
            <span class="card-title"><i class="el-icon-monitor"></i> 我的在线设备</span>
            <span class="devices-summary">
              <el-tag v-if="devices.length" size="small" type="success" effect="plain">{{ devices.length }} 台在线</el-tag>
              <el-button type="text" size="mini" @click="loadDevices" :loading="devicesLoading">
                <i class="el-icon-refresh"></i>
              </el-button>
            </span>
          </div>
          <div class="device-list" v-loading="devicesLoading">
            <div class="device-item" v-for="d in devices" :key="d.token">
              <div class="device-icon" :class="d.transport === 'UDP' ? 'icon-udp' : 'icon-tcp'">
                <i class="el-icon-cloudy"></i>
              </div>
              <div class="device-body">
                <div class="device-head">
                  <span class="device-name">
                    {{ deviceLabel(d) }}
                    <i :class="deviceTypeIcon(d)" class="device-type-icon"></i>
                  </span>
                </div>
                <div class="device-meta">
                  <span class="device-meta-item" :title="'MAC: ' + d.mac_addr">
                    <i class="el-icon-connection"></i> {{ d.ip }}
                  </span>
                  <span class="meta-sep" v-if="d.mac_addr">·</span>
                  <span class="device-meta-item mono" v-if="d.mac_addr" :title="'MAC 地址'">
                    {{ d.mac_addr }}
                  </span>
                  <span class="meta-sep">·</span>
                  <span class="device-meta-item" :title="'远端地址: ' + d.remote_addr">
                    <i class="el-icon-position"></i> {{ d.remote_addr }}
                  </span>
                  <span class="meta-sep">·</span>
                  <el-tag size="mini" effect="plain" :type="d.transport === 'UDP' ? 'success' : ''">
                    {{ d.transport }}
                  </el-tag>
                </div>
                <div class="device-traffic">
                  <span class="traffic-item" title="实时上行速率">
                    <i class="el-icon-top"></i> {{ d.bandwidth_up }}
                  </span>
                  <span class="traffic-item" title="实时下行速率">
                    <i class="el-icon-bottom"></i> {{ d.bandwidth_down }}
                  </span>
                  <span class="traffic-sep">·</span>
                  <span class="traffic-item total" title="累计上行">
                    累计 <i class="el-icon-top"></i> {{ d.bandwidth_up_all }}
                  </span>
                  <span class="traffic-item total" title="累计下行">
                    <i class="el-icon-bottom"></i> {{ d.bandwidth_down_all }}
                  </span>
                </div>
              </div>
              <div class="device-action">
                <span class="device-duration">{{ deviceDuration(d.last_login) }}</span>
                <el-popconfirm title="确定断开此设备连接？" @confirm="kickDevice(d)">
                  <el-button slot="reference" type="danger" size="mini" plain :loading="d._kicking">
                    <i class="el-icon-switch-button"></i> 断开
                  </el-button>
                </el-popconfirm>
              </div>
            </div>
          </div>
        </el-card>

        <div class="stats-row">
          <div class="stat-card">
            <div class="stat-icon" style="background: var(--color-primary-bg); color: var(--color-primary);">
              <i class="el-icon-user-solid"></i>
            </div>
            <div class="stat-body">
              <div class="stat-value">{{ userLabel(user.username, user.name) || '-' }}</div>
              <div class="stat-label">用户名</div>
            </div>
          </div>
          <div class="stat-card clickable" @click="scrollTo('groups-card')">
            <div class="stat-icon" style="background: var(--success-bg); color: var(--color-success);">
              <i class="el-icon-s-grid"></i>
            </div>
            <div class="stat-body">
              <div class="stat-value">{{ (user.groups || []).length }} 个</div>
              <div class="stat-label">所属用户组</div>
            </div>
          </div>
          <div class="stat-card">
            <div class="stat-icon" style="background: #f4f0fe; color: #9065ea;">
              <i class="el-icon-connection"></i>
            </div>
            <div class="stat-body">
              <div class="stat-value server-addr">{{ user.server_addr || '-' }}</div>
              <div class="stat-label-row">
                <span class="stat-label">VPN 服务器地址</span>
                <el-button v-if="user.server_addr" type="text" class="stat-copy-btn"
                  @click="copyText(user.server_addr)">
                  <i class="el-icon-document-copy"></i> 复制
                </el-button>
              </div>
            </div>
          </div>
          <div class="stat-card">
            <div class="stat-icon" :style="expireIconStyle">
              <i class="el-icon-time"></i>
            </div>
            <div class="stat-body">
              <div class="stat-value">{{ expireStatValue }}</div>
              <div class="stat-label">{{ expireStatLabel }}</div>
            </div>
          </div>
        </div>

        <div class="dash-grid">
          <div class="dash-left">

            <el-card shadow="never" class="table-card" v-if="showClientGuide">
              <div slot="header" class="card-header">
                <span class="card-title"><i class="el-icon-connection"></i> 客户端连接指引</span>
                <el-button v-if="dashboard.client_download_html" type="primary" size="mini" plain
                  @click="downloadDialogVisible = true">
                  <i class="el-icon-download"></i> 客户端下载
                </el-button>
              </div>
              <el-tabs v-model="clientTab" class="client-tabs">
                <el-tab-pane v-for="g in clientGuide" :key="g.name" :label="g.name" :name="g.name">
                  <div class="client-step" v-for="(s, i) in g.steps" :key="i">
                    <div class="step-num">{{ i + 1 }}</div>
                    <div v-html="replaceServerAddr(s)"></div>
                  </div>
                </el-tab-pane>
              </el-tabs>
            </el-card>

            <el-card shadow="never" class="table-card" id="groups-card" v-if="cardVisible('groups')">
              <div slot="header" class="card-header">
                <span class="card-title"><i class="el-icon-s-grid"></i> 分组权限详情</span>
                <el-tag size="small" type="info" effect="plain">{{ (user.groups_detail || []).length }} 个组</el-tag>
              </div>
              <template v-if="(user.groups_detail || []).length">
                <div class="group-list">
                  <div class="group-item" v-for="g in user.groups_detail" :key="g.name">
                    <div class="group-item-header">
                      <div class="group-name-row">
                        <span class="group-name">{{ g.name }}</span>
                        <el-tag :type="g.status === 1 ? 'success' : 'warning'" size="mini" effect="plain">
                          {{ g.status === 1 ? '正常' : '停用' }}
                        </el-tag>
                      </div>
                      <i class="el-icon-arrow-right group-arrow"></i>
                    </div>
                    <div class="group-item-body">
                      <div class="group-info-row" v-if="g.note">
                        <span class="info-label">备注</span>
                        <span class="info-val">{{ g.note }}</span>
                      </div>
                      <div class="group-info-row" v-if="(g.auth_types || []).length">
                        <span class="info-label">认证方式</span>
                        <span class="info-val">
                          <span v-for="(at, i) in g.auth_types" :key="at" class="frag">
                            <el-tag size="mini" effect="plain">{{ at }}</el-tag>
                            <i v-if="i < g.auth_types.length - 1" class="el-icon-d-arrow-right auth-arrow"></i>
                          </span>
                        </span>
                      </div>
                      <div class="group-info-row" v-if="(g.dns || []).length">
                        <span class="info-label">DNS 域</span>
                        <span class="info-val">
                          <el-tag size="mini" v-for="d in g.dns" :key="d" effect="plain" type="info">{{ d }}</el-tag>
                        </span>
                      </div>
                      <div class="group-policy-box" v-if="g.policy">
                        <div class="policy-title">📋 组策略：{{ g.policy.name }}</div>
                        <div class="policy-summary">
                          <span v-if="g.policy.allow_lan"><i class="el-icon-check text-success"></i> 允许本地局域网</span>
                          <span v-if="g.policy.bandwidth > 0"><i class="el-icon-bottom"></i> 下行 {{
                            formatBandwidth(g.policy.bandwidth) }}</span>
                          <span v-if="g.policy.bandwidth_up > 0"><i class="el-icon-top"></i> 上行 {{
                            formatBandwidth(g.policy.bandwidth_up) }}</span>
                          <span v-if="g.policy.traffic_quota > 0"><i class="el-icon-data-line"></i> 流量 {{
                            formatTraffic(user.traffic_used) }} / {{ formatTraffic(g.policy.traffic_quota) }}{{
                              resetLabel(g.policy.traffic_reset) }}</span>
                          <span v-if="g.policy.route_include > 0"><i class="el-icon-share"></i> {{
                            g.policy.route_include }} 条路由</span>
                          <span v-if="g.policy.acl_count > 0"><i class="el-icon-lock"></i> {{ g.policy.acl_count }} 条
                            ACL</span>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </template>
              <div v-else class="empty-hint">
                <i class="el-icon-info"></i> 暂无分组信息
              </div>
            </el-card>

            <el-card shadow="never" class="table-card" v-if="user.user_policy && cardVisible('personal_policy')">
              <div slot="header" class="card-header">
                <span class="card-title"><i class="el-icon-document-checked"></i> 个人策略</span>
              </div>
              <div class="policy-detail">
                <div class="policy-name">{{ user.user_policy.name }}</div>
                <div class="policy-note" v-if="user.user_policy.note">{{ user.user_policy.note }}</div>
                <div class="policy-summary mt-sm">
                  <span v-if="user.user_policy.allow_lan"><i class="el-icon-check text-success"></i> 允许本地局域网</span>
                  <span v-if="user.user_policy.bandwidth > 0"><i class="el-icon-bottom"></i> 下行 {{
                    formatBandwidth(user.user_policy.bandwidth) }}</span>
                  <span v-if="user.user_policy.bandwidth_up > 0"><i class="el-icon-top"></i> 上行 {{
                    formatBandwidth(user.user_policy.bandwidth_up) }}</span>
                  <span v-if="user.user_policy.traffic_quota > 0"><i class="el-icon-data-line"></i> 流量 {{
                    formatTraffic(user.traffic_used) }} / {{ formatTraffic(user.user_policy.traffic_quota) }}{{
                      resetLabel(user.user_policy.traffic_reset) }}</span>
                  <span v-if="user.user_policy.route_include > 0"><i class="el-icon-share"></i> 包含 {{
                    user.user_policy.route_include
                  }} 条路由</span>
                  <span v-if="user.user_policy.route_exclude > 0"><i class="el-icon-circle-close"></i> 排除 {{
                    user.user_policy.route_exclude }} 条路由</span>
                  <span v-if="user.user_policy.acl_count > 0"><i class="el-icon-lock"></i> {{ user.user_policy.acl_count
                  }} 条 ACL
                    规则</span>
                </div>
              </div>
            </el-card>
          </div>

          <div class="dash-right">

            <el-card shadow="never" class="table-card" v-if="cardVisible('password')">
              <div slot="header" class="card-header">
                <span class="card-title"><i class="el-icon-lock"></i> 修改密码</span>
              </div>
              <template v-if="user.can_change_password">
                <el-form :model="passwordForm" ref="passwordForm" label-position="top" class="pwd-form"
                  @submit.native.prevent>
                  <el-form-item label="当前密码">
                    <el-input v-model="passwordForm.old_password" type="password" prefix-icon="el-icon-unlock"
                      placeholder="输入当前密码" autocomplete="current-password" />
                  </el-form-item>
                  <el-form-item label="新密码">
                    <el-input v-model="passwordForm.new_password" type="password" prefix-icon="el-icon-lock"
                      placeholder="至少8位，含字母和数字" autocomplete="new-password" @input="calcPwdStrength" />
                  </el-form-item>
                  <el-form-item label="确认新密码">
                    <el-input v-model="passwordForm.confirm_password" type="password" prefix-icon="el-icon-lock"
                      placeholder="再次输入新密码" autocomplete="new-password" />
                  </el-form-item>
                  <div class="pwd-strength" v-if="passwordForm.new_password.length">
                    <div class="strength-bar">
                      <div class="strength-fill" :class="'level-' + pwdLevel"></div>
                    </div>
                    <span class="strength-text" :class="'text-level-' + pwdLevel">{{ pwdLevelText }}</span>
                  </div>
                  <el-button type="primary" class="full-btn" :loading="passwordLoading" @click="changePassword"
                    native-type="button">
                    <i class="el-icon-check"></i> 修改密码
                  </el-button>
                </el-form>
              </template>
              <div v-else class="no-pwd-hint">
                <div class="hint-icon"><i class="el-icon-info"></i></div>
                <p>当前账号由外部身份源（{{ typeLabel }}）认证，请到对应平台修改密码。</p>
              </div>
            </el-card>

            <el-card shadow="never" class="table-card" v-if="cardVisible('otp')">
              <div slot="header" class="card-header">
                <span class="card-title"><i class="el-icon-mobile-phone"></i> 二次验证 (OTP)</span>
                <el-tag v-if="user.otp_enabled" size="small" type="success" effect="plain">已启用</el-tag>
                <el-tag v-else size="small" type="info" effect="plain">未启用</el-tag>
              </div>
              <div v-if="!user.otp_enabled" class="otp-disabled-hint">
                <p>当前未启用二次验证。请联系管理员在后台设置中为您开启 OTP 功能。</p>
              </div>
              <template v-else>
                <div class="otp-status-info">
                  <el-button type="primary" size="small" plain @click="showOtpQR">查看密钥 / 二维码</el-button>
                  <el-button type="warning" size="small" plain @click="openOtpBind">重新绑定</el-button>
                </div>
              </template>
            </el-card>

            <el-card shadow="never" class="table-card" v-if="cardVisible('certs')">
              <div slot="header" class="card-header">
                <span class="card-title"><i class="el-icon-document"></i> 我的证书</span>
                <el-tag size="small" type="info" effect="plain">{{ certs.length }} 个</el-tag>
              </div>
              <div v-if="certsLoading" class="empty-hint"><i class="el-icon-loading"></i> 加载中…</div>
              <template v-else-if="certs.length">
                <div class="cert-list">
                  <div class="cert-item" v-for="c in certs" :key="c.id" :class="{ 'cert-disabled': c.status !== 0 }">
                    <div class="cert-item-main">
                      <div class="cert-item-header">
                        <span class="cert-groupname">{{ c.groupname }}</span>
                        <el-tag :type="c.status === 0 ? 'success' : (c.status === 1 ? 'warning' : 'danger')" size="mini"
                          effect="plain">{{ c.status_text }}</el-tag>
                      </div>
                      <div class="cert-item-meta">
                        <span class="cert-meta-text" v-if="c.serial_number">序列号: {{ c.serial_number }}</span>
                        <span class="cert-meta-text">过期: {{ formatCertTime(c.not_after) }}</span>
                        <span class="cert-meta-text" v-if="c.is_csr_based">CSR 模式</span>
                      </div>
                      <div class="cert-item-footer" v-if="c.device_binding_enabled">
                        <i class="el-icon-monitor"></i> 已绑定 {{ c.device_count }} / {{ c.max_devices }} 设备
                      </div>
                    </div>
                    <div class="cert-item-actions">
                      <el-button v-if="c.status === 0" type="primary" size="mini" plain @click="downloadCert(c)">下载
                        P12</el-button>
                      <el-tooltip v-else :content="c.status === 1 ? '证书已禁用' : '证书已过期'" placement="top">
                        <el-button type="info" size="mini" plain disabled>不可下载</el-button>
                      </el-tooltip>
                    </div>
                  </div>
                </div>
              </template>
              <div v-else class="empty-hint">
                <i class="el-icon-info"></i> 暂无客户端证书，请联系管理员签发
              </div>
            </el-card>
          </div>
        </div>

        <footer class="portal-footer">
          <template v-if="brand.footer">
            <div v-html="brand.footer"></div>
          </template>
          <template v-else>
            <span>{{ user.issuer || '企业级安全远程接入网关' }}</span>
            <span class="footer-divider">|</span>
            <span>Powered by </span>
            <a href="https://github.com/wsczx/RemLink" target="_blank" class="footer-link">RemLink</a>
          </template>
        </footer>
      </main>
    </template>

    <el-dialog :title="otpDialogTitle" :visible.sync="otpDialogVisible" width="420px" :close-on-click-modal="true"
      @closed="onOtpDialogClosed">
      <!-- 密码确认步骤 -->
      <div class="otp-dialog-body" v-if="otpStep === 'password'">
        <el-alert title="重新绑定后旧密钥将失效，所有已绑定设备均需重新扫码绑定" type="warning" :closable="false" show-icon class="otp-alert" />
        <p class="otp-tip">请输入当前密码确认操作</p>
        <el-input v-model="otpPassword" type="password" placeholder="请输入当前密码" show-password
          @keydown.enter.native.prevent="submitOtpPassword" />
      </div>
      <!-- 二维码展示步骤 -->
      <div class="otp-dialog-body" v-else>
        <div class="otp-qr-wrap" v-if="otpQrBase64">
          <img :src="'data:image/png;base64,' + otpQrBase64" alt="OTP QR" class="otp-qr-img" />
        </div>
        <div class="otp-secret-row" v-if="otpSecret">
          <span class="otp-secret-label">密钥：</span>
          <code class="otp-secret-code">{{ otpSecret }}</code>
          <el-button type="text" size="mini" @click="copyText(otpSecret)">复制</el-button>
        </div>
        <p class="otp-tip">请使用 Google Authenticator 或类似工具扫描二维码绑定</p>
      </div>
      <span slot="footer">
        <el-button @click="otpDialogVisible = false">{{ otpStep === 'qr' ? '关闭' : '取消' }}</el-button>
        <el-button v-if="otpStep === 'password'" type="primary" :loading="otpRegenLoading"
          @click="submitOtpPassword">确认</el-button>
      </span>
    </el-dialog>

    <el-dialog title="下载客户端证书" :visible.sync="certDownloadVisible" width="380px" :close-on-click-modal="true">
      <div class="cert-download-body">
        <p class="cert-download-tip">请设置证书密码，下载后导入客户端时需输入此密码。</p>
        <el-input v-model="certDownloadPassword" type="password" placeholder="至少 4 位密码" :minlength="4" show-password
          @keydown.enter.native.prevent="doDownload" />
      </div>
      <span slot="footer">
        <el-button @click="certDownloadVisible = false">取消</el-button>
        <el-button type="primary" @click="doDownload">下载</el-button>
      </span>
    </el-dialog>

    <el-dialog title="客户端下载" :visible.sync="downloadDialogVisible" width="560px" :close-on-click-modal="true"
      custom-class="download-dialog">
      <div class="download-dialog-body" v-if="dashboard.client_download_html" v-html="dashboard.client_download_html">
      </div>
    </el-dialog>
  </div>
</template>

<script>
import axios from "axios"
import { applyBrandToDocument } from "../plugins/brand"
import { defaultClientGuide } from "../plugins/clientGuide"
import userLabel from "@/mixins/userLabel"

export default {
  name: "Portal",
  mixins: [userLabel],
  data() {
    return {
      loggedIn: false,
      checking: true,
      isDark: false,
      brand: { title: "", logo: "", footer: "", sso_types: [], sms_enabled: false, features_enabled: 0, features: "" },
      featureIcons: [
        { cls: "feat-status", icon: "el-icon-circle-check" },
        { cls: "feat-group", icon: "el-icon-s-grid" },
        { cls: "feat-security", icon: "el-icon-lock" },
      ],
      user: {},
      dashboard: {},
      customCssEl: null,
      loginLoading: false,
      loginMode: "login",
      forgotLoading: false,
      forgotSent: false,
      resetLoading: false,
      resetToken: "",
      resetUsername: "",
      resetPwdLevel: 0,
      passwordLoading: false,
      forcePwdLoading: false,
      forcePwdToken: "",
      forcePwdForm: { new_password: "", confirm_password: "" },
      forcePwdLevel: 0,
      otpRegenLoading: false,
      challengeLoading: false,
      challengeType: "",
      challengeMessage: "",
      challengeCode: "",
      challengeSession: "",
      challengeForm: {},
      clientTab: "win",
      otpDialogVisible: false,
      otpStep: "password",
      otpPassword: "",
      otpQrBase64: "",
      otpSecret: "",
      certs: [],
      certsLoading: false,
      certDownloadVisible: false,
      certDownloadGroup: "",
      certDownloadPassword: "",
      downloadDialogVisible: false,
      pwdLevel: 0,
      devices: [],
      devicesLoading: false,
      devicesTimer: null,
      webvpnApps: [],
      webvpnLoading: false,
      webvpnDomain: "",
      webvpnMode: false, // 是否 WebVPN 子域名验证场景（登录页仅验证访问权限，不进入门户首页）
      webvpnGrant: "", // 登录成功后跨门户子域回跳所需的一次性授权
      loginForm: { username: "", password: "" },
      smsForm: { phone: "", code: "" },
      smsSending: false,
      smsVerifying: false,
      smsCodeSent: false,
      smsCountdown: 0,
      smsCountdownTimer: null,
      phoneTail: "",
      forgotForm: { username: "", email: "" },
      resetForm: { new_password: "", confirm_password: "" },
      passwordForm: { old_password: "", new_password: "", confirm_password: "" },
      loginRules: {
        username: [{ required: true, message: "请输入用户名", trigger: "blur" }],
        password: [{ required: true, message: "请输入密码", trigger: "blur" }],
      },
    }
  },
  computed: {
    displayName() {
      return this.userLabel(this.user.username, this.user.name) || "用户"
    },
    otpDialogTitle() {
      return this.otpStep === "password" ? "重新绑定 OTP" : "OTP 密钥"
    },
    typeLabel() {
      const m = { local: "本地用户", ldap: "LDAP 用户", radius: "RADIUS 用户", wxwork: "企微用户", feishu: "飞书用户", dingtalk: "钉钉用户", external: "外部用户" }
      return m[this.user.type] || this.user.type || "本地用户"
    },
    statusLabel() {
      return this.user.status === 1 ? "账号正常" : "账号异常"
    },
    expireLabel() {
      if (!this.user.limittime) return "永久有效"
      const ts = typeof this.user.limittime === "number"
        ? this.user.limittime
        : Date.parse(this.user.limittime) / 1000
      const d = this.remainingDaysCalc(ts)
      return d === null ? "永久有效" : (d <= 0 ? "已过期" : `还有 ${d} 天到期`)
    },
    expireStatValue() {
      if (!this.user.limittime) return "∞"
      const ts = this.getLimitTimestamp()
      if (ts === 0) return "∞"
      const dt = new Date(ts * 1000)
      const y = dt.getFullYear()
      const m = String(dt.getMonth() + 1).padStart(2, '0')
      const d = String(dt.getDate()).padStart(2, '0')
      const h = String(dt.getHours()).padStart(2, '0')
      const mi = String(dt.getMinutes()).padStart(2, '0')
      return `${y}-${m}-${d} ${h}:${mi}`
    },
    expireStatLabel() {
      if (!this.user.limittime) return "永久有效"
      const ts = this.getLimitTimestamp()
      const d = this.remainingDaysCalc(ts)
      return d === null ? "永久有效" : (d <= 0 ? "已过期" : d <= 30 ? "即将到期" : "到期时间")
    },
    expireIconStyle() {
      if (!this.user.limittime) return { background: "var(--success-bg)", color: "var(--color-success)" }
      const ts = this.getLimitTimestamp()
      const d = this.remainingDaysCalc(ts)
      if (d === null) return { background: "var(--success-bg)", color: "var(--color-success)" }
      if (d <= 0) return { background: "var(--danger-bg)", color: "var(--color-danger)" }
      if (d <= 30) return { background: "var(--warning-bg)", color: "var(--color-warning)" }
      return { background: "var(--success-bg)", color: "var(--color-success)" }
    },
    pwdLevelText() {
      return ["", "弱", "中", "强", "很强"][this.pwdLevel] || ""
    },
    forcePwdLevelText() {
      return ["", "弱", "中", "强", "很强"][this.forcePwdLevel] || ""
    },
    hasSso() {
      const t = this.brand.sso_types || []
      const has = t.includes("wxwork") || t.includes("feishu") || t.includes("dingtalk")
      // WebVPN 子域名验证场景：三方登录需跳转到专用门户域名（webvpn_sso_domain）完成认证，
      // 未配置该域名时入口不可用（避免跳到错误域名导致三方认证失败）。
      if (this.webvpnMode && !this.brand.webvpn_sso_domain) return false
      return has
    },
    ssoCount() {
      const t = this.brand.sso_types || []
      return ["wxwork", "feishu", "dingtalk"].filter((v) => t.includes(v)).length
    },
    showFeatures() {
      return this.brand.features_enabled !== 2
    },
    displayFeatures() {
      let list = []
      if (this.brand.features) {
        try {
          list = JSON.parse(this.brand.features)
        } catch (e) {
          list = []
        }
      }
      if (!Array.isArray(list) || list.length === 0) {
        list = [
          { label: "账号状态", desc: "查看在线设备与连接状态" },
          { label: "用户分组", desc: "查看所属用户分组" },
          { label: "安全设置", desc: "自主修改密码与验证方式" },
        ]
      }
      return list
    },
    showAnnouncement() {
      return this.dashboard.announcement_enabled !== 2 && !!this.dashboard.announcement
    },
    showQuickLinks() {
      return this.dashboard.quick_links_enabled !== 2 && this.quickLinks.length > 0
    },
    announcementType() {
      const t = this.dashboard.announcement_level
      return t === "success" || t === "warning" || t === "error" ? t : "info"
    },
    quickLinks() {
      let list = []
      if (this.dashboard.quick_links) {
        try {
          list = JSON.parse(this.dashboard.quick_links)
        } catch (e) {
          list = []
        }
      }
      return Array.isArray(list) ? list : []
    },
    showClientGuide() {
      return this.dashboard.client_guide_enabled === 1 && this.clientGuide.length > 0
    },
    clientGuide() {
      let list = []
      if (this.dashboard.client_guide) {
        try {
          list = JSON.parse(this.dashboard.client_guide)
        } catch (e) {
          list = []
        }
      }
      if (!Array.isArray(list) || list.length === 0) {
        list = defaultClientGuide
      }
      return list
    },
    cardsVisibleMap() {
      let m = {}
      if (this.dashboard.cards_visible) {
        try {
          m = JSON.parse(this.dashboard.cards_visible)
        } catch (e) {
          m = {}
        }
      }
      return m
    },
    cardVisible() {
      return (name) => this.cardsVisibleMap[name] !== false
    },
  },
  watch: {
    clientGuide: {
      immediate: true,
      handler(list) {
        // 平台标签名为动态平台名，默认选中第一个，避免进入时无标签匹配导致内容空白
        if (Array.isArray(list) && list.length && !list.some((g) => g.name === this.clientTab)) {
          this.clientTab = list[0].name
        }
      },
    },
  },
  mounted() {
    this.isDark = document.documentElement.classList.contains('dark');
    this.checkResetToken()
    // 初步判断 WebVPN 子域名验证场景：URL 携带 redirect 参数（网关 302 到登录页时总带，
    // 主域门户登录无此参数）。此处同步设置，避免 loadBrand（异步取 domain）与 loadMe 竞态，
    // 已登录用户访问子域名时不会闪现门户首页；loadBrand 成功后会用 domain 精确复核。
    const qr = this.$route.query.redirect
    if (qr && qr.startsWith("/") && !qr.startsWith("//")) {
      this.webvpnMode = true
    }
    this.loadBrand()
    this.loadMe()
    this.resumeCallbackChallenge()
    // 监听品牌更新事件：管理后台保存品牌配置后立即重新加载（无需刷新页面）
    this._brandHandler = () => { this.loadBrand() };
    window.addEventListener('remlink:brand-updated', this._brandHandler);
  },
  beforeDestroy() {
    if (this._brandHandler) {
      window.removeEventListener('remlink:brand-updated', this._brandHandler);
    }
    // 清理 applyDashboardTheme 注入到全局 document 的自定义样式与主色变量，
    // 避免极端自定义 CSS 残留影响登录页等其它页面
    if (this.customCssEl) {
      this.customCssEl.remove();
      this.customCssEl = null;
    }
    document.documentElement.style.removeProperty('--color-primary');
    document.documentElement.style.removeProperty('--color-primary-bg');
  },
  methods: {
    isIconImage(icon) {
      if (!icon) return false
      return /^(https?:\/\/|\/\/|\.\.?\/|data:image\/)/i.test(icon)
    },
    isIconClass(icon) {
      if (!icon) return false
      return /\s/.test(icon) || /^(el-icon|icon|fa|fas|far|fab|ivu-icon|anticon)/i.test(icon)
    },
    toggleDarkMode() {
      this.isDark = !this.isDark;
      localStorage.setItem('dark-mode', this.isDark);
      document.documentElement.classList.toggle('dark', this.isDark);
    },
    portalApi(method, url, data) {
      return axios({ method, url, data, headers: { 'Content-Type': 'application/json' } })
    },
    applyDashboardTheme() {
      // 先清除上一次注入的自定义样式与主色变量
      if (this.customCssEl) {
        this.customCssEl.remove()
        this.customCssEl = null
      }
      document.documentElement.style.removeProperty("--color-primary")
      document.documentElement.style.removeProperty("--color-primary-bg")
      const d = this.dashboard || {}
      if (d.theme_color) {
        document.documentElement.style.setProperty("--color-primary", d.theme_color)
        document.documentElement.style.setProperty("--color-primary-bg", this.hexToRgba(d.theme_color, 0.1))
      }
      if (d.custom_css) {
        const el = document.createElement("style")
        el.id = "portal-custom-css"
        el.textContent = d.custom_css
        document.head.appendChild(el)
        this.customCssEl = el
      }
    },
    hexToRgba(hex, alpha) {
      const m = /^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i.exec(hex || "")
      if (!m) return "transparent"
      const r = parseInt(m[1], 16), g = parseInt(m[2], 16), b = parseInt(m[3], 16)
      return `rgba(${r}, ${g}, ${b}, ${alpha})`
    },
    loadBrand() {
      this.portalApi("get", "/portal/api/login-config").then(resp => {
        if (resp.data.code === 0 && resp.data.data) {
          const d = resp.data.data
          this.brand = Object.assign(
            { title: "", logo: "", favicon: "", footer: "", sso_types: [], sms_enabled: false, features_enabled: 0, features: "", domain: "", webvpn_sso_domain: "" },
            d
          )
          applyBrandToDocument(this.brand)
          // 复核是否为 WebVPN 子域名验证场景：当前 host 是 WebVPN 主域的子域。
          // 仅当尚未判定为子域（mounted 已按 redirect 参数判过一次）时再据此确认，
          // 避免已判定的场景被反向覆盖。
          if (!this.webvpnMode && d.domain) {
            const host = window.location.hostname
            const main = String(d.domain).replace(/^\./, "").split(":")[0]
            if (main && host !== main && host.endsWith("." + main)) {
              this.webvpnMode = true
            }
          }
        }
      }).catch(() => { })
    },
    loadMe() {
      this.portalApi("get", "/portal/api/me").then(resp => {
        if (resp.data.code === 0) {
          this.user = resp.data.data
          // WebVPN 子域名验证场景：已登录时不展示门户首页，直接回跳内网应用。
          // 有 redirect（用户本要访问的内网路径）则回跳该路径；否则回跳子域名根，由网关兑换会话后反代内网应用。
          if (this.webvpnMode) {
            this.doWebVpnRedirect()
            return
          }
          this.dashboard = resp.data.data.dashboard || {}
          this.applyDashboardTheme()
          this.loggedIn = true
          // 已登录且从 WebVPN 子站点跳转过来带 redirect 参数时，自动回跳，
          // 不必停留在门户首页（用户本来想访问的是 WebVPN 应用）。
          // redirect 仅含路径，基于当前 origin 补全，规避反代 Host 透传问题。
          // 安全：仅允许本站相对路径（以单 "/" 开头，不以 "//" 开头避免协议相对地址跳转外部站）。
          const redirect = this.$route.query.redirect
          if (redirect && redirect.startsWith("/") && !redirect.startsWith("//")) {
            window.location.href = window.location.origin + redirect
            return
          }
          this.loadCerts()
          this.loadDevices()
          this.startDevicesPoll()
          this.loadWebVpnApps()
        }
      }).catch(() => {
        this.loggedIn = false
      }).finally(() => {
        this.checking = false
      })
    },
    // WebVPN 子域名验证场景的统一回跳：回跳到用户本要访问的内网应用。
    // redirect 仅含路径且必须以单 "/" 开头（不含 "//" 协议相对地址，防跳转外部站）。
    // 无有效 redirect 时回跳子域名根路径，由网关完成会话兑换后反代内网应用。
    doWebVpnRedirect() {
      const redirect = this.$route.query.redirect
      const path = redirect && redirect.startsWith("/") && !redirect.startsWith("//")
        ? redirect
        : "/"
      const query = this.webvpnGrant
        ? (path.includes("?") ? "&" : "?") + "webvpn_grant=" + encodeURIComponent(this.webvpnGrant)
        : ""
      window.location.href = window.location.origin + path + query
    },
    resumeCallbackChallenge() {
      if (!this.$route.query.session_id) return
      this.challengeType = this.$route.query.challenge || "otp"
      this.challengeMessage = this.challengeType === "radius" ? "请输入二次验证码" : "请输入 6 位动态验证码"
      this.challengeSession = this.$route.query.session_id
      this.challengeCode = ""
      this.loginMode = "challenge"
      this.$router.replace("/portal")
    },
    submitLogin() {
      this.$refs.loginForm.validate(valid => {
        if (!valid) return
        this.loginLoading = true
        this.portalApi("post", "/portal/api/login", this.loginForm).then(resp => {
          this.handleAuthResponse(resp.data)
        }).catch(() => {
          this.$message.error("网络请求失败，请稍后重试")
        }).finally(() => {
          this.loginLoading = false
        })
      })
    },
    switchToSms() {
      this.loginMode = "sms"
      this.smsCodeSent = false
      this.smsForm = { phone: "", code: "" }
      this.$nextTick(() => {
        const input = document.querySelector(".sms-step-phone input")
        if (input) input.focus()
      })
    },
    startSmsCountdown() {
      this.smsCountdown = 60
      if (this.smsCountdownTimer) clearInterval(this.smsCountdownTimer)
      this.smsCountdownTimer = setInterval(() => {
        this.smsCountdown--
        if (this.smsCountdown <= 0) {
          clearInterval(this.smsCountdownTimer)
          this.smsCountdownTimer = null
        }
      }, 1000)
    },
    sendSmsCode() {
      if (this.smsCountdown > 0) return
      if (!this.smsForm.phone || this.smsForm.phone.length < 11) {
        this.$message.warning("请输入正确的手机号")
        return
      }
      this.smsSending = true
      this.portalApi("post", "/portal/api/sms/send", { phone: this.smsForm.phone }).then(resp => {
        if (resp.data.code === 0) {
          this.smsCodeSent = true
          this.phoneTail = resp.data.data.phone_tail || this.smsForm.phone.slice(-4)
          this.smsForm.code = ""
          this.startSmsCountdown()
          this.$nextTick(() => {
            const input = document.querySelector(".sms-code-input input")
            if (input) input.focus()
          })
        } else {
          this.$message.error(resp.data.msg)
        }
      }).catch(() => {
        this.$message.error("发送失败，请稍后重试")
      }).finally(() => {
        this.smsSending = false
      })
    },
    submitSmsLogin() {
      if (this.smsForm.code.length !== 6) return
      this.smsVerifying = true
      this.portalApi("post", "/portal/api/sms/verify", {
        phone: this.smsForm.phone,
        code: this.smsForm.code,
      }).then(resp => {
        if (resp.data.code === 0) {
          this.challengeCode = ""
          this.challengeSession = ""
          this.loggedIn = true
          this.smsCodeSent = false
          this.loadMe()
          this.$message.success("登录成功")
        } else {
          this.$message.error(resp.data.msg)
        }
      }).catch(() => {
        this.$message.error("验证失败，请重试")
      }).finally(() => {
        this.smsVerifying = false
      })
    },
    submitChallenge() {
      if (!this.challengeCode) {
        this.$message.warning("请输入验证码")
        return
      }
      this.challengeLoading = true
      this.portalApi("post", "/portal/api/verify", {
        session_id: this.challengeSession,
        code: this.challengeCode,
      }).then(resp => {
        this.handleAuthResponse(resp.data)
      }).catch(() => {
        this.$message.error("验证失败，请重试")
      }).finally(() => {
        this.challengeLoading = false
      })
    },
    handleAuthResponse(resp) {
      if (resp.code !== 0) {
        this.$message.error(resp.msg)
        return
      }
      const data = resp.data || {}
      if (data.status === "pass") {
        this.challengeCode = ""
        this.challengeSession = ""
        this.loginForm.password = ""
        this.webvpnGrant = data.webvpn_grant || ""
        // WebVPN 子域名验证场景：登录成功后直接回跳内网应用，不进入门户首页。
        if (this.webvpnMode) {
          this.doWebVpnRedirect()
          return
        }
        this.loggedIn = true
        this.loadMe()
        // WebVPN 子站点未登录跳转过来时带了 redirect 参数，登录成功后自动回跳，
        // 避免用户停在门户首页还得手动再访问一次。redirect 仅含路径，基于 origin 补全。
        // 安全：仅允许本站相对路径（以单 "/" 开头，不以 "//" 开头避免协议相对地址跳转外部站）。
        const redirect = this.$route.query.redirect
        if (redirect && redirect.startsWith("/") && !redirect.startsWith("//")) {
          window.location.href = window.location.origin + redirect
          return
        }
        this.$message.success("登录成功")
        return
      }
      if (data.status === "change_pwd") {
        this.forcePwdToken = data.token || ""
        this.forcePwdForm = { new_password: "", confirm_password: "" }
        this.forcePwdLevel = 0
        this.loginForm.password = ""
        this.loginMode = "change_pwd"
        this.$nextTick(() => {
          const input = document.querySelector(".force-pwd-form input")
          if (input) input.focus()
        })
        return
      }
      if (data.status === "otp" || data.status === "radius" || data.status === "sms" || data.status === "verify") {
        this.challengeType = data.status
        this.challengeMessage = data.message || "请输入验证码"
        this.challengeSession = data.session_id
        this.challengeCode = ""
        this.loginMode = "challenge"
        this.$nextTick(() => {
          const input = document.querySelector(".otp-input input")
          if (input) input.focus()
        })
        return
      }
      this.$message.warning(data.message || "需要继续认证")
    },
    cancelChallenge() {
      this.loginMode = "login"
      this.challengeCode = ""
      this.challengeSession = ""
    },
    startSSO(type) {
      // 携带回跳地址（当前完整 URL + 原目标路径）发起第三方登录。
      // 统一请求当前域名的 /portal/api/sso（相对路径）：
      //   - 门户主域：PortalSSO 直接进入三方平台认证（回调地址即门户域名，与平台绑定一致）。
      //   - WebVPN 子域名：PortalSSO 识别子域后，后端按 webvpn_sso_domain 跳转到门户域名
      //     完成三方认证，再由后端回跳回当前子域名。前端不拼域名/端口，避免丢端口。
      let href = "/portal/api/sso?type=" + encodeURIComponent(type)
      const rp = this.$route.query.redirect
      if (rp && rp.startsWith("/") && !rp.startsWith("//")) {
        href += "&redirect=" + encodeURIComponent(window.location.origin + rp)
      }
      window.location.href = href
    },
    changePassword() {
      if (!this.passwordForm.old_password || !this.passwordForm.new_password) {
        this.$message.warning("请输入新旧密码")
        return
      }
      const pwdErr = this.validatePassword(this.passwordForm.new_password)
      if (pwdErr) {
        this.$message.warning(pwdErr)
        return
      }
      if (this.passwordForm.new_password !== this.passwordForm.confirm_password) {
        this.$message.warning("两次输入的新密码不一致")
        return
      }
      this.passwordLoading = true
      this.portalApi("post", "/portal/api/change_password", {
        old_password: this.passwordForm.old_password,
        new_password: this.passwordForm.new_password,
      }).then(resp => {
        if (resp.data.code === 0) {
          this.$message.success("密码修改成功，下次登录请使用新密码")
          this.passwordForm = { old_password: "", new_password: "", confirm_password: "" }
          this.pwdLevel = 0
        } else {
          this.$message.error(resp.data.msg)
        }
      }).catch(() => {
        this.$message.error("请求失败，请稍后重试")
      }).finally(() => {
        this.passwordLoading = false
      })
    },
    // 首次登录强制改密提交（内联，无需旧密码）
    submitForceChange() {
      if (!this.forcePwdForm.new_password) {
        this.$message.warning("请输入新密码")
        return
      }
      const pwdErr = this.validatePassword(this.forcePwdForm.new_password)
      if (pwdErr) {
        this.$message.warning(pwdErr)
        return
      }
      if (this.forcePwdForm.new_password !== this.forcePwdForm.confirm_password) {
        this.$message.warning("两次输入的新密码不一致")
        return
      }
      this.forcePwdLoading = true
      this.portalApi("post", "/portal/api/force_change_password", {
        token: this.forcePwdToken,
        new_password: this.forcePwdForm.new_password,
        new_password_confirm: this.forcePwdForm.confirm_password,
      }).then(resp => {
        if (resp.data.code !== 0) {
          this.$message.error(resp.data.msg)
          return
        }
        // 改密后可能直接登录，或继续 OTP 二次认证
        this.handleAuthResponse(resp.data)
      }).catch(() => {
        this.$message.error("请求失败，请稍后重试")
      }).finally(() => {
        this.forcePwdLoading = false
      })
    },
    calcForcePwdStrength() {
      const pwd = this.forcePwdForm.new_password
      if (!pwd) { this.forcePwdLevel = 0; return }
      let score = 0
      if (pwd.length >= 8) score++
      if (pwd.length >= 12) score++
      if (/[a-z]/.test(pwd) && /[A-Z]/.test(pwd)) score++
      if (/\d/.test(pwd)) score++
      if (/[^a-zA-Z0-9]/.test(pwd)) score++
      this.forcePwdLevel = Math.min(4, score)
    },
    // 密码重置
    checkResetToken() {
      const token = this.$route.query.reset_token
      if (!token || this.loggedIn) return
      this.portalApi("get", "/portal/api/reset_password/verify?token=" + encodeURIComponent(token)).then(resp => {
        const d = resp.data.data || {}
        if (d.valid) {
          this.resetToken = token
          this.resetUsername = d.username || "未知用户"
          this.loginMode = "reset"
        } else {
          this.$message.error("重置链接无效或已过期")
        }
      }).catch(() => {
        this.$message.error("验证重置链接失败")
      })
    },
    switchToLogin() {
      this.loginMode = "login"
      this.forgotSent = false
      this.forgotForm = { username: "", email: "" }
      this.resetForm = { new_password: "", confirm_password: "" }
      this.resetPwdLevel = 0
      this.smsCodeSent = false
      this.smsForm = { phone: "", code: "" }
      this.smsCountdown = 0
      if (this.smsCountdownTimer) {
        clearInterval(this.smsCountdownTimer)
        this.smsCountdownTimer = null
      }
    },
    submitForgot() {
      if (!this.forgotForm.username || !this.forgotForm.email) {
        this.$message.warning("请输入用户名和邮箱")
        return
      }
      this.forgotLoading = true
      this.portalApi("post", "/portal/api/forgot_password", this.forgotForm).then(resp => {
        if (resp.data.code === 0) {
          this.forgotSent = true
          this.$message.success((resp.data.data && resp.data.data.message) || "重置邮件已发送")
        } else {
          this.$message.success((resp.data.data && resp.data.data.message) || resp.data.msg || "如果账号匹配，重置邮件已发送")
          this.forgotSent = true
        }
      }).catch(() => {
        this.$message.error("请求失败，请稍后重试")
      }).finally(() => {
        this.forgotLoading = false
      })
    },
    submitReset() {
      if (!this.resetForm.new_password) {
        this.$message.warning("请输入新密码")
        return
      }
      const pwdErr = this.validatePassword(this.resetForm.new_password)
      if (pwdErr) {
        this.$message.warning(pwdErr)
        return
      }
      if (this.resetForm.new_password !== this.resetForm.confirm_password) {
        this.$message.warning("两次输入的密码不一致")
        return
      }
      this.resetLoading = true
      this.portalApi("post", "/portal/api/reset_password", {
        token: this.resetToken,
        new_password: this.resetForm.new_password,
      }).then(resp => {
        if (resp.data.code === 0) {
          this.$message.success("密码重置成功，请使用新密码登录")
          this.switchToLogin()
        } else {
          this.$message.error(resp.data.msg)
        }
      }).catch(() => {
        this.$message.error("请求失败，请稍后重试")
      }).finally(() => {
        this.resetLoading = false
      })
    },
    validatePassword(pwd) {
      if (!pwd || pwd.length < 8) return "密码长度至少 8 位"
      if (!/[a-zA-Z]/.test(pwd)) return "密码必须包含至少一个字母"
      if (!/[0-9]/.test(pwd)) return "密码必须包含至少一个数字"
      return ""
    },
    calcResetPwdStrength() {
      const pwd = this.resetForm.new_password
      if (!pwd) { this.resetPwdLevel = 0; return }
      let score = 0
      if (pwd.length >= 8) score++
      if (pwd.length >= 12) score++
      if (/[a-z]/.test(pwd) && /[A-Z]/.test(pwd)) score++
      if (/\d/.test(pwd)) score++
      if (/[^a-zA-Z0-9]/.test(pwd)) score++
      this.resetPwdLevel = Math.min(4, score)
    },

    calcPwdStrength() {
      const pwd = this.passwordForm.new_password
      if (!pwd) { this.pwdLevel = 0; return }
      let score = 0
      if (pwd.length >= 8) score++
      if (pwd.length >= 12) score++
      if (/[a-z]/.test(pwd) && /[A-Z]/.test(pwd)) score++
      if (/\d/.test(pwd)) score++
      if (/[^a-zA-Z0-9]/.test(pwd)) score++
      this.pwdLevel = Math.min(4, score)
    },
    showOtpQR() {
      this.portalApi("get", "/portal/api/otp/status").then(resp => {
        if (resp.data.code === 0) {
          const d = resp.data.data
          this.otpSecret = d.secret || ""
          this.otpQrBase64 = d.qr_base64 || ""
          this.otpStep = "qr"
          this.otpDialogVisible = true
        } else {
          this.$message.error(resp.data.msg)
        }
      })
    },
    openOtpBind() {
      this.otpStep = "password"
      this.otpPassword = ""
      this.otpSecret = ""
      this.otpQrBase64 = ""
      this.otpDialogVisible = true
      this.$nextTick(() => {
        const input = document.querySelector(".otp-dialog-body input")
        if (input) input.focus()
      })
    },
    submitOtpPassword() {
      if (!this.otpPassword) {
        this.$message.warning("请输入密码")
        return
      }
      this.otpRegenLoading = true
      this.portalApi("post", "/portal/api/otp/regenerate", { password: this.otpPassword }).then(resp => {
        if (resp.data.code === 0) {
          const d = resp.data.data
          this.otpSecret = d.secret || ""
          this.otpQrBase64 = d.qr_base64 || ""
          this.otpStep = "qr"
          this.otpPassword = ""
          this.user.otp_enabled = true
          this.$message.success("请立即扫码绑定二次验证")
        } else {
          this.$message.error(resp.data.msg)
        }
      }).catch(() => {
        this.$message.error("请求失败")
      }).finally(() => {
        this.otpRegenLoading = false
      })
    },
    onOtpDialogClosed() {
      this.otpStep = "password"
      this.otpPassword = ""
      this.otpSecret = ""
      this.otpQrBase64 = ""
    },
    loadCerts() {
      this.certsLoading = true
      this.portalApi("get", "/portal/api/certs").then(resp => {
        if (resp.data.code === 0) {
          this.certs = resp.data.data || []
        }
      }).catch(() => { }).finally(() => {
        this.certsLoading = false
      })
    },
    downloadCert(item) {
      this.certDownloadGroup = item.groupname
      this.certDownloadPassword = ""
      this.certDownloadVisible = true
    },
    async doDownload() {
      if (!this.certDownloadPassword || this.certDownloadPassword.length < 4) {
        this.$message.warning("P12 密码至少 4 位")
        return
      }
      try {
        const resp = await axios({
          method: "post",
          url: "/portal/api/certs/download",
          data: { groupname: this.certDownloadGroup, password: this.certDownloadPassword },
          headers: { "Content-Type": "application/json" },
          responseType: "blob",
        })
        const url = window.URL.createObjectURL(new Blob([resp.data]))
        const a = document.createElement("a")
        a.href = url
        a.download = this.certDownloadGroup + ".p12"
        document.body.appendChild(a)
        a.click()
        document.body.removeChild(a)
        window.URL.revokeObjectURL(url)
        this.certDownloadVisible = false
        this.certDownloadPassword = ""
        this.certDownloadGroup = ""
      } catch (e) {
        this.$message.error("下载失败")
      }
    },
    formatCertTime(t) {
      if (!t) return "-"
      if (typeof t === "number") return new Date(t * 1000).toLocaleDateString("zh-CN")
      const d = new Date(t)
      if (!isNaN(d.getTime())) return d.toLocaleDateString("zh-CN")
      return String(t).slice(0, 10)
    },
    loadDevices() {
      this.devicesLoading = true
      this.portalApi("get", "/portal/api/devices").then(resp => {
        if (resp.data.code === 0) {
          this.devices = (resp.data.data || []).map(d => ({ ...d, _kicking: false }))
        }
      }).catch(() => { }).finally(() => {
        this.devicesLoading = false
      })
    },
    loadWebVpnApps() {
      this.webvpnLoading = true
      // 先取域名，再取应用列表
      this.portalApi("get", "/webvpn/login-config").then(cfgResp => {
        if (cfgResp.data.code === 0) {
          this.webvpnDomain = cfgResp.data.data.domain || ""
        }
      }).catch(() => { })
      this.portalApi("get", "/webvpn/my-apps").then(resp => {
        if (resp.data.code === 0) {
          this.webvpnApps = resp.data.data || []
        }
      }).catch(() => { }).finally(() => {
        this.webvpnLoading = false
      })
    },
    kickDevice(device) {
      device._kicking = true
      this.portalApi("post", "/portal/api/devices/offline", { token: device.token }).then(resp => {
        if (resp.data.code === 0) {
          this.$message.success("已断开该设备连接")
          this.devices = this.devices.filter(d => d.token !== device.token)
        } else {
          this.$message.error(resp.data.msg)
          device._kicking = false
        }
      }).catch(() => {
        this.$message.error("操作失败")
        device._kicking = false
      })
    },
    formatDeviceTime(t) {
      if (!t) return ""
      // 后端返回 "2006-01-02 15:04:05"，前端转为友好显示
      const d = new Date(t.replace(/-/g, "/"))
      if (isNaN(d.getTime())) return t
      const now = new Date()
      const diffMs = now - d
      const diffMin = Math.floor(diffMs / 60000)
      if (diffMin < 1) return "刚刚"
      if (diffMin < 60) return diffMin + " 分钟前"
      const diffHour = Math.floor(diffMin / 60)
      if (diffHour < 24) return diffHour + " 小时前"
      const diffDay = Math.floor(diffHour / 24)
      if (diffDay < 30) return diffDay + " 天前"
      return d.toLocaleDateString("zh-CN")
    },
    deviceLabel(d) {
      if (d.device_type) {
        let label = d.device_type
        if (d.platform_version) label += " " + d.platform_version
        return label
      }
      if (d.client === "mobile") return "移动设备"
      if (d.client === "pc") return "桌面设备"
      return d.transport === "UDP" ? "DTLS 设备" : "TCP 设备"
    },
    deviceTypeIcon(d) {
      if (d.client === "mobile") return "el-icon-mobile-phone device-icon-mobile"
      if (d.client === "pc") return "el-icon-s-platform device-icon-desktop"
      // 根据 device_type 猜测
      const dt = (d.device_type || "").toLowerCase()
      if (dt.includes("ios") || dt.includes("iphone") || dt.includes("ipad") || dt.includes("android"))
        return "el-icon-mobile-phone device-icon-mobile"
      return "el-icon-s-platform device-icon-desktop"
    },
    deviceDuration(t) {
      if (!t) return ""
      const d = new Date(t.replace(/-/g, "/"))
      if (isNaN(d.getTime())) return ""
      const diffMs = Date.now() - d.getTime()
      if (diffMs < 0) return ""
      const diffMin = Math.floor(diffMs / 60000)
      if (diffMin < 1) return "刚刚上线"
      if (diffMin < 60) return "已连接 " + diffMin + " 分钟"
      const h = Math.floor(diffMin / 60)
      const m = diffMin % 60
      if (h < 24) return "已连接 " + h + "h" + (m > 0 ? m + "m" : "")
      const day = Math.floor(h / 24)
      const rh = h % 24
      return "已连接 " + day + "d" + (rh > 0 ? rh + "h" : "")
    },
    startDevicesPoll() {
      this.stopDevicesPoll()
      this.devicesTimer = setInterval(() => {
        this.loadDevices()
      }, 30000)
    },
    stopDevicesPoll() {
      if (this.devicesTimer) {
        clearInterval(this.devicesTimer)
        this.devicesTimer = null
      }
    },
    logout() {
      this.stopDevicesPoll()
      // WebVPN 单点登出：清除独立 webvpn_session，不影响门户登录态
      this.portalApi("post", "/webvpn/logout").catch(() => { })
      this.portalApi("post", "/portal/api/logout").finally(() => {
        this.loggedIn = false
        this.user = {}
        this.dashboard = {}
        this.applyDashboardTheme()
        this.devices = []
        this.webvpnApps = []
        this.loginMode = "login"
        this.challengeCode = ""
        this.challengeSession = ""
      })
    },
    replaceServerAddr(html) {
      const addr = this.user.server_addr || "vpn.example.com"
      return String(html).replace(/\{\{server_addr\}\}/g, addr)
    },
    copyText(text) {
      if (!text) return
      if (navigator.clipboard) {
        navigator.clipboard.writeText(text).then(() => {
          this.$message.success("已复制到剪贴板")
        })
      } else {
        const ta = document.createElement("textarea")
        ta.value = text
        ta.style.position = "fixed"; ta.style.left = "-9999px"
        document.body.appendChild(ta)
        ta.select()
        document.execCommand("copy")
        document.body.removeChild(ta)
        this.$message.success("已复制到剪贴板")
      }
    },
    scrollTo(id) {
      const el = document.getElementById(id)
      if (el) el.scrollIntoView({ behavior: "smooth", block: "start" })
    },
    getLimitTimestamp() {
      if (!this.user.limittime) return 0
      return typeof this.user.limittime === "number"
        ? this.user.limittime
        : Date.parse(this.user.limittime) / 1000
    },
    remainingDaysCalc(ts) {
      if (!ts) return null
      const now = Date.now() / 1000
      return Math.ceil((ts - now) / 86400)
    },
    formatBandwidth(bytePerSec) {
      if (!bytePerSec) return "-"
      // 后端 Policy.Bandwidth 单位是 Byte/s，换算为 Mbps 显示
      // 1 Mbps = 1,000,000 bps = 125,000 Byte/s
      const mbps = bytePerSec * 8 / 1000000
      if (mbps >= 1000) return (mbps / 1000).toFixed(1) + " Gbps"
      if (mbps >= 1) return mbps.toFixed(1) + " Mbps"
      return (bytePerSec * 8 / 1000).toFixed(0) + " Kbps"
    },
    formatTraffic(bytes) {
      if (!bytes) return "-"
      // 流量配额使用 IEC 二进制单位（1 GB = 1024^3 Byte）
      const GB = 1024 * 1024 * 1024
      const MB = 1024 * 1024
      const KB = 1024
      if (bytes >= GB) return (bytes / GB).toFixed(2) + " GB"
      if (bytes >= MB) return (bytes / MB).toFixed(2) + " MB"
      if (bytes >= KB) return (bytes / KB).toFixed(2) + " KB"
      return bytes + " B"
    },
    resetLabel(period) {
      switch (period) {
        case 'daily': return '/日'
        case 'weekly': return '/周'
        case 'monthly': return '/月'
        default: return ''
      }
    },
  },
}
</script>

<style scoped>
/* 根容器 */
.portal-page {
  min-height: 100vh;
  overflow-x: hidden;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC",
    "Hiragino Sans GB", "Microsoft YaHei", "Helvetica Neue", Arial, sans-serif;
  -webkit-font-smoothing: antialiased;
}

/* 登录页 */
.portal-login-bg {
  position: fixed;
  inset: 0;
  z-index: 0;
  background: linear-gradient(135deg, #1b2138 0%, #2a3a5c 40%, #1a3668 100%);
}

.bg-circle {
  position: absolute;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.03);
}

.bg-circle-1 {
  width: 420px;
  height: 420px;
  top: -120px;
  right: -80px;
}

.bg-circle-2 {
  width: 340px;
  height: 340px;
  bottom: -100px;
  left: -60px;
}

.bg-circle-3 {
  width: 260px;
  height: 260px;
  top: 40%;
  left: 60%;
  background: rgba(64, 158, 255, 0.06);
}

.portal-login-card {
  position: relative;
  z-index: 1;
  width: 440px;
  margin: 8vh auto 0;
  background: var(--bg-card);
  border-radius: 16px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3), 0 0 0 1px rgba(255, 255, 255, 0.08);
  padding: 40px 40px 32px;
}

.portal-brand {
  text-align: center;
  margin-bottom: 28px;
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

.portal-features {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 10px;
  margin-bottom: 24px;
}

.feat-item {
  background: var(--bg-hover);
  border: 1px solid #edf1f7;
  border-radius: 8px;
  padding: 12px 10px;
  text-align: center;
  transition: all var(--transition-fast);
}

.feat-item:hover {
  border-color: var(--border-color);
  background: var(--bg-card);
}

.feat-icon {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  margin: 0 auto 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 15px;
}

.feat-status {
  background: var(--color-primary-bg);
  color: var(--color-primary);
}

.feat-group {
  background: var(--success-bg);
  color: var(--color-success);
}

.feat-security {
  background: var(--danger-bg);
  color: var(--color-danger);
}

.feat-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.feat-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
}

.feat-desc {
  font-size: 11px;
  color: var(--text-secondary);
}

.portal-login-form {
  margin-bottom: 18px;
}

.portal-login-form .el-input__inner {
  height: 44px;
  line-height: 44px;
  font-size: 14px;
}

.login-submit-btn {
  width: 100%;
  height: 44px;
  font-size: 15px;
  font-weight: 600;
  letter-spacing: 4px;
  margin-top: 4px;
}

.forgot-link-wrap {
  text-align: right;
  margin-top: 8px;
}

.forgot-link {
  font-size: 13px;
  color: var(--color-primary);
  cursor: pointer;
}

.forgot-sep {
  font-size: 12px;
  color: var(--text-placeholder);
  margin: 0 6px;
  user-select: none;
}

.sms-login-form {
  width: 100%;
}

.sms-step-phone {
  width: 100%;
}

.sms-step-phone .el-form {
  margin-bottom: 0;
}

.sms-step-code {
  width: 100%;
}

.sms-code-header {
  text-align: center;
  margin-bottom: 20px;
}

.sms-icon {
  font-size: 36px;
  color: var(--color-primary);
  display: block;
  margin-bottom: 10px;
}

.sms-code-title {
  font-size: 16px;
  font-weight: 600;
  margin: 0 0 6px;
  color: var(--text-primary);
}

.sms-code-desc {
  font-size: 13px;
  color: var(--text-secondary);
  margin: 0;
}

.sms-code-input .el-input__inner {
  font-size: 20px;
  letter-spacing: 8px;
  text-align: center;
}

.sms-actions {
  text-align: center;
  margin-top: 12px;
  font-size: 13px;
}

.sms-actions a {
  color: var(--color-primary);
  cursor: pointer;
}

.sms-resend {
  color: var(--color-primary);
  cursor: pointer;
}

.sms-resend-disabled {
  color: var(--text-placeholder) !important;
  cursor: not-allowed !important;
}

.sms-actions-sep {
  margin: 0 6px;
  color: var(--text-placeholder);
}

.sms-switch-link {
  text-align: center;
  margin-top: 12px;
}

.sms-switch-link a {
  font-size: 13px;
  color: var(--color-primary);
  cursor: pointer;
}

.forgot-link:hover {
  text-decoration: underline;
}

.forgot-form {
  padding-top: 4px;
}

.forgot-form .form-title {
  font-size: 16px;
  font-weight: 650;
  color: var(--text-primary);
  text-align: center;
  margin-bottom: 20px;
}

.forgot-form .el-input__inner {
  height: 44px;
  line-height: 44px;
  font-size: 14px;
}

.forgot-success {
  text-align: center;
  padding: 16px 0;
}

.forgot-success i {
  font-size: 48px;
  color: var(--color-success);
  margin-bottom: 12px;
}

.forgot-success p {
  font-size: 14px;
  color: var(--text-regular);
  margin: 8px 0 0;
}

.forgot-back {
  text-align: center;
  margin-top: 12px;
}

.forgot-back a {
  font-size: 13px;
  color: var(--text-secondary);
  cursor: pointer;
}

.forgot-back a:hover {
  color: var(--color-primary);
}

.reset-username {
  font-size: 14px;
  color: var(--text-regular);
  text-align: center;
  margin-bottom: 16px;
  padding: 8px;
  background: var(--bg-hover);
  border-radius: 6px;
}

.sso-section {
  text-align: center;
}

.sso-divider {
  display: flex;
  align-items: center;
  gap: 12px;
  color: var(--text-secondary);
  font-size: 12px;
  margin-bottom: 12px;
}

.sso-divider::before,
.sso-divider::after {
  content: "";
  flex: 1;
  height: 1px;
  background: var(--border-color-light);
}

.sso-buttons {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}

.sso-count-1 {
  grid-template-columns: 1fr;
  justify-items: center;
}

.sso-count-1 .sso-btn {
  width: 100%;
  max-width: 280px;
}

.sso-count-3 {
  grid-template-columns: 1fr 1fr 1fr;
}

.sso-count-3 .sso-btn {
  padding-left: 8px;
  padding-right: 8px;
  font-size: 13px;
  white-space: nowrap;
}

.sso-btn {
  margin-left: 0 !important;
  border-radius: 6px;
  font-weight: 500;
}

.sso-btn:hover {
  transform: translateY(-1px);
}

.sso-wxwork {
  color: #07c160;
  border-color: #b7ebd0;
}

.sso-wxwork:hover {
  background: #f0faf4;
  border-color: #07c160;
}

.sso-feishu {
  color: #3370ff;
  border-color: #b8cfff;
}

.sso-feishu:hover {
  background: var(--color-primary-bg);
  border-color: #3370ff;
}

.sso-dingtalk {
  color: #ff6a00;
  border-color: #ffcfb0;
}

.sso-dingtalk:hover {
  background: #fff3eb;
  border-color: #ff6a00;
}

/* 门户首页 */
.portal-logged-in {
  background: var(--content-bg);
}

.portal-dash-header {
  height: 56px;
  background: var(--bg-card);
  border-bottom: 1px solid var(--border-color-light);
  box-shadow: var(--header-shadow);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 28px;
  position: sticky;
  top: 0;
  z-index: 10;
}

.header-brand {
  display: flex;
  align-items: center;
  gap: 10px;
}

.header-logo {
  font-size: 22px;
  color: var(--color-primary);
}

.header-logo-img {
  width: 26px;
  height: 26px;
  object-fit: contain;
}

.header-title {
  font-size: 16px;
  font-weight: 650;
  color: var(--text-primary);
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.header-greeting {
  font-size: 14px;
  color: var(--text-regular);
}

.portal-dash-main {
  max-width: 1200px;
  margin: 0 auto;
  padding: 24px 28px;
}

.welcome-banner {
  background: linear-gradient(135deg, #1a3668 0%, #2a4a7f 50%, #1b3a6b 100%);
  border-radius: 12px;
  padding: 24px 32px;
  margin-bottom: 20px;
  color: var(--text-inverse);
  position: relative;
  overflow: hidden;
}

.welcome-banner::after {
  content: "";
  position: absolute;
  width: 280px;
  height: 280px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.03);
  right: -60px;
  top: -80px;
}

.welcome-body {
  position: relative;
  z-index: 1;
}

.welcome-greeting {
  margin: 0 0 10px;
  font-size: 22px;
  font-weight: 650;
}

.welcome-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  opacity: 0.85;
  margin-bottom: 10px;
}

.meta-item {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.meta-divider {
  opacity: 0.5;
}

.welcome-groups {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.welcome-groups>i {
  font-size: 13px;
  opacity: 0.7;
}

.group-sep {
  width: 2px;
}

.frag {
  display: contents;
}

.text-success {
  color: var(--color-success) !important;
}

.text-danger {
  color: var(--color-danger) !important;
}

.text-warning {
  color: var(--color-warning) !important;
}

.stats-row {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 14px;
  margin-bottom: 20px;
}

.stat-card {
  background: var(--bg-card);
  border-radius: 10px;
  padding: 16px 18px;
  display: flex;
  align-items: center;
  gap: 14px;
  border: 1px solid var(--border-color-light);
  transition: box-shadow var(--transition-fast);
}

.stat-card:hover {
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.06);
}

.stat-card.clickable {
  cursor: pointer;
}

.stat-icon {
  width: 44px;
  height: 44px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  flex-shrink: 0;
}

.stat-body {
  min-width: 0;
}

.stat-value {
  font-size: 16px;
  font-weight: 650;
  color: var(--text-primary);
  word-break: break-all;
}

.stat-value.server-addr {
  font-size: 13px;
  font-family: "SFMono-Regular", Consolas, monospace;
}

.stat-label {
  font-size: 12px;
  color: var(--text-secondary);
  margin-top: 2px;
}

.dash-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 400px;
  gap: 18px;
  margin-bottom: 20px;
}

.dash-left,
.dash-right {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.dash-left .table-card,
.dash-right .table-card {
  margin-bottom: 0;
}

.table-card {
  border-radius: 10px;
  border: 1px solid var(--border-color-light);
}

.table-card>>>.el-card__header {
  padding: 14px 20px;
  border-bottom: 1px solid var(--border-color-light);
}

.table-card>>>.el-card__body {
  padding: 16px 20px;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.card-title {
  font-size: 15px;
  font-weight: 650;
  color: var(--text-primary);
}

.card-title i {
  margin-right: 6px;
  color: var(--color-primary);
}

.stat-copy-btn {
  padding: 0 4px;
  margin-left: 6px;
  font-size: 12px;
  color: var(--color-primary);
  flex-shrink: 0;
}

.stat-label-row {
  display: flex;
  align-items: center;
}

.server-addr {
  word-break: break-all;
}

.client-tabs>>>.el-tabs__header {
  margin-bottom: 8px;
}

.client-tabs>>>.el-tabs__item {
  font-size: 13px;
}

.client-step {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 10px 0;
  font-size: 13px;
  color: var(--text-regular);
  line-height: 1.7;
}

.client-step+.client-step {
  border-top: 1px dashed #eee;
}

.step-num {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--color-primary);
  color: var(--text-inverse);
  font-size: 11px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  margin-top: 1px;
}

.addr-code {
  display: inline-block;
  background: var(--bg-hover);
  padding: 2px 8px;
  border-radius: 4px;
  font-family: "SFMono-Regular", Consolas, monospace;
  font-size: 12px;
  color: var(--color-primary);
  border: 1px solid #e8eaed;
}

.group-item {
  border: 1px solid #edf1f7;
  border-radius: 8px;
  margin-bottom: 10px;
  transition: border-color var(--transition-fast);
}

.group-item:last-child {
  margin-bottom: 0;
}

.group-item:hover {
  border-color: var(--border-color);
}

.group-item-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 14px;
  cursor: pointer;
}

.group-name-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.group-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
}

.group-arrow {
  font-size: 12px;
  color: var(--text-secondary);
  transition: transform var(--transition-fast);
}

.group-item-body {
  padding: 0 14px 12px;
}

.group-info-row {
  display: flex;
  gap: 10px;
  padding: 6px 0;
  font-size: 13px;
  align-items: flex-start;
}

.info-label {
  color: var(--text-secondary);
  white-space: nowrap;
  min-width: 60px;
  flex-shrink: 0;
}

.info-val {
  color: var(--text-primary);
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
}

.auth-arrow {
  font-size: 10px;
  color: var(--text-placeholder);
}

.group-policy-box {
  margin-top: 8px;
  padding: 10px 12px;
  background: var(--bg-hover);
  border: 1px solid #e8eaed;
  border-radius: 6px;
}

.policy-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 6px;
}

.policy-summary {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  font-size: 12px;
  color: var(--text-secondary);
}

.policy-summary span {
  display: inline-flex;
  align-items: center;
  gap: 3px;
}

.policy-detail {
  padding: 4px 0;
}

.policy-name {
  font-size: 15px;
  font-weight: 650;
  color: var(--text-primary);
}

.policy-note {
  font-size: 13px;
  color: var(--text-secondary);
  margin-top: 4px;
}

.mt-sm {
  margin-top: 10px;
}

.pwd-form {
  padding-top: 4px;
}

.pwd-form .el-form-item {
  margin-bottom: 14px;
}

.full-btn {
  width: 100%;
}

.pwd-strength {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: -4px 0 12px;
}

.strength-bar {
  flex: 1;
  height: 4px;
  background: #edf1f7;
  border-radius: 2px;
  overflow: hidden;
}

.strength-fill {
  height: 100%;
  border-radius: 2px;
  transition: width .3s, background .3s;
}

.strength-fill.level-1 {
  width: 25%;
  background: var(--color-danger);
}

.strength-fill.level-2 {
  width: 50%;
  background: var(--color-warning);
}

.strength-fill.level-3 {
  width: 75%;
  background: var(--color-primary);
}

.strength-fill.level-4 {
  width: 100%;
  background: var(--color-success);
}

.strength-text {
  font-size: 12px;
  white-space: nowrap;
}

.text-level-1 {
  color: var(--color-danger);
}

.text-level-2 {
  color: var(--color-warning);
}

.text-level-3 {
  color: var(--color-primary);
}

.text-level-4 {
  color: var(--color-success);
}

.no-pwd-hint {
  text-align: center;
  padding: 20px 0;
  color: var(--text-secondary);
}

.hint-icon {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background: var(--warning-bg);
  color: var(--color-warning);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 22px;
  margin: 0 auto 12px;
}

.no-pwd-hint p {
  margin: 0;
  font-size: 13px;
  line-height: 1.7;
}

.otp-disabled-hint {
  text-align: center;
  padding: 16px 0;
  color: var(--text-secondary);
  font-size: 13px;
}

.otp-disabled-hint p {
  margin: 0;
  line-height: 1.7;
}

.otp-status-info {
  display: flex;
  gap: 10px;
  padding: 4px 0;
}

.portal-footer {
  text-align: center;
  padding: 20px 0 32px;
  font-size: 12px;
  color: var(--text-secondary);
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 6px 8px;
}

.footer-divider {
  margin: 0;
  opacity: 0.4;
}

.portal-login-footer {
  margin-top: 18px;
  padding-top: 14px;
  border-top: 1px solid var(--border-color-light);
  text-align: center;
  font-size: 12px;
  color: var(--text-secondary);
  line-height: 1.6;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 6px 8px;
}

/* 窄屏页脚换行后，竖线分隔符已无意义，隐藏避免孤立竖线 */
@media (max-width: 480px) {
  .portal-footer .footer-divider,
  .portal-login-footer .footer-divider {
    display: none;
  }
}

.empty-hint {
  text-align: center;
  padding: 24px 0;
  color: var(--text-secondary);
  font-size: 13px;
}

.empty-hint i {
  margin-right: 4px;
}

/* OTP 验证步骤（Portal 二次验证，与 Admin Login 统一风格） */
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

.force-pwd-section {
  text-align: center;
}

.force-pwd-header {
  margin-bottom: 24px;
}

.force-pwd-icon {
  font-size: 44px;
  color: var(--color-warning);
  margin-bottom: 12px;
}

.force-pwd-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0 0 8px 0;
}

.force-pwd-desc {
  font-size: 13px;
  color: var(--text-secondary);
  margin: 0;
}

.force-pwd-form {
  margin-top: 8px;
  text-align: left;
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

.otp-dialog-body {
  text-align: center;
}

.otp-alert {
  margin-bottom: 16px;
  text-align: left;
}

.otp-qr-wrap {
  margin-bottom: 14px;
}

.otp-qr-img {
  width: 180px;
  height: 180px;
  border: 1px solid #edf1f7;
  border-radius: 8px;
}

.otp-secret-row {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  margin-bottom: 10px;
  font-size: 13px;
}

.otp-secret-label {
  color: var(--text-secondary);
  white-space: nowrap;
  flex-shrink: 0;
}

.otp-secret-code {
  background: var(--bg-hover);
  padding: 2px 6px;
  border-radius: 4px;
  font-family: monospace;
  font-size: 12px;
}

.otp-tip {
  font-size: 12px;
  color: var(--text-secondary);
  margin: 0 0 10px 0;
}

.cert-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.cert-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border: 1px solid #edf1f7;
  border-radius: 8px;
  padding: 12px 14px;
  transition: border-color var(--transition-fast);
}

.cert-item:hover {
  border-color: var(--border-color);
}

.cert-item.cert-disabled {
  opacity: 0.6;
}

.cert-item-main {
  min-width: 0;
  flex: 1;
}

.cert-item-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
}

.cert-groupname {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
}

.cert-item-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.cert-meta-text {
  font-size: 12px;
  color: var(--text-secondary);
}

.cert-item-footer {
  margin-top: 6px;
  font-size: 12px;
  color: var(--text-secondary);
  display: flex;
  align-items: center;
  gap: 4px;
}

.cert-item-actions {
  flex-shrink: 0;
  margin-left: 12px;
}

.cert-download-body {
  text-align: center;
}

.cert-download-tip {
  font-size: 13px;
  color: var(--text-secondary);
  margin: 0 0 14px;
  line-height: 1.6;
}

.download-dialog-body {
  font-size: 13px;
  color: var(--text-regular);
  line-height: 1.8;
  word-break: break-word;
}

/* 在线设备卡片 */
.devices-card {
  border-radius: 10px;
  border: 1px solid #d6e4ff;
  background: linear-gradient(135deg, #fafcff 0%, #f0f7ff 100%);
  margin-bottom: 20px;
}

.devices-card>>>.el-card__header {
  border-bottom: 1px solid #e4edf8;
}

.devices-summary {
  display: flex;
  align-items: center;
  gap: 6px;
}

.devices-summary .el-button--text {
  padding: 4px;
}

.device-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.device-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 16px;
  border: 1px solid #e8eef5;
  border-radius: 8px;
  background: var(--bg-card);
  transition: border-color .2s;
}

.device-item:hover {
  border-color: #b8cee8;
}

.device-icon {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  flex-shrink: 0;
  align-self: center;
}

.icon-tcp {
  background: var(--color-primary-bg);
  color: var(--color-primary);
}

.icon-udp {
  background: var(--success-bg);
  color: var(--color-success);
}

.device-body {
  flex: 1;
  min-width: 0;
}

.device-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
}

.device-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
}

.device-type-icon {
  font-size: 16px;
  margin-left: 4px;
  vertical-align: -2px;
}

.device-icon-mobile {
  color: var(--color-danger);
}

.device-icon-desktop {
  color: var(--color-primary);
}

.device-duration {
  font-size: 12px;
  color: var(--text-secondary);
  white-space: nowrap;
}

.device-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--text-secondary);
  margin-bottom: 6px;
}

.device-meta-item {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  white-space: nowrap;
}

.device-meta-item i {
  font-size: 11px;
  opacity: 0.7;
}

.device-meta-item.mono {
  font-family: "SFMono-Regular", Consolas, monospace;
}

.meta-sep {
  color: var(--text-placeholder);
}

.device-traffic {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--text-secondary);
}

.traffic-item {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  white-space: nowrap;
}

.traffic-item i {
  font-size: 11px;
  opacity: 0.7;
}

.traffic-item.total {
  color: var(--text-secondary);
}

.traffic-sep {
  color: var(--text-placeholder);
}

.device-action {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
}

/* 响应式 */
@media (max-width: 960px) {
  .dash-grid {
    grid-template-columns: 1fr;
  }

  .stats-row {
    grid-template-columns: repeat(2, 1fr);
  }

  .portal-features {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 860px) {
  .portal-login-card {
    width: 90%;
    padding: 28px 24px 24px;
  }

  .portal-dash-header {
    padding: 0 16px;
  }

  .portal-dash-main {
    padding: 16px;
  }

  .dash-grid {
    grid-template-columns: 1fr;
  }

  .stats-row {
    grid-template-columns: 1fr;
  }

  .welcome-banner {
    padding: 18px 20px;
  }

  .welcome-greeting {
    font-size: 18px;
  }
}

@media (max-width: 768px) {
  .portal-dash-header {
    height: 56px;
    padding: 0 14px;
    gap: 8px;
  }

  .header-brand {
    flex: 1;
    min-width: 0;
  }

  .header-title {
    font-size: 13px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .header-logo {
    font-size: 18px;
  }

  .header-actions {
    flex-wrap: nowrap;
    gap: 6px;
    flex-shrink: 0;
  }

  .header-greeting {
    font-size: 12px;
    max-width: 80px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .portal-dash-main {
    padding: 12px 10px;
  }

  .welcome-banner {
    padding: 16px;
    border-radius: 10px;
  }

  .welcome-greeting {
    font-size: 16px;
  }

  .welcome-meta {
    flex-wrap: wrap;
    gap: 6px;
    font-size: 12px;
  }

  .meta-divider {
    display: none;
  }

  .stats-row {
    grid-template-columns: repeat(2, 1fr);
    gap: 10px;
  }

  .stat-card-box {
    padding: 14px;
  }

  .stat-icon {
    width: 36px;
    height: 36px;
    font-size: 16px;
  }

  .stat-value {
    font-size: 20px;
  }

  .stat-label {
    font-size: 11px;
  }

  .info-card {
    padding: 14px;
  }

  .info-card-label {
    font-size: 12px;
  }

  .info-card-value {
    font-size: 14px;
  }

  .portal-login-card {
    width: 92%;
    padding: 24px 20px 20px;
    margin-top: 5vh;
  }

  .portal-features {
    grid-template-columns: 1fr;
    gap: 8px;
  }

  .feat-item {
    padding: 10px 8px;
  }

  .portal-page .el-table {
    font-size: 12px;
  }

  .portal-page .el-button--small {
    padding: 6px 10px;
    font-size: 12px;
  }

  .header-actions .el-tag {
    display: none;
  }

  .theme-toggle-inline {
    width: 28px;
    height: 28px;
    flex-shrink: 0;
  }

  .theme-toggle-inline i {
    font-size: 16px;
  }
}

@media (max-width: 480px) {
  .portal-login-card {
    width: 94%;
    padding: 20px 16px 18px;
    margin-top: 3vh;
    border-radius: 12px;
  }

  .brand-name {
    font-size: 20px;
  }

  .brand-desc {
    font-size: 12px;
  }

  .stats-row {
    grid-template-columns: 1fr;
  }

  .portal-dash-main {
    padding: 10px 8px;
  }

  .welcome-banner {
    padding: 14px;
  }
}

.portal-announcement {
  margin-bottom: 16px;
}

.portal-announcement .announcement-content {
  line-height: 1.6;
  word-break: break-word;
}

.quicklinks-card {
  margin-bottom: 16px;
}

.quicklinks-body {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

.quicklink-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 14px;
  border: 1px solid var(--border-color-light);
  border-radius: 8px;
  color: var(--color-primary);
  text-decoration: none;
  font-size: 14px;
  transition: all 0.2s;
  background: var(--color-primary-bg);
}

.quicklink-item:hover {
  border-color: var(--color-primary);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
}

.ql-icon-img {
  width: 16px;
  height: 16px;
  object-fit: contain;
}

.ql-icon-text {
  font-size: 14px;
  line-height: 1;
}

.webvpn-card {
  margin-bottom: 16px;
}

.webvpn-body {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 12px;
}

.webvpn-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 12px 14px;
  border: 1px solid var(--border-color-light);
  border-radius: 8px;
  color: var(--color-primary);
  text-decoration: none;
  background: var(--color-primary-bg);
  transition: all 0.2s;
}

.webvpn-item:hover {
  border-color: var(--color-primary);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
}

.webvpn-icon {
  font-size: 18px;
}

.webvpn-name {
  font-size: 14px;
  font-weight: 500;
}

.webvpn-host {
  font-size: 12px;
  color: var(--text-color-secondary);
  word-break: break-all;
}

</style>

<style>
/* 客户端指引步骤自定义 HTML（Portal.replaceServerAddr） */
.client-step>div {
  line-height: 1.7;
  word-break: break-word;
}

.client-step>div :first-child {
  margin-top: 0;
}

.client-step>div :last-child {
  margin-bottom: 0;
}

.client-step>div a {
  color: var(--color-primary);
  text-decoration: none;
}

.client-step>div a:hover {
  text-decoration: underline;
}

.client-step>div img {
  max-width: 100%;
  border-radius: 4px;
}

.client-step>div code {
  background: var(--bg-code, #f5f5f5);
  padding: 1px 5px;
  border-radius: 3px;
  font-size: 12px;
}

.client-step>div ul,
.client-step>div ol {
  margin: 0;
  padding-left: 20px;
}

.client-step>div li {
  margin-bottom: 2px;
}

/* 客户端下载指引自定义 HTML（Portal.download-dialog-body v-html 注入） */
.download-dialog-body h3,
.download-dialog-body h4 {
  margin: 0 0 8px;
  font-size: 15px;
  color: var(--text-primary);
}

.download-dialog-body table {
  border-collapse: collapse;
  width: 100%;
}

.download-dialog-body td,
.download-dialog-body th {
  padding: 4px 8px;
}

.download-dialog-body a {
  color: var(--color-primary);
  text-decoration: none;
}

.download-dialog-body a:hover {
  text-decoration: underline;
}

.download-dialog-body img {
  max-width: 100%;
  border-radius: 4px;
}

.download-dialog-body ul,
.download-dialog-body ol {
  margin: 0;
  padding-left: 20px;
}

.download-dialog-body li {
  margin-bottom: 2px;
}

/* 门户公告自定义 HTML（Portal.announcement-content v-html 注入） */
.portal-announcement .announcement-content :first-child {
  margin-top: 0;
}

.portal-announcement .announcement-content :last-child {
  margin-bottom: 0;
}

/* ========== Portal 暗色模式切换按钮 ========== */
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
  color: #fff;
}

.theme-toggle-inline {
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

.theme-toggle-inline:hover {
  background: var(--bg-hover);
  color: var(--color-primary);
}

.theme-toggle-inline i {
  font-size: 18px;
}

/* Portal 内 Element UI 组件暗色覆盖 */
html.dark .portal-page .el-input__inner,
html.dark .portal-page .el-textarea__inner {
  background: var(--bg-hover) !important;
  border-color: var(--border-color) !important;
  color: var(--text-primary) !important;
}

html.dark .portal-page .el-input__inner::placeholder {
  color: var(--text-placeholder) !important;
}

html.dark .portal-page .el-table,
html.dark .portal-page .el-table tr,
html.dark .portal-page .el-table td.el-table__cell {
  background: var(--bg-card) !important;
  color: var(--text-regular) !important;
  border-color: var(--border-color-light) !important;
}

html.dark .portal-page .el-table th.el-table__cell {
  background: var(--bg-header) !important;
  color: var(--text-primary) !important;
}

html.dark .portal-page .el-table__body tr:hover>td.el-table__cell {
  background: var(--bg-hover) !important;
}

html.dark .portal-page .el-button--default {
  background: var(--bg-card) !important;
  color: var(--text-regular) !important;
  border-color: var(--border-color) !important;
}

html.dark .portal-page .el-dialog,
html.dark .portal-page .el-message-box {
  background: var(--bg-card) !important;
}

html.dark .portal-page .el-dialog__title,
html.dark .portal-page .el-dialog__body,
html.dark .portal-page .el-message-box__title,
html.dark .portal-page .el-message-box__content {
  color: var(--text-primary) !important;
}

html.dark .portal-page .el-tabs__item {
  color: var(--text-secondary) !important;
}

html.dark .portal-page .el-tabs__item.is-active {
  color: var(--color-primary) !important;
}

html.dark .portal-page .el-tabs__nav-wrap::after {
  background-color: var(--border-color-light) !important;
}
</style>
