<template>
  <div class="cert-page">
    <el-card class="cert-card" shadow="never">
      <el-tabs v-model="activeTab" class="cert-tabs">
        <!-- ===== 自定义证书 ===== -->
        <el-tab-pane name="customCert">
          <span slot="label"><i class="el-icon-upload2"></i> 自定义证书</span>
          <div class="cert-settings-wrap">
            <div class="setting-card">
              <div class="setting-card-title"><i class="el-icon-lock"></i> 上传自定义 SSL 证书</div>
              <el-form ref="customCert" :model="customCert" label-width="100px" size="small">
                <el-form-item label="证书类型">
                  <el-radio-group v-model="customCert.slot">
                    <el-radio label="main">主证书（门户/登录）</el-radio>
                    <el-radio label="wild">WebVPN 泛域名（*.{{ webvpnDomain }}）</el-radio>
                  </el-radio-group>
                  <div class="form-tip" v-if="customCert.slot === 'wild'">
                    泛域名证书用于 WebVPN 子域（如 app.{{ webvpnDomain }}）。上传的证书必须包含 <b>*.</b>{{ webvpnDomain }} 的
                    SAN，否则浏览器会报证书不匹配。
                  </div>
                </el-form-item>
                <el-form-item label="证书文件">
                  <el-upload class="uploadCert" :before-upload="beforeCertUpload" :action="certUpload" :limit="1"
                    accept=".pem,.crt,.cer">
                    <el-button size="small" icon="el-icon-plus" slot="trigger">选择证书</el-button>
                    <el-tooltip effect="dark" content="PEM 格式证书，支持 .pem / .crt / .cer 后缀" placement="top">
                      <i class="el-icon-info help-icon-inline"></i>
                    </el-tooltip>
                  </el-upload>
                </el-form-item>
                <el-form-item label="私钥文件">
                  <el-upload class="uploadCert" :before-upload="beforeKeyUpload" :action="certUpload" :limit="1"
                    accept=".pem,.key">
                    <el-button size="small" icon="el-icon-plus" slot="trigger">选择私钥</el-button>
                    <el-tooltip effect="dark" content="PEM 格式私钥，支持 .pem / .key 后缀" placement="top">
                      <i class="el-icon-info help-icon-inline"></i>
                    </el-tooltip>
                  </el-upload>
                </el-form-item>
                <el-form-item>
                  <el-button size="small" icon="el-icon-upload" type="primary"
                    @click="submitCustomCert">上传证书</el-button>
                </el-form-item>
              </el-form>
            </div>
          </div>
        </el-tab-pane>

        <!-- ===== Let's Encrypt 证书 ===== -->
        <el-tab-pane name="letsCert">
          <span slot="label"><i class="el-icon-s-promotion"></i> Let's Encrypt</span>
          <div class="cert-settings-wrap">
            <div class="setting-card">
              <div class="setting-card-title"><i class="el-icon-s-promotion"></i> 通过 Let's Encrypt 申请免费 SSL 证书</div>
              <el-form :model="letsCert" ref="letsCert" :rules="rules" label-width="120px" size="small">
                <el-form-item label="证书类型">
                  <el-radio-group v-model="letsCert.certType">
                    <el-radio label="main">主证书（门户/登录）</el-radio>
                    <el-radio label="wild">WebVPN 泛域名（*.{{ webvpnDomain }}）</el-radio>
                  </el-radio-group>
                  <div class="form-tip" v-if="letsCert.certType === 'wild'">
                    泛域名证书会向 Let's Encrypt 申请 <b>*.</b>{{ webvpnDomain }}
                  </div>
                </el-form-item>
                <el-form-item label="域名" prop="domain">
                  <el-input v-if="letsCert.certType === 'wild'" :value="wildDomainText" disabled></el-input>
                  <el-input v-else v-model="letsCert.domain" placeholder="如 vpn.example.com"></el-input>
                  <div class="form-tip" v-if="letsCert.certType === 'wild'">
                    泛域名证书的域名由「WebVPN 域名」自动决定，无需填写。如需修改请前往 WebVPN 设置页调整。
                  </div>
                </el-form-item>
                <el-form-item label="邮箱" prop="legomail">
                  <el-input v-model="letsCert.legomail" placeholder="admin@example.com"></el-input>
                  <div class="form-tip">用于注册 Let's Encrypt 账号并接收证书到期提醒。</div>
                </el-form-item>
                <el-form-item label="DNS 服务商" prop="name">
                  <el-radio-group v-model="letsCert.name">
                    <el-radio label="aliyun">阿里云</el-radio>
                    <el-radio label="txcloud">腾讯云</el-radio>
                    <el-radio label="cfcloud">Cloudflare</el-radio>
                  </el-radio-group>
                  <div class="form-tip">用于自动添加 DNS 记录完成域名验证，需填写对应密钥。</div>
                </el-form-item>
                <el-form-item v-for="component in dnsProvider[letsCert.name]" :key="component.prop"
                  :label="component.label" :rules="component.rules">
                  <component :is="component.component" :type="component.type"
                    v-model="letsCert[letsCert.name][component.prop]"></component>
                </el-form-item>
                <el-form-item label="DNS 服务器">
                  <el-input v-model="letsCert.dnsResolver"
                    placeholder="多个用逗号分隔，留空默认用阿里 DNS (223.6.6.6,223.5.5.5)"></el-input>
                </el-form-item>
                <el-form-item>
                  <el-switch v-model="letsCert.renew" active-color="#13ce66" inactive-color="#ff4949"
                    inactive-text="自动续期" />
                </el-form-item>
                <el-form-item>
                  <el-button type="primary" icon="el-icon-s-promotion" @click="submitLetsCert" :loading="letsLoading">
                    申请证书
                  </el-button>
                  <el-button icon="el-icon-refresh" @click="resetForm('letsCert')">重置</el-button>
                </el-form-item>
              </el-form>
            </div>
          </div>
        </el-tab-pane>

        <!-- ===== 客户端证书 ===== -->
        <el-tab-pane name="clientCert">
          <span slot="label"><i class="el-icon-s-custom"></i> 客户端证书</span>
          <div class="cert-settings-wrap">
            <div class="setting-card">
              <div class="setting-card-title"><i class="el-icon-s-custom"></i> 客户端证书管理</div>

              <!-- 操作栏 -->
              <div class="action-bar">
                <el-button :type="caInitialized ? 'danger' : 'warning'" size="small" @click="initClientCA">
                  {{ caInitialized ? '强制重置客户端 CA' : '初始化客户端 CA' }}
                </el-button>
                <el-tooltip :content="caInitialized ? '强制重置将重新生成客户端CA，所有现有客户端证书将立即失效！' : '首次使用前需要初始化客户端CA'"
                  placement="top">
                  <i class="el-icon-question help-icon-inline"></i>
                </el-tooltip>
                <el-button type="primary" size="small" @click="generateClientCert">生成证书</el-button>
                <el-button type="success" size="small" icon="el-icon-plus" @click="batchGenerateClientCert">批量生成</el-button>
                <el-button size="small" :disabled="multipleSelection.length === 0" @click="batchSendCertMail">
                  <i class="el-icon-message"></i> 发送邮件
                </el-button>
                <el-button size="small" :disabled="multipleSelection.length === 0" @click="batchDownloadCerts">
                  <i class="el-icon-download"></i> 批量下载
                </el-button>
                <el-button size="small" type="danger" :disabled="multipleSelection.length === 0"
                  @click="batchDeleteCerts">
                  <i class="el-icon-delete"></i> 批量删除
                </el-button>
              </div>

              <!-- 生成证书对话框 -->
              <el-dialog title="生成客户端证书" :visible.sync="generateCertDialog" width="520px" :append-to-body="true">
                <el-form :model="generateForm" label-width="100px" size="small">
                  <el-form-item label="用户名">
                    <el-select v-model="generateForm.username" placeholder="请输入或选择用户名" filterable clearable allow-create
                      default-first-option style="width: 100%;" @change="onUserChange">
                      <el-option v-for="user in userList" :key="user.username"
                        :label="userLabel(user.username, user.nickname)" :value="user.username" />
                    </el-select>
                  </el-form-item>
                  <el-form-item label="用户组" v-if="userGroups.length > 0">
                    <el-select v-model="generateForm.groupName" placeholder="请选择用户组" style="width: 100%;"
                      @change="onGroupChange">
                      <el-option v-for="group in userGroups" :key="group" :label="group" :value="group" />
                    </el-select>
                  </el-form-item>
                  <el-form-item label="生成方式">
                    <el-radio-group v-model="generateForm.generateType">
                      <el-radio label="server">服务端生成 (P12)</el-radio>
                      <el-radio label="csr">上传 CSR</el-radio>
                    </el-radio-group>
                  </el-form-item>
                  <el-form-item v-if="generateForm.generateType === 'csr'" label="CSR 文件">
                    <el-input type="textarea" v-model="generateForm.csrData" placeholder="粘贴 CSR 内容 (PEM 格式)"
                      :rows="10" />
                  </el-form-item>
                  <el-form-item label="设备绑定">
                    <el-switch v-model="generateForm.deviceBindingEnabled" active-color="#13ce66"
                      inactive-color="#ff4949" />
                  </el-form-item>
                  <el-form-item v-if="generateForm.deviceBindingEnabled" label=" ">
                    <div class="warn-box">
                      <i class="el-icon-warning"></i>
                      开启后仅 Cisco AnyConnect 客户端支持证书认证功能，后续可手动关闭
                    </div>
                  </el-form-item>
                  <el-form-item label="最大设备数">
                    <el-input-number v-model="generateForm.maxDevices" :min="1" :max="10" :step="1" size="small"
                      style="width: 120px;" />
                    <span style="margin-left: 10px; font-size: 12px; color: var(--text-secondary);">该证书允许绑定的最大设备数</span>
                  </el-form-item>
                </el-form>
                <span slot="footer">
                  <el-button @click="generateCertDialog = false">取消</el-button>
                  <el-button type="primary" @click="confirmGenerateCert">确定生成</el-button>
                </span>
              </el-dialog>

              <!-- 批量生成证书对话框 -->
              <el-dialog title="批量生成客户端证书" :visible.sync="batchGenerateDialog" width="560px" :append-to-body="true">
                <el-form :model="batchGenerateForm" label-width="100px" size="small">
                  <el-form-item label="用户组" required>
                    <el-select v-model="batchGenerateForm.groupName" placeholder="请选择目标用户组" filterable style="width: 100%;"
                      @change="onBatchGroupChange">
                      <el-option v-for="group in allGroups" :key="group" :label="group" :value="group" />
                    </el-select>
                    <div class="batch-tip">
                      <i class="el-icon-info"></i>
                      <span>所选组下的用户需已存在，非该组成员将被自动跳过。</span>
                    </div>
                  </el-form-item>
                  <el-form-item label="用户名列表">
                    <el-checkbox v-model="batchGenerateForm.allUsers" @change="onAllUsersChange">全选该组用户</el-checkbox>
                    <div class="batch-tip">
                      <i class="el-icon-info"></i>
                      <span>勾选或留空 = 为该组全部用户生成；如需部分用户，请在下方勾选。</span>
                    </div>
                    <el-input v-model="batchUserSearch" placeholder="搜索用户名 / 昵称..." size="small"
                      prefix-icon="el-icon-search" clearable :disabled="batchGenerateForm.allUsers"
                      style="margin: 8px 0;">
                      <template slot="append" v-if="batchGenerateForm.allUsers">
                        <span style="color: var(--text-placeholder);">全选模式</span>
                      </template>
                    </el-input>
                    <div v-if="batchGenerateForm.allUsers" class="apply-empty" style="padding:12px;">
                      <i class="el-icon-info"></i>
                      <p>将为「{{ batchGenerateForm.groupName || '未选组' }}」下全部用户生成</p>
                    </div>
                    <div v-else-if="filteredGroupUsers.length > 0" class="apply-list apply-list-scroll">
                      <el-checkbox-group v-model="batchGenerateForm.usernames">
                        <div v-for="u in filteredGroupUsers" :key="u.username" class="apply-item">
                          <el-checkbox :label="u.username">
                            <span class="apply-item-name">{{ certUserLabel(u.username) }}</span>
                          </el-checkbox>
                        </div>
                      </el-checkbox-group>
                    </div>
                    <div v-else class="apply-empty">
                      <i class="el-icon-search"></i>
                      <p>该组下暂无用户，或没有匹配的用户</p>
                    </div>
                  </el-form-item>
                  <el-form-item label="设备绑定">
                    <el-switch v-model="batchGenerateForm.deviceBindingEnabled" active-color="#13ce66"
                      inactive-color="#ff4949" />
                  </el-form-item>
                  <el-form-item label="最大设备数">
                    <el-input-number v-model="batchGenerateForm.maxDevices" :min="1" :max="10" :step="1" size="small"
                      style="width: 120px;" />
                  </el-form-item>
                </el-form>
                <span slot="footer">
                  <el-button @click="batchGenerateDialog = false">取消</el-button>
                  <el-button type="primary" @click="confirmBatchGenerate" :loading="batchGenerating">开始生成</el-button>
                </span>
              </el-dialog>

              <!-- 发送证书邮件对话框 -->
              <el-dialog title="发送证书邮件" :visible.sync="sendMailDialog" width="480px" :append-to-body="true">
                <el-form :model="sendMailForm" label-width="100px" size="small">
                  <el-form-item label="P12 密码">
                    <el-input v-model="sendMailForm.password" placeholder="留空则不设置密码" show-password></el-input>
                    <div class="batch-tip">
                      <i class="el-icon-info"></i>
                      <span>安装证书时需要输入的密码，留空允许无密码安装。</span>
                    </div>
                  </el-form-item>
                  <el-form-item label="发送对象">
                    <div class="send-mail-list" v-if="sendMailForm.certs.length > 0">
                      <el-tag v-for="(c, i) in sendMailForm.certs" :key="i" size="small" type="info"
                        style="margin: 2px 4px;">
                        {{ c.username }} / {{ c.groupname }}
                      </el-tag>
                    </div>
                    <span v-else class="text-muted">未选择证书记录</span>
                  </el-form-item>
                </el-form>
                <span slot="footer">
                  <el-button @click="sendMailDialog = false">取消</el-button>
                  <el-button type="primary" @click="confirmSendMail" :loading="sendMailLoading">确认发送</el-button>
                </span>
              </el-dialog>

              <!-- 搜索栏 -->
              <div class="search-bar">
                <el-form :inline="true" :model="searchForm" size="small" class="search-form-inline">
                  <el-form-item label="用户名">
                    <el-input v-model="searchForm.username" placeholder="用户名" clearable style="width: 180px;" />
                  </el-form-item>
                  <el-form-item label="用户组">
                    <el-input v-model="searchForm.groupname" placeholder="用户组" clearable style="width: 180px;" />
                  </el-form-item>
                  <el-form-item label="状态">
                    <el-select v-model="searchForm.status" placeholder="全部" clearable style="width: 110px;">
                      <el-option label="启用" :value="0" />
                      <el-option label="禁用" :value="1" />
                      <el-option label="过期" :value="2" />
                    </el-select>
                  </el-form-item>
                  <el-form-item>
                    <el-button type="primary" @click="handleSearch">搜索</el-button>
                    <el-button @click="resetSearch">重置</el-button>
                  </el-form-item>
                </el-form>
              </div>

              <!-- 证书列表 -->
              <div class="cert-table-wrap">
                <el-table ref="certTable" :data="clientCertList" style="width: 100%" border
                  :header-cell-style="{ background: 'var(--bg-header)', color: 'var(--text-primary)', fontWeight: '600', fontSize: '13px' }"
                  @selection-change="handleSelectionChange">
                  <el-table-column type="selection" width="45" align="center"></el-table-column>
                  <el-table-column label="用户名" min-width="200" sortable class-name="cert-username-col">
                    <template slot-scope="scope">
                      <span class="cert-username-cell">{{ certUserLabel(scope.row.username) }}</span>
                    </template>
                  </el-table-column>
                  <el-table-column prop="groupname" label="用户组" min-width="120" show-overflow-tooltip
                    sortable></el-table-column>
                  <el-table-column label="设备绑定" width="75" align="center">
                    <template slot-scope="scope">
                      <el-tag :type="scope.row.device_binding_enabled ? 'success' : 'info'" size="small"
                        @click="editDeviceBinding(scope.row)" style="cursor: pointer;">
                        {{ scope.row.device_binding_enabled ? '开启' : '关闭' }}
                      </el-tag>
                    </template>
                  </el-table-column>
                  <el-table-column label="设备ID" min-width="200" class-name="cert-device-col">
                    <template slot-scope="scope">
                      <el-tooltip v-if="scope.row.device_id && scope.row.device_id.length > 0"
                        placement="top" effect="dark">
                        <div slot="content" class="device-tip-content">
                          <div v-for="(deviceId, index) in scope.row.device_id" :key="index"
                            class="device-tip-line">
                            {{ deviceId }}
                          </div>
                        </div>
                        <div class="device-cell-ellipsis">
                          <div v-for="(deviceId, index) in scope.row.device_id" :key="index" class="device-item">
                            <span class="device-id-text">{{ deviceId }}</span>
                            <el-button size="mini" type="text" class="device-unbind-btn"
                              @click="unbindDevice(scope.row, deviceId)" title="解绑此设备">
                              <i class="el-icon-unlock"></i>
                            </el-button>
                          </div>
                        </div>
                      </el-tooltip>
                      <span v-else class="text-muted">未绑定</span>
                    </template>
                  </el-table-column>
                  <el-table-column label="最大设备" width="80" align="center">
                    <template slot-scope="scope">
                      <span>{{ scope.row.max_devices }}</span>
                      <el-button size="mini" type="text" @click="editMaxDevices(scope.row)"
                        class="edit-link">改</el-button>
                    </template>
                  </el-table-column>
                  <el-table-column prop="created_at" label="创建时间" :formatter="dateFormat" width="155"
                    sortable></el-table-column>
                  <el-table-column prop="not_after" label="过期时间" :formatter="dateFormat" width="155"
                    sortable></el-table-column>
                  <el-table-column label="状态" width="65" align="center">
                    <template slot-scope="scope">
                      <el-tag :type="getStatusType(scope.row.status)" size="small">
                        {{ getStatusText(scope.row.status) }}
                      </el-tag>
                    </template>
                  </el-table-column>
                  <el-table-column label="操作" min-width="280" class-name="col-ops">
                    <template slot-scope="scope">
                      <div class="action-btns">
                        <el-button size="mini" @click="downloadCert(scope.row)">下载</el-button>
                        <el-button size="mini" @click="sendCertMail(scope.row)">发送邮件</el-button>
                        <el-button size="mini" :type="scope.row.status === 0 ? 'warning' : 'success'"
                          @click="changeCertStatus(scope.row)" :disabled="scope.row.status === 2">
                          {{ scope.row.status === 0 ? '禁用' : '启用' }}
                        </el-button>
                        <el-button size="mini" type="primary" @click="renewCert(scope.row)"
                          :disabled="scope.row.status === 2">续期</el-button>
                        <el-button size="mini" type="danger" @click="deleteCert(scope.row)">删除</el-button>
                      </div>
                    </template>
                  </el-table-column>
                </el-table>
              </div>

              <div class="pagination-wrap">
                <el-pagination @size-change="handleSizeChange" @current-change="handleCurrentChange"
                  :current-page="pagination.current" :page-sizes="[10, 20, 50, 100]" :page-size="pagination.size"
                  layout="total, sizes, prev, pager, next, jumper" :total="pagination.total" background />
              </div>
            </div>
          </div>
        </el-tab-pane>
      </el-tabs>
    </el-card>
  </div>
</template>

<script>
import axios from "axios";
import userLabel from "@/mixins/userLabel";

export default {
  mixins: [userLabel],
  name: "Cert",
  created() {
    this.$emit("update:route_path", this.$route.path);
    this.$emit("update:route_name", ["系统设置", "证书设置"]);
  },
  mounted() {
    this.getletsCert();
    // 获取 WebVPN 域名用于证书类型提示
    axios.get("/webvpn/domain").then(resp => {
      if (resp.data.code === 0) this.webvpnDomain = resp.data.data.domain || "";
    }).catch(() => { });
    // 初始加载客户端证书状态（如果当前是客户端证书tab）
    if (this.activeTab === 'clientCert') {
      this.checkCAStatus();
      this.loadClientUserInfo();
      this.loadClientCertList();
    }
  },
  watch: {
    activeTab(val) {
      if (val === 'letsCert') { this.getletsCert(); }
      else if (val === 'clientCert') {
        this.checkCAStatus();
        this.loadClientUserInfo();
        this.loadClientCertList();
      }
    },
    // 切换证书类型时清除域名的残留校验提示
    "letsCert.certType"() {
      this.$nextTick(() => {
        if (this.$refs.letsCert) this.$refs.letsCert.clearValidate("domain");
      });
    },
  },
  data() {
    return {
      activeTab: "customCert",
      webvpnDomain: "",
      customCert: { slot: "main", cert: "", key: "" },
      letsCert: {
        domain: "", legomail: "", name: "", renew: "", dnsResolver: "",
        certType: "main",
        aliyun: { apiKey: "", secretKey: "" },
        txcloud: { secretId: "", secretKey: "" },
        cfcloud: { authToken: "" },
      },
      rules: {
        domain: {
          validator: (rule, value, callback) => {
            // 泛域名证书的域名由 WebVPN 域名决定，无需校验输入框
            if (this.letsCert.certType === "wild") {
              if (!this.webvpnDomain) return callback(new Error("未配置 WebVPN 域名，无法申请泛域名证书"));
              return callback();
            }
            if (!value) return callback(new Error("请输入域名"));
            callback();
          },
          trigger: "blur",
        },
        legomail: { required: true, message: "请输入邮箱", trigger: "blur" },
        name: { required: true, message: "请选择DNS服务商", trigger: "blur" },
      },
      certUpload: "/set/other/customcert",
      dnsProvider: {
        aliyun: [
          {
            label: "APIKey", prop: "apiKey", component: "el-input", type: "password",
            rules: { required: true, message: "请输入APIKey", trigger: "blur" }
          },
          {
            label: "SecretKey", prop: "secretKey", component: "el-input", type: "password",
            rules: { required: true, message: "请输入SecretKey", trigger: "blur" }
          },
        ],
        txcloud: [
          {
            label: "SecretID", prop: "secretId", component: "el-input", type: "password",
            rules: { required: true, message: "请输入SecretID", trigger: "blur" }
          },
          {
            label: "SecretKey", prop: "secretKey", component: "el-input", type: "password",
            rules: { required: true, message: "请输入SecretKey", trigger: "blur" }
          },
        ],
        cfcloud: [
          {
            label: "AuthToken", prop: "authToken", component: "el-input", type: "password",
            rules: { required: true, message: "请输入AuthToken", trigger: "blur" }
          },
        ],
      },
      // 客户端证书
      caInitialized: false,
      generateCertDialog: false,
      generateForm: {
        username: '', groupName: '', maxDevices: 3,
        deviceBindingEnabled: false, generateType: 'server', csrData: ''
      },
      userList: [], userGroups: [], allGroups: [],
      batchGenerateDialog: false,
      batchGenerating: false,
      batchUserSearch: '',
      groupUsers: [],
      batchGenerateForm: {
        groupName: '', usernames: [], allUsers: false, maxDevices: 3, deviceBindingEnabled: false
      },
      clientCertList: [],
      searchForm: { username: '', groupname: '', status: '' },
      pagination: { current: 1, size: 10, total: 0 },
      letsLoading: false,
      // 批量选择 & 发送邮件
      multipleSelection: [],
      sendMailDialog: false,
      sendMailLoading: false,
      sendMailForm: { password: '', certs: [] },
    };
  },
  computed: {
    // 泛域名模式下域名输入框的只读展示值
    wildDomainText() {
      return this.webvpnDomain ? "*." + this.webvpnDomain : "未配置 WebVPN 域名";
    },
    // 批量生成：按搜索词过滤当前组用户，避免一次性渲染大量节点
    filteredGroupUsers() {
      if (!this.batchUserSearch) return this.groupUsers;
      const s = this.batchUserSearch.toLowerCase();
      return this.groupUsers.filter(u =>
        (u.username && u.username.toLowerCase().includes(s)) ||
        (u.nickname && u.nickname.toLowerCase().includes(s))
      );
    },
  },
  methods: {
    // ===== 自定义证书 =====
    beforeCertUpload(file) { this.customCert.cert = file; },
    beforeKeyUpload(file) { this.customCert.key = file; },
    submitCustomCert() {
      if (!this.customCert.slot) {
        this.$message.error("请选择证书类型");
        return;
      }
      const formData = new FormData();
      formData.append("slot", this.customCert.slot);
      formData.append("cert", this.customCert.cert);
      formData.append("key", this.customCert.key);
      axios.post(this.certUpload, formData).then(resp => {
        if (resp.data.code === 0) this.$message.success(resp.data.msg);
        else this.$message.error(resp.data.msg);
      }).catch(() => this.$message.error("上传失败"));
    },

    // ===== Let's Encrypt =====
    getletsCert() {
      axios.get("/set/other/getcertset").then(resp => {
        if (resp.data.code === 0) this.letsCert = Object.assign({}, this.letsCert, resp.data.data);
      });
    },
    submitLetsCert() {
      this.$refs['letsCert'].validate(valid => {
        if (!valid) return;
        this.letsLoading = true;
        axios.post("/set/other/createcert", this.letsCert).then(resp => {
          if (resp.data.code === 0) this.$message.success(resp.data.msg);
          else this.$message.error(resp.data.msg);
        }).catch(() => this.$message.error("申请失败"))
          .finally(() => { this.letsLoading = false; });
      });
    },

    // ===== 客户端证书 =====
    checkCAStatus() {
      axios.get('/set/client_cert/ca_status').then(resp => {
        if (resp.data.code === 0) this.caInitialized = resp.data.data && resp.data.data.initialized;
      });
    },
    initClientCA() {
      const isReset = this.caInitialized;
      const msg = isReset
        ? '此操作将重新生成客户端CA，<b style="color:red">所有现有客户端证书将立即失效</b>，确定继续？'
        : '确定要初始化客户端 CA 吗？';
      this.$confirm(msg, isReset ? '强制重置客户端 CA' : '初始化客户端 CA', {
        confirmButtonText: '确定', cancelButtonText: '取消',
        dangerouslyUseHTMLString: isReset, type: isReset ? 'error' : 'warning',
      }).then(() => {
        const url = isReset ? '/set/client_cert/init_ca?force=true' : '/set/client_cert/init_ca';
        axios.post(url).then(resp => {
          if (resp.data.code === 0) {
            this.$message.success(resp.data.msg || '操作成功');
            this.caInitialized = true;
          } else this.$message.error(resp.data.msg);
        });
      }).catch(() => { });
    },
    loadClientUserInfo() {
      axios.get('/set/client_cert/user_cert_info').then(resp => {
        if (resp.data.code === 0) {
          this.userList = resp.data.data.users || [];
          this.allGroups = (resp.data.data.groups || []).map(g => g.name || g);
        }
      });
    },
    certUserLabel(username) {
      const user = this.userList.find(item => item.username === username);
      return this.userLabel(username, user && user.nickname);
    },
    generateClientCert() {
      this.generateCertDialog = true;
      this.generateForm = { username: '', groupName: '', maxDevices: 3, deviceBindingEnabled: false, generateType: 'server', csrData: '' };
      this.userGroups = [];
      this.loadClientUserInfo();
    },
    onUserChange(username) {
      this.generateForm.groupName = '';
      this.userGroups = [];
      if (username) {
        const u = this.userList.find(u => u.username === username);
        if (u && u.groups) {
          this.userGroups = u.groups;
          if (this.userGroups.length === 1) this.generateForm.groupName = this.userGroups[0];
        }
      }
    },
    onGroupChange(groupName) {
      if (!groupName) return;
      axios.get('/group/cert_auth_check', { params: { groupname: groupName } }).then(resp => {
        if (resp.data.code === 0 && !resp.data.data.has_cert_auth) {
          this.$message.warning(`当前组"${groupName}"尚未配置证书认证，建议先在"用户组管理 > 编辑组 > 认证方式"中添加 TLS 证书认证步骤`);
        }
      }).catch(() => { });
    },
    confirmGenerateCert() {
      if (!this.generateForm.username) { this.$message.error('请选择或输入用户名'); return; }
      if (this.userGroups.length > 0 && !this.generateForm.groupName) { this.$message.error('请选择用户组'); return; }
      if (this.generateForm.generateType === 'csr' && !this.generateForm.csrData) { this.$message.error('请粘贴CSR内容'); return; }
      const fd = new FormData();
      fd.append('username', this.generateForm.username);
      if (this.generateForm.groupName) fd.append('group_name', this.generateForm.groupName);
      fd.append('device_binding_enabled', this.generateForm.deviceBindingEnabled);
      fd.append('max_devices', this.generateForm.maxDevices);
      if (this.generateForm.generateType === 'csr') fd.append('csr', this.generateForm.csrData);
      axios.post('/set/client_cert/generate', fd).then(resp => {
        if (resp.data.code === 0) {
          this.$message.success('证书生成成功');
          this.generateCertDialog = false;
          this.loadClientCertList();
        } else this.$message.error(resp.data.msg);
      });
    },
    // 批量生成：打开对话框并重置表单
    batchGenerateClientCert() {
      this.batchGenerateDialog = true;
      this.groupUsers = [];
      this.batchUserSearch = '';
      this.batchGenerateForm = { groupName: '', usernames: [], allUsers: false, maxDevices: 3, deviceBindingEnabled: false };
    },
    onBatchGroupChange(groupName) {
      this.batchGenerateForm.usernames = [];
      this.batchGenerateForm.allUsers = false;
      this.batchUserSearch = '';
      this.groupUsers = this.userList.filter(u => (u.groups || u.Groups || []).includes(groupName));
      if (!groupName) return;
      axios.get('/group/cert_auth_check', { params: { groupname: groupName } }).then(resp => {
        if (resp.data.code === 0 && !resp.data.data.has_cert_auth) {
          this.$message.warning(`当前组"${groupName}"尚未配置证书认证，建议先配置 TLS 证书认证步骤`);
        }
      }).catch(() => { });
    },
    onAllUsersChange(checked) {
      if (checked) this.batchGenerateForm.usernames = [];
    },
    confirmBatchGenerate() {
      const form = this.batchGenerateForm;
      if (!form.groupName) { this.$message.error('请选择目标用户组'); return; }
      // 留空（未选具体用户且未勾全选）时，默认该组全部用户
      let usernames = form.usernames;
      if (form.allUsers || usernames.length === 0) {
        usernames = this.groupUsers.map(u => u.username);
      }
      if (usernames.length === 0) {
        this.$message.error('该组下没有可生成证书的用户，请确认用户已加入该组');
        return;
      }
      const fd = new FormData();
      fd.append('usernames', usernames.join('\n'));
      fd.append('group_name', form.groupName);
      fd.append('device_binding_enabled', form.deviceBindingEnabled);
      fd.append('max_devices', form.maxDevices);
      this.batchGenerating = true;
      axios.post('/set/client_cert/batch_generate', fd).then(resp => {
        this.batchGenerating = false;
        if (resp.data.code === 0) {
          const { success, failed } = resp.data.data;
          let msg = `成功生成 ${success} 个证书`;
          if (failed && failed.length) msg += `，失败 ${failed.length} 个：` + failed.join('；');
          this.$message({ type: success ? 'success' : 'warning', message: msg, duration: 5000 });
          this.batchGenerateDialog = false;
          this.loadClientCertList();
        } else this.$message.error(resp.data.msg);
      }).catch(() => { this.batchGenerating = false; });
    },
    // 续期：对已有证书以相同参数重新签发
    renewCert(row) {
      this.$confirm(`确定要为 ${row.username}（组：${row.groupname}）续期证书吗？旧证书将被替换。`, '续期确认', {
        confirmButtonText: '确定续期', cancelButtonText: '取消', type: 'warning'
      }).then(() => {
        const fd = new FormData();
        fd.append('username', row.username);
        fd.append('group_name', row.groupname);
        axios.post('/set/client_cert/renew', fd).then(resp => {
          if (resp.data.code === 0) {
            this.$message.success('证书续期成功');
            this.loadClientCertList();
          } else this.$message.error(resp.data.msg);
        });
      }).catch(() => { });
    },
    downloadCert(row) {
      if (row.is_csr_based) {
        this.doDownloadCert(row, '');
        return;
      }
      this.$prompt('请输入证书密码，留空则不使用密码:', {
        confirmButtonText: '下载', cancelButtonText: '取消',
        inputType: 'password', inputPlaceholder: '留空则不使用密码',
      }).then(({ value }) => this.doDownloadCert(row, value)).catch(() => { });
    },
    doDownloadCert(row, password) {
      const params = new URLSearchParams();
      params.append('username', row.username);
      params.append('groupname', row.groupname);
      params.append('password', password || '');
      axios({ method: 'get', url: '/set/client_cert/download?' + params.toString(), responseType: 'blob' })
        .then(response => {
          const ct = response.headers['content-type'];
          if (ct && ct.includes('application/json')) {
            const reader = new FileReader();
            reader.onload = () => { try { this.$message.error(JSON.parse(reader.result).msg); } catch (e) { /* ignore */ } };
            reader.readAsText(response.data);
            return;
          }
          const blob = new Blob([response.data], { type: ct });
          const url = window.URL.createObjectURL(blob);
          const link = document.createElement('a');
          link.href = url;
          link.download = ct && ct.includes('application/x-pem-file') ? `${row.username}.cer` : `${row.username}.p12`;
          document.body.appendChild(link); link.click(); document.body.removeChild(link);
          window.URL.revokeObjectURL(url);
          this.$message.success('证书下载成功');
        }).catch((err) => {
          if (err.response && err.response.data) this.$message.error(err.response.data.msg || '下载失败');
        });
    },
    loadClientCertList() {
      const params = { page_size: this.pagination.size, page_index: this.pagination.current };
      if (this.searchForm.username) params.username = this.searchForm.username;
      if (this.searchForm.groupname) params.groupname = this.searchForm.groupname;
      if (this.searchForm.status !== '') params.status = this.searchForm.status;
      axios.get('/set/client_cert/list', { params }).then(resp => {
        if (resp.data.code === 0) {
          this.clientCertList = resp.data.data.list;
          this.pagination.total = resp.data.data.total;
        }
      });
    },
    handleSearch() { this.pagination.current = 1; this.loadClientCertList(); },
    resetSearch() { this.searchForm = { username: '', groupname: '', status: '' }; this.pagination.current = 1; this.loadClientCertList(); },
    handleSizeChange(val) { this.pagination.size = val; this.loadClientCertList(); },
    handleCurrentChange(val) { this.pagination.current = val; this.loadClientCertList(); },

    dateFormat(row, col, val) {
      if (!val) return '';
      return new Date(val).toLocaleString('zh-CN', { hour12: false });
    },
    getStatusText(s) { return { 0: '启用', 1: '禁用', 2: '过期' }[s] || '未知'; },
    getStatusType(s) { return { 0: 'success', 1: 'warning', 2: 'danger' }[s] || ''; },

    changeCertStatus(row) {
      const action = row.status === 0 ? '禁用' : '启用';
      this.$confirm(`确定要${action}用户 ${row.username} 的证书吗？`, '提示', { type: 'warning' }).then(() => {
        const fd = new FormData();
        fd.append('username', row.username); fd.append('groupname', row.groupname);
        axios.post('/set/client_cert/changecertstatus', fd).then(resp => {
          if (resp.data.code === 0) { this.$message.success(`证书${action}成功`); this.loadClientCertList(); }
          else this.$message.error(resp.data.msg);
        });
      });
    },
    unbindDevice(row, deviceId) {
      this.$confirm(`确定要解绑设备 "${deviceId}" 吗？`, '提示', { type: 'warning' }).then(() => {
        const fd = new FormData();
        fd.append('username', row.username); fd.append('groupname', row.groupname); fd.append('device_id', deviceId);
        axios.post('/set/client_cert/unbind_device', fd).then(resp => {
          if (resp.data.code === 0) { this.$message.success('设备解绑成功'); this.loadClientCertList(); }
          else this.$message.error(resp.data.msg);
        });
      });
    },
    editMaxDevices(row) {
      this.$prompt('请输入新的最大设备数:', '修改最大设备数', {
        confirmButtonText: '确定', cancelButtonText: '取消',
        inputValue: row.max_devices, inputType: 'number',
        inputValidator: (v) => parseInt(v) >= 1, inputErrorMessage: '请输入大于0的数字'
      }).then(({ value }) => {
        const fd = new FormData();
        fd.append('username', row.username); fd.append('groupname', row.groupname); fd.append('max_devices', parseInt(value));
        axios.post('/set/client_cert/update_max_devices', fd).then(resp => {
          if (resp.data.code === 0) { this.$message.success('修改成功'); this.loadClientCertList(); }
          else this.$message.error(resp.data.msg);
        });
      });
    },
    editDeviceBinding(row) {
      const action = row.device_binding_enabled ? '关闭' : '开启';
      const msg = row.device_binding_enabled
        ? '关闭设备绑定将清空所有已绑定的设备，确定要关闭吗？'
        : '开启设备绑定后，仅 Cisco AnyConnect 客户端支持该证书，确定要开启吗？';
      this.$confirm(msg, '提示', { type: 'warning' }).then(() => {
        const fd = new FormData();
        fd.append('username', row.username); fd.append('groupname', row.groupname);
        fd.append('device_binding_enabled', (!row.device_binding_enabled).toString());
        axios.post('/set/client_cert/update_device_binding', fd).then(resp => {
          if (resp.data.code === 0) { this.$message.success(`设备绑定${action}成功`); this.loadClientCertList(); }
          else this.$message.error(resp.data.msg);
        });
      });
    },
    deleteCert(row) {
      this.$confirm(`确定要删除用户 ${row.username} 的证书吗？`, '提示', { type: 'warning' }).then(() => {
        const fd = new FormData();
        fd.append('username', row.username); fd.append('groupname', row.groupname);
        axios.post('/set/client_cert/delete', fd).then(resp => {
          if (resp.data.code === 0) { this.$message.success('删除成功'); this.loadClientCertList(); }
          else this.$message.error(resp.data.msg);
        });
      });
    },
    // ===== 证书邮件发送 =====
    handleSelectionChange(val) {
      this.multipleSelection = val;
    },
    sendCertMail(row) {
      this.sendMailForm = {
        password: '',
        certs: [{ username: row.username, groupname: row.groupname }]
      };
      this.sendMailDialog = true;
    },
    batchSendCertMail() {
      if (this.multipleSelection.length === 0) {
        this.$message.warning('请先选择要发送邮件的证书记录');
        return;
      }
      this.sendMailForm = {
        password: '',
        certs: this.multipleSelection.map(r => ({
          username: r.username, groupname: r.groupname
        }))
      };
      this.sendMailDialog = true;
    },
    confirmSendMail() {
      this.sendMailLoading = true;
      const body = { certs: this.sendMailForm.certs, password: this.sendMailForm.password || '' };
      axios.post('/set/client_cert/send_mail', body).then(resp => {
        if (resp.data.code === 0) {
          this.$message.success(resp.data.msg || '发送成功');
          this.sendMailDialog = false;
          this.$refs.certTable && this.$refs.certTable.clearSelection();
        } else {
          this.$message.error(resp.data.msg);
        }
      }).catch(() => {
        this.$message.error('请求失败');
      }).finally(() => {
        this.sendMailLoading = false;
      });
    },
    // ===== 批量删除 =====
    batchDeleteCerts() {
      if (this.multipleSelection.length === 0) return;
      const names = this.multipleSelection.map(r => r.username + '/' + r.groupname).join('、');
      this.$confirm(`确定要删除以下 ${this.multipleSelection.length} 个证书吗？<br/>${names}`, '批量删除证书', {
        confirmButtonText: '确定删除', cancelButtonText: '取消',
        dangerouslyUseHTMLString: true, type: 'warning',
      }).then(() => {
        const body = { certs: this.multipleSelection.map(r => ({ username: r.username, groupname: r.groupname })) };
        axios.post('/set/client_cert/batch_delete', body).then(resp => {
          if (resp.data.code === 0) {
            this.$message.success(resp.data.msg || '删除成功');
            this.$refs.certTable && this.$refs.certTable.clearSelection();
            this.loadClientCertList();
          } else this.$message.error(resp.data.msg);
        });
      }).catch(() => { });
    },
    // ===== 批量下载 =====
    batchDownloadCerts() {
      if (this.multipleSelection.length === 0) return;
      this.$prompt('请输入 P12 证书密码，留空则不设置密码:', '批量下载证书', {
        confirmButtonText: '下载', cancelButtonText: '取消',
        inputType: 'password', inputPlaceholder: '留空则不设置密码',
      }).then(({ value }) => {
        const body = {
          certs: this.multipleSelection.map(r => ({ username: r.username, groupname: r.groupname })),
          password: value || '',
        };
        axios({ method: 'post', url: '/set/client_cert/batch_download', data: body, responseType: 'blob' })
          .then(response => {
            const blob = new Blob([response.data], { type: 'application/zip' });
            const url = window.URL.createObjectURL(blob);
            const link = document.createElement('a');
            link.href = url; link.download = 'certificates.zip';
            document.body.appendChild(link); link.click(); document.body.removeChild(link);
            window.URL.revokeObjectURL(url);
            this.$message.success('证书下载成功');
            this.$refs.certTable && this.$refs.certTable.clearSelection();
          }).catch(() => this.$message.error('下载失败'));
      }).catch(() => { });
    },
    resetForm(formName) {
      this.$refs[formName] && this.$refs[formName].resetFields();
    },
  },
};
</script>

<style scoped>
.cert-page {
  padding: 4px 0;
}

.cert-card {
  border-radius: var(--card-radius);
  overflow: hidden;
  border: 1px solid var(--border-color-light);
}

.cert-tabs ::v-deep .el-tabs__content {
  padding: 20px;
}

.cert-settings-wrap {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* 设置卡片（与 Other.vue 统一） */
.setting-card {
  background: var(--bg-card);
  border: 1px solid var(--border-color-light);
  border-radius: 10px;
  padding: 20px 24px 4px;
  transition: border-color 0.2s;
}

.setting-card:hover {
  border-color: #d4d9e1;
}

.setting-card-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 16px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--border-color-light);
  display: flex;
  align-items: center;
  gap: 8px;
}

.setting-card-title i {
  color: var(--color-primary);
  font-size: 16px;
}

.help-icon-inline {
  margin-left: 4px;
  color: var(--text-placeholder);
  cursor: pointer;
  font-size: 15px;
  vertical-align: -1px;
}

.help-icon-inline:hover {
  color: var(--color-primary);
}

/* 操作栏 */
.action-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 20px;
  padding: 14px 16px;
  background: var(--bg-stripe);
  border: 1px solid var(--border-color-light);
  border-radius: 8px;
}

/* 搜索栏 */
.search-bar {
  margin-bottom: 16px;
  padding: 0 2px;
}

.search-form-inline ::v-deep .el-form-item {
  margin-right: 12px;
  margin-bottom: 0;
}

.search-form-inline ::v-deep .el-form-item:last-child {
  margin-right: 0;
}

.warn-box {
  font-size: 13px;
  line-height: 22px;
  color: #E6A23C;
  font-weight: 500;
  padding: 8px 12px;
  background: #FDF6EC;
  border: 1px solid #F5DAB1;
  border-radius: 6px;
}

.warn-box i {
  margin-right: 6px;
}

.text-muted {
  color: var(--text-placeholder);
  font-size: 12px;
}

.send-mail-list {
  max-height: 200px;
  overflow-y: auto;
  padding: 4px 0;
}

.device-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 2px 0;
}

.device-id-text {
  flex: 1;
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.device-unbind-btn {
  color: var(--color-danger) !important;
  padding: 2px 4px;
}

.edit-link {
  margin-left: 4px;
  color: var(--color-primary);
  font-size: 11px;
}

.pagination-wrap {
  display: flex;
  justify-content: flex-end;
  padding-top: 16px;
}

/* 操作列按钮紧凑排列 */
.action-btns {
  display: flex;
  align-items: center;
  gap: 4px;
  white-space: nowrap;
}

.action-btns .el-button--mini {
  padding: 5px 10px;
}

/* 表格响应式容器 */
.cert-table-wrap {
  width: 100%;
  overflow-x: auto;
}

/* ========== 响应式 ========== */
@media (max-width: 1100px) {
  .action-btns .el-button--mini {
    padding: 4px 6px;
    font-size: 11px;
  }

  .cert-table-wrap ::v-deep .col-ops {
    min-width: 250px !important;
  }
}

@media (max-width: 880px) {

  /* 操作列按钮折行为两行 */
  .action-btns {
    flex-wrap: wrap;
    gap: 3px;
    white-space: normal;
    justify-content: center;
  }

  .action-btns .el-button--mini {
    padding: 3px 5px;
    font-size: 11px;
    line-height: 1.2;
    min-height: 26px;
  }

  .cert-table-wrap ::v-deep .col-ops {
    min-width: 180px !important;
  }

  .action-bar {
    flex-wrap: wrap;
    gap: 6px;
  }

  .setting-card ::v-deep .el-form-item__label {
    float: none;
    display: block;
    text-align: left;
    padding-bottom: 6px;
    line-height: 1.5;
  }

  .setting-card ::v-deep .el-form-item__content {
    margin-left: 0 !important;
  }
}

@media (max-width: 600px) {

  /* 操作列极致缩小 */
  .action-btns {
    gap: 2px;
  }

  .action-btns .el-button--mini {
    padding: 2px 3px;
    font-size: 10px;
    min-height: 22px;
  }

  .cert-table-wrap ::v-deep .col-ops {
    min-width: 140px !important;
  }

  /* 搜索栏块级并撑满，避免手机端输入框被压窄 */
  .search-bar ::v-deep .search-form-inline {
    display: flex;
    flex-direction: column;
  }

  .search-bar ::v-deep .el-form-item {
    margin-right: 0;
    margin-bottom: 8px;
    display: flex;
    flex-direction: column;
    align-items: stretch;
  }

  .search-bar ::v-deep .el-form-item__label {
    text-align: left;
    margin-bottom: 4px;
  }

  .search-bar ::v-deep .el-form-item__content {
    margin-left: 0 !important;
    line-height: normal;
  }

  .search-bar ::v-deep .el-input,
  .search-bar ::v-deep .el-select {
    width: 100% !important;
  }

  .search-bar .el-button {
    width: 100%;
    margin: 0 0 8px;
  }

  /* 分页在手机端：隐藏每页条数/跳页/总数，仅保留 首页-上-页码-下-尾，
     整条横向滚动可见，"下一页"不再被裁切 */
  .pagination-wrap {
    justify-content: flex-start;
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
  }

  .pagination-wrap ::v-deep .el-pagination {
    white-space: nowrap;
  }

  .pagination-wrap ::v-deep .el-pagination__total,
  .pagination-wrap ::v-deep .el-pagination__sizes,
  .pagination-wrap ::v-deep .el-pagination__jump {
    display: none;
  }

  /* 卡片在手机端减少内边距，内容更舒展 */
  .setting-card {
    padding: 16px 14px 4px;
  }

  /* 收敛证书页多层嵌套内边距：tabs 内容区 + 外层卡片，避免手机端过窄 */
  .cert-card {
    border-radius: 6px;
  }

  .cert-tabs ::v-deep .el-tabs__content {
    padding: 12px 2px 0;
  }

  /* action-bar 纵向全宽 */
  .action-bar {
    flex-direction: column;
    align-items: stretch;
    gap: 6px;
    padding: 10px 12px;
  }

  .action-bar .el-button {
    width: 100%;
    margin: 0 !important;
  }
}

/* 批量生成：信息提示框（蓝色 info 风格，提行展示） */
.batch-tip {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  margin: 8px 0 4px;
  padding: 8px 12px;
  font-size: 12px;
  line-height: 20px;
  color: #606266;
  background: #f4f4f5;
  border: 1px solid #e9e9eb;
  border-radius: 6px;
}

.batch-tip i {
  margin-top: 1px;
  font-size: 14px;
  flex-shrink: 0;
  color: #909399;
}

/* 批量生成：用户勾选列表（参考策略管理「应用到用户」） */
.apply-list-scroll {
  max-height: 320px;
  overflow-y: auto;
  border: 1px solid var(--border-color-light);
  border-radius: 6px;
  padding: 4px 0;
}

.apply-item {
  padding: 6px 12px;
  border-radius: 6px;
  transition: background 0.15s;
}

.apply-item:hover {
  background: var(--bg-stripe);
}

.apply-item-name {
  font-weight: 500;
}

.apply-empty {
  text-align: center;
  color: var(--text-placeholder);
  padding: 24px 0;
}

/* 用户名列：自适应换行，长三方 ID 不断裂截断完整显示 */
.cert-username-col .cell {
  white-space: normal;
  word-break: break-all;
  line-height: 20px;
}

.cert-username-cell {
  font-weight: 500;
}

/* 设备ID列：单元格正常显示，悬浮 tooltip 分行展示完整 ID */
.cert-device-col .device-cell-ellipsis {
  max-width: 100%;
}

/* tooltip 内容：每个设备一行，长 ID 自动断行 */
.device-tip-content {
  max-width: 360px;
  white-space: normal;
  word-break: break-all;
}

.device-tip-line {
  line-height: 20px;
  padding: 1px 0;
}

.device-tip-line + .device-tip-line {
  border-top: 1px solid rgba(255, 255, 255, 0.15);
  margin-top: 2px;
  padding-top: 3px;
}

.cert-device-col .device-item + .device-item {
  border-top: 1px dashed var(--border-color-light);
  margin-top: 2px;
  padding-top: 4px;
}

.apply-empty i {
  font-size: 28px;
  margin-bottom: 8px;
}
</style>
