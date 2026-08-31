<template>
  <el-card class="security-page">
    <div class="page-head">
      <div>
        <div class="page-title">安全设置</div>
        <div class="page-subtitle">修改管理员密码，保障账户安全</div>
      </div>
    </div>

    <div v-if="systemWarnings.length" class="notice-stack">
      <el-alert v-if="hasTempPasswordWarning"
        title="当前仍在使用临时管理员密码，请立即修改。该临时密码只会在生成时显示一次，退出后台后无法再次查看。"
        type="error" show-icon :closable="false" style="margin-bottom:8px">
      </el-alert>
      <el-alert v-for="(w, idx) in visibleSystemWarnings" :key="'warn-' + idx" :title="warningMessage(w)"
        :type="warningLevel(w)" show-icon :closable="false" style="margin-bottom:8px">
      </el-alert>
    </div>

    <div class="section">
      <div class="section-title">修改密码</div>
      <el-form :model="form" :rules="rules" ref="passwordForm" label-width="100px" size="small" class="password-form">
        <el-form-item label="当前密码" prop="old_password">
          <el-input type="password" v-model="form.old_password" placeholder="请输入当前密码"
            prefix-icon="el-icon-lock" show-password>
          </el-input>
        </el-form-item>
        <el-form-item label="新密码" prop="new_password">
          <el-input type="password" v-model="form.new_password" placeholder="请输入新密码（至少8位）"
            prefix-icon="el-icon-lock" show-password>
          </el-input>
        </el-form-item>
        <el-form-item label="确认密码" prop="confirm_password">
          <el-input type="password" v-model="form.confirm_password" placeholder="请再次输入新密码"
            prefix-icon="el-icon-lock" show-password>
          </el-input>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="saving" @click="submitForm">
            修改密码
          </el-button>
        </el-form-item>
      </el-form>
    </div>

    <div class="section">
      <div class="section-title">两步验证（OTP）</div>
      <div class="otp-status">
        <template v-if="otpEnabled">
          <el-tag type="success" size="small" style="margin-bottom:12px">已启用</el-tag>
          <p class="otp-desc">两步验证已开启，登录时需要输入动态验证码。</p>
          <div class="otp-actions">
            <el-button type="primary" size="small" icon="el-icon-view" @click="viewOtpQr">
              查看密钥 / 二维码
            </el-button>
            <el-button type="danger" size="small" icon="el-icon-close" @click="showDisableDialog = true">
              禁用两步验证
            </el-button>
          </div>
        </template>
        <template v-else>
          <el-tag type="info" size="small" style="margin-bottom:12px">未启用</el-tag>
          <p class="otp-desc">启用两步验证后，登录时除密码外还需输入 6 位动态验证码，大幅提升账户安全性。</p>
          <el-button type="success" size="small" icon="el-icon-mobile-phone" @click="enableOtp">
            启用两步验证
          </el-button>
        </template>
      </div>
    </div>

    <!-- 查看 / 启用 OTP 弹窗 -->
    <el-dialog :title="otpDialogMode === 'enable' ? '启用两步验证' : '两步验证密钥'"
      :visible.sync="otpDialogVisible" width="420px" center top="5vh" @closed="otpViewVerified = false">
      <div class="otp-dialog-content">
        <template v-if="otpDialogMode === 'enable'">
          <p class="otp-step-tip">
            使用 Google Authenticator、Microsoft Authenticator 等应用扫描下方二维码，然后输入生成的 6 位验证码完成绑定。
          </p>
          <img v-if="otpQrBase64" :src="'data:image/png;base64,' + otpQrBase64" alt="OTP QR Code" class="otp-qr-img" />
          <div class="otp-secret-box">
            <div class="otp-secret-label">密钥（手动输入）</div>
            <code class="otp-secret-text" @click="copySecret">{{ otpSecret }}</code>
            <el-button type="text" size="mini" icon="el-icon-document-copy" @click="copySecret">复制</el-button>
          </div>
          <div class="otp-verify-row">
            <el-input v-model="otpVerifyCode" placeholder="请输入 6 位验证码" size="small"
              maxlength="6" style="width:180px">
            </el-input>
            <el-button type="primary" size="small" :loading="otpConfirming" @click="confirmOtp">
              确认绑定
            </el-button>
          </div>
        </template>
        <template v-else>
          <div v-if="!otpViewVerified">
            <p class="otp-step-tip">查看密钥需要重新验证管理员密码和当前动态验证码。</p>
            <el-input v-model="otpViewPassword" type="password" placeholder="请输入管理员密码" size="small"
              show-password style="margin-bottom:12px">
            </el-input>
            <el-input v-model="otpViewOtpCode" placeholder="请输入当前 6 位验证码" size="small"
              maxlength="6" style="margin-bottom:12px">
            </el-input>
            <el-button type="primary" size="small" :loading="otpViewVerifying" @click="verifyOtpView"
              style="width:100%">
              验证并查看
            </el-button>
          </div>
          <div v-else>
            <p class="otp-step-tip">使用以下密钥或二维码将两步验证添加到您的认证器应用。</p>
            <img v-if="otpQrBase64" :src="'data:image/png;base64,' + otpQrBase64" alt="OTP QR Code" class="otp-qr-img" />
            <div class="otp-secret-box">
              <div class="otp-secret-label">密钥（手动输入）</div>
              <code class="otp-secret-text" @click="copySecret">{{ otpSecret }}</code>
              <el-button type="text" size="mini" icon="el-icon-document-copy" @click="copySecret">复制</el-button>
            </div>
          </div>
        </template>
      </div>
      <span slot="footer" v-if="otpDialogMode === 'view'">
        <el-button size="small" @click="otpDialogVisible = false; otpViewVerified = false">关闭</el-button>
      </span>
    </el-dialog>

    <!-- 禁用确认弹窗 -->
    <el-dialog title="禁用两步验证" :visible.sync="showDisableDialog" width="380px" center top="15vh">
      <p>禁用后登录将不再需要动态验证码，降低账户安全性。</p>
      <el-input v-model="otpDisableCode" placeholder="请输入当前动态验证码" size="small"
        maxlength="6" style="margin:12px 0">
      </el-input>
      <span slot="footer">
        <el-button size="small" @click="showDisableDialog = false; otpDisableCode = ''">取消</el-button>
        <el-button type="danger" size="small" :loading="otpDisabling" @click="disableOtp">确认禁用</el-button>
      </span>
    </el-dialog>

    <div class="section">
      <div class="section-title">数据加密</div>
      <div class="crypto-status">
        <template v-if="cryptoLoading">
          <el-skeleton :rows="3" animated />
        </template>
        <template v-else>
          <el-tag :type="cryptoEnabled ? 'success' : 'info'" size="small" style="margin-bottom:12px">
            {{ cryptoEnabled ? '已启用' : '未启用' }}
          </el-tag>
          <p class="crypto-desc">
            <template v-if="cryptoEnabled">
              敏感字段加密已启用，数据库中存储的密码、密钥等敏感信息均为密文。
            </template>
            <template v-else>
              敏感字段明文存储在数据库中。启用加密后，系统会使用 AES-256 自动加密所有密码、密钥等敏感字段。
            </template>
          </p>
          <div class="crypto-actions">
            <template v-if="!cryptoEnabled">
              <el-button type="primary" size="small" icon="el-icon-key" @click="showEnableDialog = true">
                生成密钥并启用
              </el-button>
              <el-button type="warning" size="small" icon="el-icon-edit" @click="showUploadDialog = true">
                导入已有密钥
              </el-button>
            </template>
            <template v-else>
              <el-button type="danger" size="small" icon="el-icon-close" @click="disableEncryption">
                关闭加密
              </el-button>
            </template>
          </div>
          <!-- 密钥备份警告 -->
          <el-alert v-if="cryptoEnabled" type="warning" :closable="false" show-icon style="margin-top:12px">
            <template slot="title">
              <strong>请立即备份加密密钥！</strong>密钥丢失将导致所有加密数据永久无法恢复。
            </template>
            <template slot="default">
              <div class="key-backup-tip">
                <p>密钥文件路径：<code>{{ cryptoKeyPath }}</code></p>
                <p>请将密钥文件复制到安全位置备份。服务器迁移或文件损坏时必须使用同一密钥才能解密数据。</p>
              </div>
            </template>
          </el-alert>
          <!-- 操作结果提示 -->
          <el-alert v-if="cryptoResult" :title="cryptoResult" :type="cryptoResultType"
            :closable="true" show-icon style="margin-top:12px" @close="cryptoResult = ''">
          </el-alert>
          <!-- 解密警告列表 -->
          <div v-if="cryptoWarnings && cryptoWarnings.length" style="margin-top:12px">
            <el-alert v-for="(w, idx) in cryptoWarnings" :key="'cwarn-' + idx"
              :title="w" type="warning" :closable="false" show-icon style="margin-bottom:4px">
            </el-alert>
          </div>
        </template>
      </div>
    </div>

    <!-- 生成密钥并启用弹窗 -->
    <el-dialog title="生成密钥并启用加密" :visible.sync="showEnableDialog" width="520px" center top="12vh">
      <div class="crypto-dialog-body">
        <div class="crypto-dialog-icon">
          <i class="el-icon-warning-outline"></i>
        </div>
        <p class="crypto-dialog-desc">
          将生成 <strong>AES-256</strong> 加密密钥并加密数据库中所有敏感字段（密码、密钥、证书等）。
          密钥文件将保存在工作目录（可通过 <code>REMLINK_ENCRYPTION_KEY_DIR</code> 环境变量更改）。
          启用后<strong>请立即备份密钥文件</strong>，丢失将导致加密数据永久无法恢复。
        </p>
      </div>
      <span slot="footer">
        <el-button size="small" @click="showEnableDialog = false">取消</el-button>
        <el-button type="primary" size="small" :loading="enablingEncryption" @click="doEnableEncryption">
          <i class="el-icon-key"></i> 生成并启用
        </el-button>
      </span>
    </el-dialog>

    <!-- 首次启用后备份密钥指引弹窗 -->
    <el-dialog title="加密已启用 - 请备份密钥文件" :visible.sync="showEnableKeyDialog" width="500px" center top="8vh"
      :close-on-click-modal="false" :close-on-press-escape="false" :show-close="false">
      <div class="key-dialog-content">
        <el-alert type="error" :closable="false" show-icon style="margin-bottom:16px">
          <template slot="title">
            <strong>请立即备份密钥文件！</strong>密钥丢失将导致所有加密数据永久无法恢复。
          </template>
        </el-alert>
        <p style="font-size:13px;color:var(--text-regular);margin-bottom:12px">
          密钥文件已生成在服务器上，请立即将其备份到安全位置。
        </p>
        <div class="key-field">
          <label>密钥文件路径</label>
          <code class="key-path-text">{{ enableKeyPath }}</code>
        </div>
        <p style="margin-top:12px;font-size:12px;color:var(--text-secondary)">
          服务器迁移或密钥文件损坏时，必须使用备份的同一密钥文件才能解密数据。
        </p>
      </div>
      <span slot="footer">
        <el-button type="primary" size="small" @click="closeEnableKeyDialog">
          我已备份，关闭
        </el-button>
      </span>
    </el-dialog>

    <!-- 导入已有密钥弹窗 -->
    <el-dialog title="导入密钥并启用加密" :visible.sync="showUploadDialog" width="520px" center top="12vh"
      @closed="uploadKey = ''">
      <div class="crypto-dialog-body">
        <div class="crypto-dialog-icon">
          <i class="el-icon-warning-outline"></i>
        </div>
        <p class="crypto-dialog-desc">
          输入已有的 <strong>AES-256</strong> 密钥（64 位十六进制），导入后将加密数据库中所有敏感字段。
        </p>
        <div class="crypto-dialog-field">
          <label>密钥（十六进制）</label>
          <el-input v-model="uploadKey" placeholder="a1b2c3d4e5f6...（共 64 个字符）"
            maxlength="64" show-word-limit class="crypto-hex-input">
          </el-input>
        </div>
      </div>
      <span slot="footer">
        <el-button size="small" @click="showUploadDialog = false">取消</el-button>
        <el-button type="primary" size="small" :loading="uploadingKey" :disabled="uploadKey.length !== 64" @click="doUploadKey">
          导入并启用
        </el-button>
      </span>
    </el-dialog>

    <div class="section">
      <div class="section-title">其他安全提示</div>
      <el-alert title="定期修改管理员密码，避免使用弱密码。" type="info" :closable="false" show-icon style="margin-bottom:8px">
      </el-alert>
      <el-alert title="若忘记管理员密码，请先停止服务，在服务器执行以下命令重置后再重启服务：" type="warning" :closable="false" show-icon>
        <template slot="default">
          <code style="display:block;margin-top:6px;padding:6px 12px;background:var(--bg-hover);border-radius:4px;font-family:Menlo,Monaco,Consolas,monospace;user-select:all">
            remlink --reset-admin-password
          </code>
        </template>
      </el-alert>
      <el-alert title="若两步验证（OTP）密钥丢失无法登录，请先停止服务，在服务器执行以下命令强制禁用后再重启服务：" type="warning" :closable="false" show-icon style="margin-top:8px">
        <template slot="default">
          <code style="display:block;margin-top:6px;padding:6px 12px;background:var(--bg-hover);border-radius:4px;font-family:Menlo,Monaco,Consolas,monospace;user-select:all">
            remlink --disable-admin-otp
          </code>
        </template>
      </el-alert>
    </div>
  </el-card>
</template>

<script>
import axios from "axios";

export default {
  name: "Security",
  created() {
    this.$emit('update:route_path', this.$route.path)
    this.$emit('update:route_name', ['系统设置', '安全设置'])
    this.getSecurityStatus()
    this.checkOtpStatus()
  },
  data() {
    var validateConfirm = (rule, value, callback) => {
      if (value !== this.form.new_password) {
        callback(new Error('两次输入的新密码不一致'));
      } else {
        callback();
      }
    };
    return {
      saving: false,
      systemWarnings: [],
      form: {
        old_password: '',
        new_password: '',
        confirm_password: '',
      },
      rules: {
        old_password: [
          {required: true, message: '请输入当前密码', trigger: 'blur'},
        ],
        new_password: [
          {required: true, message: '请输入新密码', trigger: 'blur'},
          {min: 8, message: '新密码长度不能少于 8 位', trigger: 'blur'},
        ],
        confirm_password: [
          {required: true, message: '请再次输入新密码', trigger: 'blur'},
          {validator: validateConfirm, trigger: 'blur'},
        ],
      },
      // OTP 相关
      otpEnabled: false,
      otpDialogVisible: false,
      otpDialogMode: 'enable', // 'enable' | 'view'
      otpQrBase64: '',
      otpSecret: '',
      otpVerifyCode: '',
      otpConfirming: false,
      showDisableDialog: false,
      otpDisableCode: '',
      otpDisabling: false,
      // 查看密钥时的重新验证
      otpViewVerified: false,
      otpViewPassword: '',
      otpViewOtpCode: '',
      otpViewVerifying: false,
      // 数据加密
      cryptoEnabled: false,
      cryptoLoading: true,
      cryptoResult: '',
      cryptoResultType: 'info',
      cryptoWarnings: [],
      cryptoKeyPath: '',
      // 生成密钥并启用
      showEnableDialog: false,
      enablingEncryption: false,
      // 导入已有密钥
      showUploadDialog: false,
      uploadKey: '',
      uploadingKey: false,
      // 首次启用后备份密钥指引
      showEnableKeyDialog: false,
      enableKeyPath: '',
    }
  },
  computed: {
    hasTempPasswordWarning() {
      return this.systemWarnings.some(w => this.warningCode(w) === 'admin_temp_password')
    },
    visibleSystemWarnings() {
      return this.systemWarnings.filter(w => this.warningCode(w) !== 'admin_temp_password')
    },
  },
  methods: {
    getSecurityStatus() {
      axios.get('/set/soft/status').then(resp => {
        const data = resp.data.data || {}
        this.systemWarnings = data.warnings || []
      }).catch(() => {
        this.systemWarnings = []
      })
    },
    warningMessage(w) {
      return typeof w === 'string' ? w : (w && w.message) || ''
    },
    warningCode(w) {
      return typeof w === 'string' ? '' : (w && w.code) || ''
    },
    warningLevel(w) {
      return typeof w === 'string' ? 'error' : (w && w.level) || 'error'
    },
    submitForm() {
      this.$refs.passwordForm.validate((valid) => {
        if (!valid) return false;
        this.saving = true
        axios.post('/set/change_password', {
          old_password: this.form.old_password,
          new_password: this.form.new_password,
        }).then(resp => {
          var rdata = resp.data
          if (rdata.code === 0) {
            this.$message.success('密码修改成功')
            this.form.old_password = ''
            this.form.new_password = ''
            this.form.confirm_password = ''
            this.$refs.passwordForm.resetFields()
            this.getSecurityStatus()
          } else {
            this.$message.error(rdata.msg)
          }
        }).catch((err) => {
          var msg = ''
          try { msg = err.response && err.response.data && err.response.data.msg } catch (e) { /* empty */ }
          this.$message.error(msg || '请求出错，请检查网络')
        }).finally(() => {
          this.saving = false
        });
      });
    },

    // ---- OTP 方法 ----

    checkOtpStatus() {
      axios.get('/set/otp_qr').then(resp => {
        if (resp.data.code === 0) {
          this.otpEnabled = true
        } else {
          this.otpEnabled = false
        }
      }).catch(() => {
        this.otpEnabled = false
      })
    },

    enableOtp() {
      this.otpDialogMode = 'enable'
      this.otpQrBase64 = ''
      this.otpSecret = ''
      this.otpVerifyCode = ''
      axios.post('/set/otp_generate').then(resp => {
        if (resp.data.code === 0) {
          this.otpQrBase64 = resp.data.data.qr_base64
          this.otpSecret = resp.data.data.secret
          this.otpDialogVisible = true
        } else {
          this.$message.error(resp.data.msg || '生成密钥失败')
        }
      }).catch(() => {
        this.$message.error('请求出错')
      })
    },

    viewOtpQr() {
      this.otpDialogMode = 'view'
      this.otpViewVerified = false
      this.otpViewPassword = ''
      this.otpViewOtpCode = ''
      this.otpQrBase64 = ''
      this.otpSecret = ''
      this.otpVerifyCode = ''
      this.otpDialogVisible = true
    },

    verifyOtpView() {
      if (!this.otpViewPassword) {
        this.$message.warning('请输入管理员密码')
        return
      }
      if (!this.otpViewOtpCode || this.otpViewOtpCode.length < 6) {
        this.$message.warning('请输入 6 位验证码')
        return
      }
      this.otpViewVerifying = true
      axios.post('/set/otp_qr', {
        password: this.otpViewPassword,
        otp_code: this.otpViewOtpCode,
      }).then(resp => {
        if (resp.data.code === 0) {
          this.otpQrBase64 = resp.data.data.qr_base64
          this.otpSecret = resp.data.data.secret
          this.otpViewVerified = true
        } else {
          this.$message.error(resp.data.msg || '验证失败')
        }
      }).catch((err) => {
        var msg = ''
        try { msg = err.response && err.response.data && err.response.data.msg } catch (e) { /* empty */ }
        this.$message.error(msg || '请求出错')
      }).finally(() => {
        this.otpViewVerifying = false
      })
    },

    confirmOtp() {
      if (!this.otpVerifyCode || this.otpVerifyCode.length < 6) {
        this.$message.warning('请输入 6 位验证码')
        return
      }
      this.otpConfirming = true
      axios.post('/set/otp_confirm', { otp_code: this.otpVerifyCode }).then(resp => {
        if (resp.data.code === 0) {
          this.$message.success('两步验证已启用')
          this.otpEnabled = true
          this.otpDialogVisible = false
        } else {
          this.$message.error(resp.data.msg)
        }
      }).catch((err) => {
        var msg = ''
        try { msg = err.response && err.response.data && err.response.data.msg } catch (e) { /* empty */ }
        this.$message.error(msg || '请求出错')
      }).finally(() => {
        this.otpConfirming = false
      })
    },

    disableOtp() {
      if (!this.otpDisableCode || this.otpDisableCode.length < 6) {
        this.$message.warning('请输入当前 6 位动态验证码')
        return
      }
      this.otpDisabling = true
      axios.post('/set/otp_disable', { otp_code: this.otpDisableCode }).then(resp => {
        if (resp.data.code === 0) {
          this.$message.success('两步验证已禁用')
          this.otpEnabled = false
          this.showDisableDialog = false
          this.otpDisableCode = ''
        } else {
          this.$message.error(resp.data.msg)
        }
      }).catch((err) => {
        var msg = ''
        try { msg = err.response && err.response.data && err.response.data.msg } catch (e) { /* empty */ }
        this.$message.error(msg || '请求出错')
      }).finally(() => {
        this.otpDisabling = false
      })
    },

    copySecret() {
      if (!this.otpSecret) return
      navigator.clipboard.writeText(this.otpSecret).then(() => {
        this.$message.success('密钥已复制到剪贴板')
      }).catch(() => {
        // fallback
        var ta = document.createElement('textarea')
        ta.value = this.otpSecret
        ta.style.position = 'fixed'
        ta.style.opacity = '0'
        document.body.appendChild(ta)
        ta.select()
        document.execCommand('copy')
        document.body.removeChild(ta)
        this.$message.success('密钥已复制到剪贴板')
      })
    },

    // ---- 数据加密方法 ----

    getCryptoStatus() {
      this.cryptoLoading = true
      axios.get('/set/secret/status').then(resp => {
        if (resp.data.code === 0) {
          this.cryptoEnabled = resp.data.data.enabled
          this.cryptoKeyPath = resp.data.data.key_path || ''
        }
      }).catch(() => {
        this.cryptoEnabled = false
      }).finally(() => {
        this.cryptoLoading = false
      })
    },

    // 生成密钥并启用（从弹窗触发）
    doEnableEncryption() {
      this.enablingEncryption = true
      const loading = this.$loading({ text: '正在生成密钥并加密数据...', lock: true })
      axios.post('/set/secret/enable').then(resp => {
        if (resp.data.code === 0) {
          const data = resp.data.data
          this.cryptoEnabled = true
          this.cryptoKeyPath = data.key_path || ''
          this.cryptoResult = '加密已启用，所有敏感字段已加密存储。'
          this.cryptoResultType = 'success'
          this.cryptoWarnings = []
          this.showEnableDialog = false
          // 展示备份指引弹窗
          this.enableKeyPath = data.key_path || ''
          this.showEnableKeyDialog = true
          this.$message.success('加密启用成功，请立即备份密钥文件')
        } else {
          this.$message.error(resp.data.msg || '启用加密失败')
        }
      }).catch((err) => {
        var msg = ''
        try { msg = err.response && err.response.data && err.response.data.msg } catch (e) { /* empty */ }
        this.$message.error(msg || '请求出错')
      }).finally(() => {
        this.enablingEncryption = false
        loading.close()
      })
    },

    doUploadKey() {
      var hexKey = this.uploadKey.trim()
      if (!hexKey || hexKey.length !== 64) {
        this.$message.warning('请输入 64 位十六进制密钥')
        return
      }
      this.uploadingKey = true
      const loading = this.$loading({ text: '正在导入密钥并加密数据...', lock: true })
      axios.post('/set/secret/upload', { key: hexKey }).then(resp => {
        if (resp.data.code === 0) {
          this.showUploadDialog = false
          this.uploadKey = ''
          this.cryptoEnabled = true
          this.cryptoResult = '加密已启用，所有敏感字段已加密存储。'
          this.cryptoResultType = 'success'
          this.cryptoWarnings = []
          this.$message.success('密钥导入成功，加密已启用')
          this.getCryptoStatus()
        } else {
          this.$message.error(resp.data.msg || '导入密钥失败')
        }
      }).catch((err) => {
        var msg = ''
        try { msg = err.response && err.response.data && err.response.data.msg } catch (e) { /* empty */ }
        this.$message.error(msg || '请求出错')
      }).finally(() => {
        this.uploadingKey = false
        loading.close()
      })
    },

    closeEnableKeyDialog() {
      this.showEnableKeyDialog = false
      this.enableKeyPath = ''
    },

    disableEncryption() {
      this.$confirm('系统将解密所有敏感字段并存为明文，然后删除加密密钥。确认关闭？', '关闭加密', {
        confirmButtonText: '确认关闭',
        cancelButtonText: '取消',
        type: 'warning'
      }).then(() => {
        const loading = this.$loading({ text: '正在解密数据...' })
        axios.post('/set/secret/disable').then(resp => {
          if (resp.data.code === 0) {
            this.cryptoEnabled = false
            this.cryptoResult = '加密已关闭，所有敏感字段已恢复为明文存储。'
            this.cryptoResultType = 'success'
            this.cryptoWarnings = resp.data.data.warnings || []
            this.cryptoKeyPath = ''
            this.enableKeyPath = ''
            this.showEnableKeyDialog = false
            this.$message.success('加密已关闭')
          } else {
            this.$message.error(resp.data.msg || '关闭加密失败')
          }
        }).catch((err) => {
          var msg = ''
          try { msg = err.response && err.response.data && err.response.data.msg } catch (e) { /* empty */ }
          this.$message.error(msg || '请求出错')
        }).finally(() => { loading.close() })
      }).catch(() => { /* 取消 */ })
    },
  },

  mounted() {
    this.getCryptoStatus()
  },
}
</script>

<style scoped>
.security-page {
  border-radius: var(--card-radius);
  overflow: hidden;
  border: 1px solid var(--border-color-light);
}

.page-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 24px;
  padding-bottom: 16px;
  border-bottom: 1px solid var(--border-color-light);
}

.page-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
}

.page-subtitle {
  margin-top: 4px;
  font-size: 12px;
  color: var(--text-secondary);
}

.section {
  margin-bottom: 28px;
}

.section-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 16px;
  padding-left: 12px;
  border-left: 3px solid var(--color-primary);
}

.password-form {
  max-width: 480px;
}

/* OTP 样式 */
.otp-status {
  padding: 4px 0;
}

.otp-desc {
  color: var(--text-regular);
  font-size: 13px;
  line-height: 1.6;
  margin: 0 0 12px 0;
}

.otp-actions {
  display: flex;
  gap: 10px;
}

.otp-dialog-content {
  text-align: center;
}

.otp-step-tip {
  color: var(--text-regular);
  font-size: 13px;
  line-height: 1.6;
  margin: 0 0 16px 0;
  text-align: left;
}

.otp-qr-img {
  width: 200px;
  height: 200px;
  border: 1px solid var(--border-color-light);
  border-radius: 8px;
  padding: 8px;
  margin-bottom: 16px;
}

.otp-secret-box {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  margin-bottom: 16px;
  padding: 8px 12px;
  background: var(--bg-hover);
  border-radius: 6px;
  flex-wrap: wrap;
}

.otp-secret-label {
  font-size: 12px;
  color: var(--text-secondary);
}

.otp-secret-text {
  font-family: Menlo, Monaco, Consolas, monospace;
  font-size: 13px;
  color: var(--color-primary);
  cursor: pointer;
  user-select: all;
  word-break: break-all;
}

.otp-verify-row {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  margin-top: 8px;
}

/* 数据加密 */
.crypto-status {
  padding: 4px 0;
}

.crypto-desc {
  color: var(--text-regular);
  font-size: 13px;
  line-height: 1.6;
  margin: 0 0 12px 0;
}

.crypto-actions {
  display: flex;
  gap: 10px;
}

/* 加密对话框共用样式 */
.crypto-dialog-body {
  padding: 0 4px;
}

.crypto-dialog-icon {
  text-align: center;
  margin-bottom: 12px;
}

.crypto-dialog-icon i {
  font-size: 42px;
  color: var(--color-warning);
  background: var(--warning-bg);
  width: 64px;
  height: 64px;
  line-height: 64px;
  border-radius: 50%;
  display: inline-block;
}

.crypto-dialog-desc {
  color: var(--text-regular);
  font-size: 13px;
  line-height: 1.8;
  margin: 0 0 20px 0;
  text-align: center;
}

.crypto-dialog-desc strong {
  color: var(--text-primary);
}

.crypto-dialog-field {
  margin-bottom: 16px;
}

.crypto-dialog-field label {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--text-primary);
  font-weight: 500;
  margin-bottom: 8px;
}

.crypto-field-hint {
  color: var(--text-secondary);
  cursor: help;
  font-size: 15px;
  transition: color .2s;
}

.crypto-field-hint:hover {
  color: var(--color-primary);
}

/* 十六进制密钥输入框 */
.crypto-hex-input .el-input__inner {
  font-family: Menlo, Monaco, Consolas, 'Courier New', monospace;
  letter-spacing: 0.5px;
}

.key-backup-tip {
  margin: 0;
  font-size: 12px;
  color: var(--color-warning);
  line-height: 1.8;
}

.key-backup-tip p {
  margin: 2px 0;
}

.key-backup-tip code {
  background: var(--warning-bg);
  padding: 1px 6px;
  border-radius: 3px;
  font-family: Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
}

/* 密钥查看弹窗 */
.key-dialog-content {
  text-align: left;
}

.key-dialog-desc {
  color: var(--text-regular);
  font-size: 13px;
  line-height: 1.6;
  margin: 0 0 14px 0;
}

.key-field {
  margin-bottom: 14px;
}

.key-field label {
  display: block;
  font-size: 12px;
  color: var(--text-secondary);
  margin-bottom: 6px;
  font-weight: 500;
}

.key-display-box {
  display: flex;
  align-items: flex-start;
  padding: 10px 14px;
  background: var(--danger-bg);
  border: 1px solid var(--danger-bg);
  border-radius: 6px;
  gap: 10px;
}

.key-display-text {
  font-family: Menlo, Monaco, Consolas, monospace;
  font-size: 13px;
  color: var(--color-danger);
  word-break: break-all;
  flex: 1;
  user-select: all;
  line-height: 1.6;
}

.key-path-text {
  display: block;
  padding: 8px 12px;
  background: var(--bg-hover);
  border: 1px solid var(--border-color);
  border-radius: 4px;
  font-family: Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  color: var(--text-regular);
  user-select: all;
  word-break: break-all;
}

.key-dialog-warn {
  margin: 12px 0 0 0;
  font-size: 12px;
  color: var(--color-warning);
  line-height: 1.6;
  padding: 8px 12px;
  background: var(--warning-bg);
  border-radius: 4px;
  border-left: 3px solid var(--color-warning);
}
</style>
