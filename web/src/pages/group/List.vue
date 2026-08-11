<template>
  <div class="group-page">
    <!-- 统计卡片 -->
    <div class="stats-row">
      <div class="stat-card">
        <div class="stat-icon stat-icon-total">
          <i class="el-icon-s-grid"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ statTotal }}</div>
          <div class="stat-label">用户组总数</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-icon-active">
          <i class="el-icon-circle-check"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ statActive }}</div>
          <div class="stat-label">已启用</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-icon-policy">
          <i class="el-icon-s-management"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ statWithPolicy }}</div>
          <div class="stat-label">已配策略</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-icon-auth">
          <i class="el-icon-lock"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ statWithAuth }}</div>
          <div class="stat-label">组合认证</div>
        </div>
      </div>
    </div>

    <!-- 用户组表格 -->
    <el-card class="table-card" shadow="never" v-loading="loading">
      <div slot="header" class="card-header">
        <span class="card-title"><i class="el-icon-s-grid"></i> 用户组列表</span>
        <div class="card-actions">
          <el-button size="small" type="primary" icon="el-icon-plus" @click="handleEdit('')">
            添加用户组
          </el-button>
        </div>
      </div>

      <div class="group-table-wrap">
        <el-table ref="multipleTable" :data="tableData" stripe highlight-current-row border style="width:100%"
          :header-cell-style="{ background: 'var(--bg-header)', color: 'var(--text-primary)', fontWeight: '600', fontSize: '13px' }">
          <el-table-column sortable prop="id" label="ID" width="70" align="center"></el-table-column>
          <el-table-column prop="name" label="组名" min-width="140">
            <template slot-scope="scope">
              <span class="group-name">{{ scope.row.name }}</span>
              <span v-if="scope.row.note" class="group-note">{{ scope.row.note }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="policy_name" label="策略" width="160" align="center">
            <template slot-scope="scope">
              <el-tag v-if="scope.row.policy_id === 0" size="small" type="danger" effect="plain">
                {{ scope.row.policy_name }}
              </el-tag>
              <el-tag v-else-if="scope.row.policy_name" size="small" type="primary" effect="plain">
                {{ scope.row.policy_name }}
              </el-tag>
              <span v-else class="text-muted">-</span>
            </template>
          </el-table-column>
          <el-table-column label="认证方式" width="160" align="center">
            <template slot-scope="scope">
              <div v-if="scope.row.authSteps && scope.row.authSteps.length" class="auth-flow-mini">
                <span v-for="(s, i) in scope.row.authSteps" :key="'ta-' + i" style="display:contents">
                  <el-tag size="mini" effect="plain">{{ s }}</el-tag>
                  <i v-if="i < scope.row.authSteps.length - 1" class="el-icon-right auth-arrow"></i>
                </span>
              </div>
              <span v-else class="text-muted">本地密码</span>
            </template>
          </el-table-column>
          <el-table-column prop="status" label="状态" width="80" align="center">
            <template slot-scope="scope">
              <span :class="scope.row.status === 1 ? 'status-dot-online' : 'status-dot-offline'"
                class="status-dot"></span>
              <span class="status-text" :class="scope.row.status === 1 ? 'text-success' : 'text-danger'">
                {{ scope.row.status === 1 ? '启用' : '停用' }}
              </span>
            </template>
          </el-table-column>
          <el-table-column prop="updated_at" label="更新时间" :formatter="tableDateFormat" width="165"></el-table-column>
          <el-table-column label="操作" width="110" class-name="col-ops" min-width="110" align="center">
            <template slot-scope="scope">
              <el-dropdown trigger="click" @command="(cmd) => handleRowCmd(scope.row, cmd)">
                <el-button size="mini" class="action-more-btn">
                  操作<i class="el-icon-arrow-down el-icon--right"></i>
                </el-button>
                <el-dropdown-menu slot="dropdown">
                  <el-dropdown-item command="edit" icon="el-icon-edit">编辑用户组</el-dropdown-item>
                  <el-dropdown-item command="delete" icon="el-icon-delete" divided
                    class="dropdown-danger">删除用户组</el-dropdown-item>
                </el-dropdown-menu>
              </el-dropdown>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <div class="pagination-wrap">
        <el-pagination background layout="prev,pager,next" :pager-count="9" @current-change="pageChange"
          :current-page="page" :total="count" />
      </div>
    </el-card>

    <!-- 编辑弹窗 -->
    <el-dialog :close-on-click-modal="false" :title="ruleForm.id ? '编辑用户组' : '新增用户组'" :visible.sync="user_edit_dialog"
      width="900px" top="4vh" @close="closeDialog" class="group-edit-dialog">
      <el-form :model="ruleForm" :rules="rules" ref="ruleForm" label-width="100px">
        <!-- 基本信息 -->
        <div class="edit-basic-row">
          <el-form-item label="用户组ID" prop="id" v-if="ruleForm.id" class="form-item-compact">
            <el-input v-model="ruleForm.id" disabled></el-input>
          </el-form-item>
          <el-form-item label="组名" prop="name" class="form-item-compact">
            <el-input v-model="ruleForm.name" :disabled="ruleForm.id > 0" placeholder="请输入组名"></el-input>
          </el-form-item>
          <el-form-item label="备注" prop="note" class="form-item-compact">
            <el-input v-model="ruleForm.note" placeholder="备注信息"></el-input>
          </el-form-item>
          <el-form-item label="状态" prop="status" class="form-item-compact">
            <el-radio-group v-model="ruleForm.status" size="small">
              <el-radio-button :label="1">启用</el-radio-button>
              <el-radio-button :label="0">停用</el-radio-button>
            </el-radio-group>
          </el-form-item>
        </div>

        <el-divider></el-divider>

        <div class="edit-section">
          <el-form-item prop="policy_id">
            <span slot="label">
              策略
              <el-tooltip placement="top" content="选择该组使用的策略模版，包含带宽、DNS、路由等配置。启用状态的用户组必须指定策略。">
                <i class="el-icon-info label-tip-icon"></i>
              </el-tooltip>
            </span>
            <el-select v-model="ruleForm.policy_id" placeholder="请选择策略" style="width:200px">
              <el-option v-for="pt in policyList" :key="pt.id" :label="pt.name" :value="pt.id"></el-option>
            </el-select>
            <el-button type="text" size="small" style="margin-left:8px" @click="openPolicyCreateDialog">
              快速新建策略
            </el-button>
          </el-form-item>

          <el-form-item prop="split_dns">
            <span slot="label">
              内网域名
              <el-tooltip placement="top" content="分割DNS：留空则使用策略模版的DNS。配置后只有指定的域名（含子域名）走配置的DNS解析。">
                <i class="el-icon-info label-tip-icon"></i>
              </el-tooltip>
            </span>
            <div class="dynamic-list">
              <div v-for="(item, index) in ruleForm.split_dns" :key="index" class="dynamic-item">
                <div class="dynamic-col-val">
                  <el-input v-model="item.val" placeholder="域名" size="small"></el-input>
                </div>
                <div class="dynamic-col-note">
                  <el-input v-model="item.note" placeholder="备注" size="small"></el-input>
                </div>
                <div class="dynamic-col-ops">
                  <el-button size="mini" type="danger" icon="el-icon-delete" circle
                    @click.prevent="removeDomain(ruleForm.split_dns, index)"></el-button>
                </div>
              </div>
              <el-button size="small" type="primary" plain icon="el-icon-plus"
                @click.prevent="addDomain(ruleForm.split_dns)" class="dynamic-add-btn">
                添加域名
              </el-button>
            </div>
          </el-form-item>
        </div>

        <el-divider></el-divider>

        <!-- 组级别 IP 分配 -->
        <div class="edit-section">
          <el-form-item label="独立IP段">
            <el-switch v-model="enableGroupIP">
              <span slot="active-text">组级别分配</span>
              <span slot="inactive-text">全局 IP 池</span>
            </el-switch>
          </el-form-item>
          <transition name="ip-config-slide">
            <div v-if="enableGroupIP" class="ip-config-fields">
              <div class="ip-config-row">
                <el-form-item label="网段" prop="client_cidr" class="ip-config-col">
                  <el-input v-model="ruleForm.client_cidr" placeholder="如 10.0.1.0/24" size="small"></el-input>
                </el-form-item>
                <el-form-item label="网关地址" prop="client_gateway" class="ip-config-col">
                  <el-input v-model="ruleForm.client_gateway" placeholder="如 10.0.1.1" size="small"></el-input>
                </el-form-item>
              </div>
              <div class="ip-config-row">
                <el-form-item label="起始地址" prop="client_start" class="ip-config-col">
                  <el-input v-model="ruleForm.client_start" placeholder="如 10.0.1.100" size="small"></el-input>
                </el-form-item>
                <el-form-item label="结束地址" prop="client_end" class="ip-config-col">
                  <el-input v-model="ruleForm.client_end" placeholder="如 10.0.1.200" size="small"></el-input>
                </el-form-item>
              </div>
              <div class="ip-config-row">
                <el-form-item label="IPv6 网段" prop="client_cidr6" class="ip-config-col">
                  <el-input v-model="ruleForm.client_cidr6" placeholder="如 2001:db8:10::/64，留空用全局 v6 池"
                    size="small"></el-input>
                </el-form-item>
                <el-form-item label="出网网卡" prop="out_dev" class="ip-config-col">
                  <el-select v-model="ruleForm.out_dev" placeholder="留空=默认 master_dev，可手动输入任意网卡名（如 br0/bond0）"
                    size="small" clearable filterable allow-create default-first-option style="width:100%">
                    <el-option label="留空（使用默认 master_dev）" value=""></el-option>
                    <el-option v-for="iface in outDevOptions" :key="iface" :label="iface" :value="iface"></el-option>
                  </el-select>
                </el-form-item>
              </div>
              <div class="form-tip form-tip-info" style="margin-left:100px">
                <i class="el-icon-info"></i>
                <span>该组用户将从指定网段获取 IP，网关和掩码也对应变化，4 项必须全部填写。开启 IPv6 双栈时，可在 IPv6 网段处指定独立 v6 出网段（前缀须小于 128），留空则复用全局 v6
                  池。</span>
              </div>
            </div>
          </transition>
        </div>

        <el-divider></el-divider>

        <!-- 认证方式 -->
        <div class="edit-section">
          <el-form-item label="认证流程" prop="auth_profile">
            <div v-if="ruleForm.auth_profile.step.length === 0" class="form-tip form-tip-info"
              style="margin-bottom:10px">
              认证流程为空时将使用"本地密码"认证。按顺序组合多个认证步骤实现多因素认证。
            </div>
            <draggable v-model="ruleForm.auth_profile.step" handle=".step-drag-handle">
              <div v-for="(step, index) in ruleForm.auth_profile.step" :key="step._key" class="auth-step-item">
                <div class="step-bar">
                  <i class="el-icon-rank step-drag-handle"></i>
                  <span class="step-num">{{ index + 1 }}</span>
                  <el-select v-model="step.type" @change="onStepTypeChange(index)" size="small" class="step-type-select"
                    placeholder="认证方式">
                    <el-option label="本地密码" value="local"></el-option>
                    <el-option label="TLS证书" value="cert"></el-option>
                    <el-option label="LDAP" value="ldap"></el-option>
                    <el-option label="RADIUS" value="radius"></el-option>
                    <el-option label="OTP验证" value="otp"></el-option>
                    <el-option label="企微" value="wxwork"></el-option>
                    <el-option label="飞书" value="feishu"></el-option>
                    <el-option label="钉钉" value="dingtalk"></el-option>
                    <el-option label="短信验证" value="sms"></el-option>
                  </el-select>
                  <el-select v-if="providerTypes.includes(step.type)" v-model="step.provider" size="small"
                    class="step-provider-select" placeholder="选择认证源" clearable
                    @visible-change="(v) => v && loadProviders(step.type)">
                    <el-option v-for="p in providerNames[step.type]" :key="p" :label="p" :value="p"></el-option>
                  </el-select>
                  <el-button size="mini" type="danger" icon="el-icon-minus" circle @click="removeAuthStep(index)"
                    title="删除"></el-button>
                </div>
              </div>
            </draggable>
            <el-button size="small" type="primary" plain icon="el-icon-plus" @click="addAuthStep"
              style="margin-top:10px">添加认证步骤</el-button>
            <div v-if="ruleForm.auth_profile.step.length > 0" class="pipeline-flow">
              <span class="pipeline-label">认证流程</span>
              <span v-for="(step, index) in ruleForm.auth_profile.step" :key="'tp-' + step._key"
                style="display:contents">
                <el-tag size="small" type="success" effect="dark" class="pipeline-tag">{{ getStepLabel(step.type)
                }}</el-tag>
                <i v-if="index < ruleForm.auth_profile.step.length - 1" class="el-icon-right pipeline-arrow"></i>
              </span>
            </div>
            <div v-if="authConflictTip" class="form-tip form-tip-danger">
              <i class="el-icon-warning"></i>
              <span>{{ authConflictTip }}</span>
            </div>
            <div class="step-hint">
              <i class="el-icon-warning"></i>
              <span>认证步骤按序执行，前一步通过才进入下一步。常用组合：本地密码→OTP、证书→LDAP、RADIUS→OTP 等。</span>
            </div>
          </el-form-item>
        </div>

        <el-divider></el-divider>

        <div class="dialog-footer">
          <el-button @click="closeDialog">取消</el-button>
          <el-button type="primary" icon="el-icon-check" @click="submitForm('ruleForm')" :loading="isSubmitting"
            :disabled="isSubmitting">保存用户组</el-button>
        </div>
      </el-form>
    </el-dialog>

    <!-- 快速新建策略弹窗 -->
    <el-dialog :close-on-click-modal="false" :visible.sync="policyCreateDialog" width="760px" top="4vh"
      custom-class="policy-create-dialog" @close="policyCreateForm = getDefaultPolicyForm()">
      <div slot="title" class="dialog-title-wrap">
        <span class="dialog-title-icon"><i class="el-icon-document-add"></i></span>
        <span>快速新建策略</span>
      </div>
      <el-form :model="policyCreateForm" ref="policyCreateFormRef" label-width="100px">
        <!-- 基础信息区 -->
        <div class="pquick-section">
          <div class="pquick-section-header">
            <span class="pquick-section-dot"></span>基础信息
          </div>
          <div class="policy-quick-grid">
            <el-form-item label="策略名称" prop="name"
              :rules="[{ required: true, message: '请输入策略名称', trigger: 'blur' }, { max: 60, message: '长度不超过 60 个字符', trigger: 'blur' }]">
              <el-input v-model="policyCreateForm.name" placeholder="如：办公策略、运维策略" size="small"></el-input>
            </el-form-item>
            <el-form-item label="状态">
              <el-radio-group v-model="policyCreateForm.status" size="small">
                <el-radio-button :label="1">启用</el-radio-button>
                <el-radio-button :label="0">停用</el-radio-button>
              </el-radio-group>
            </el-form-item>
            <el-form-item label="下行带宽">
              <el-input v-model="policyCreateForm.bandwidth_format" placeholder="0 表示不限速" size="small">
                <template slot="append">Mbps</template>
              </el-input>
            </el-form-item>
            <el-form-item label="上行带宽">
              <el-input v-model="policyCreateForm.bandwidth_up_format" placeholder="0 表示不限速" size="small">
                <template slot="append">Mbps</template>
              </el-input>
            </el-form-item>
            <el-form-item label="本地网络">
              <el-switch v-model="policyCreateForm.allow_lan" active-text="允许" inactive-text="禁止"></el-switch>
              <span class="pquick-field-hint">允许访问本地局域网</span>
            </el-form-item>
            <el-form-item label="备注" class="pquick-col-full">
              <el-input v-model="policyCreateForm.note" placeholder="可选" size="small"></el-input>
            </el-form-item>
          </div>
        </div>

        <!-- DNS 区 -->
        <div class="pquick-section">
          <div class="pquick-section-header">
            <span class="pquick-section-dot pquick-section-dot--dns"></span>
            DNS 服务器
          </div>
          <div class="pquick-card">
            <div v-for="(item, i) in policyCreateForm.client_dns" :key="'dns-' + i" class="valdata-row">
              <span class="valdata-index">{{ i + 1 }}</span>
              <el-input v-model="item.val" placeholder="DNS 地址，如 114.114.114.114" size="small"
                class="valdata-input-main"></el-input>
              <el-input v-model="item.note" placeholder="备注" size="small" class="valdata-input-note"></el-input>
              <el-button type="text" icon="el-icon-delete" class="valdata-del-btn"
                @click="policyCreateForm.client_dns.splice(i, 1)"
                :disabled="policyCreateForm.client_dns.length <= 1"></el-button>
            </div>
            <div class="valdata-add-row">
              <el-button type="dashed" icon="el-icon-plus" size="small" @click="policyAddDns">添加 DNS</el-button>
            </div>
          </div>
        </div>

        <!-- 路由区 -->
        <div class="pquick-section">
          <div class="pquick-section-header">
            <span class="pquick-section-dot pquick-section-dot--route"></span>
            路由设置
          </div>
          <div class="pquick-card">
            <div class="form-tip form-tip-info" style="margin-top:0">
              <i class="el-icon-info"></i> 包含路由决定哪些流量走 VPN 隧道，"all" 表示全局代理。
            </div>
            <div class="section-label">
              <span class="section-label-icon"><i class="el-icon-upload2"></i></span>
              包含路由（走 VPN）
            </div>
            <div v-for="(item, i) in policyCreateForm.route_include" :key="'ri-' + i" class="valdata-row">
              <span class="valdata-index valdata-index--inc">{{ i + 1 }}</span>
              <el-input v-model="item.val" placeholder="CIDR 地址或 all" size="small"
                class="valdata-input-main"></el-input>
              <el-input v-model="item.note" placeholder="备注" size="small" class="valdata-input-note"></el-input>
              <el-button type="text" icon="el-icon-delete" class="valdata-del-btn"
                @click="policyCreateForm.route_include.splice(i, 1)"
                :disabled="policyCreateForm.route_include.length <= 1"></el-button>
            </div>
            <div class="valdata-add-row">
              <el-button type="dashed" icon="el-icon-plus" size="small"
                @click="policyAddRouteInclude">添加包含路由</el-button>
            </div>

            <div class="section-label" style="margin-top:18px">
              <span class="section-label-icon section-label-icon--exc"><i class="el-icon-download"></i></span>
              排除路由（不走 VPN）
            </div>
            <div v-for="(item, i) in policyCreateForm.route_exclude" :key="'re-' + i" class="valdata-row">
              <span class="valdata-index valdata-index--exc">{{ i + 1 }}</span>
              <el-input v-model="item.val" placeholder="CIDR 地址" size="small" class="valdata-input-main"></el-input>
              <el-input v-model="item.note" placeholder="备注" size="small" class="valdata-input-note"></el-input>
              <el-button type="text" icon="el-icon-delete" class="valdata-del-btn"
                @click="policyCreateForm.route_exclude.splice(i, 1)"></el-button>
            </div>
            <div class="valdata-add-row">
              <el-button type="dashed" icon="el-icon-plus" size="small"
                @click="policyAddRouteExclude">添加排除路由</el-button>
            </div>
          </div>
        </div>

        <div class="pquick-footer-hint">
          <i class="el-icon-warning-outline"></i>
          <span>ACL 访问控制<span v-if="showFakeDNS">、域名拆分隧道、FakeDNS</span>等高级功能请前往「策略管理」页面配置</span>
        </div>
      </el-form>
      <div slot="footer" class="dialog-footer-v2">
        <el-button @click="policyCreateDialog = false" size="small">取消</el-button>
        <el-button type="primary" icon="el-icon-plus" @click="createPolicy" size="small">新建并选择</el-button>
      </div>
    </el-dialog>
  </div>
</template>

<script>
import axios from "axios";
import draggable from 'vuedraggable'

const AUTH_LABELS = {
  local: '本地密码', cert: 'TLS证书', ldap: 'LDAP',
  radius: 'RADIUS', otp: 'OTP验证', wxwork: '企微', feishu: '飞书', dingtalk: '钉钉', sms: '短信验证',
};

export default {
  name: "GroupList",
  components: { draggable },
  created() {
    this.$emit('update:route_path', this.$route.path)
    this.$emit('update:route_name', ['用户组管理'])
  },
  mounted() {
    this.loadAllPolicyNames().then(() => { this.getData(1); });
    this.loadPolicyList();
    this.loadIfaces();
  },
  data() {
    return {
      loading: false,
      isSubmitting: false,
      ifaces: [],
      page: 1,
      tableData: [],
      count: 10,
      stats: {},
      policyList: [],
      allPolicyNames: [],
      stepDefaults: {
        local: {}, otp: {}, cert: {}, sms: {},
        radius: { addr: "", secret: "", nasip: "" },
        ldap: {
          addr: "", tls: false, tls_verify: false, bind_name: "", bind_pwd: "", base_dn: "",
          object_class: "person", search_attr: "sAMAccountName",
          member_of: "", sync_user_status: false, enable_otp: false
        },
        wxwork: {
          corp_id: "", agent_id: "", secret: "",
          use_default_browser: false, allowed_departments: ""
        },
        feishu: {
          app_id: "", app_secret: "",
          use_default_browser: false, allowed_departments: ""
        },
      },
      providerTypes: ['ldap', 'radius', 'wxwork', 'feishu', 'dingtalk'],
      providerNames: { ldap: [], radius: [], wxwork: [], feishu: [], dingtalk: [] },
      stepKeyCounter: 1,
      policyCreateDialog: false,
      policyCreateForm: {
        name: '', note: '', status: 1, allow_lan: true,
        bandwidth_format: '0',
        bandwidth_up_format: '0',
        client_dns: [{ val: '114.114.114.114', note: '' }],
        route_include: [{ val: 'all', note: '' }],
        route_exclude: [],
      },
      ruleForm: {
        policy_id: 0,
        status: 1,
        split_dns: [],
        auth_profile: { step: [] },
        client_cidr: '',
        client_start: '',
        client_end: '',
        client_gateway: '',
        client_cidr6: '',
        out_dev: '',
      },
      enableGroupIP: false,
      rules: {
        name: [
          { required: true, message: '请输入组名', trigger: 'blur' },
          { max: 30, message: '长度不超过 30 个字符', trigger: 'blur' }
        ],
        policy_id: [
          { type: 'number', message: '请选择策略', trigger: 'change' },
          {
            validator: (rule, value, callback) => {
              if (this.ruleForm.status === 1 && (!value || value <= 0)) {
                callback(new Error('启用的用户组必须指定策略'));
              } else {
                callback();
              }
            }, trigger: 'change'
          }
        ],
        client_cidr: [
          {
            validator: (rule, value, callback) => {
              if (!value) {
                callback();
                return;
              }
              const m = value.match(/^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})\/(\d{1,2})$/);
              if (!m) {
                callback(new Error('IPv4 网段格式无效（如 10.0.1.0/24）'));
                return;
              }
              const octets = [m[1], m[2], m[3], m[4]].map(Number);
              if (octets.some(o => o > 255)) {
                callback(new Error('IPv4 地址每段须为 0-255'));
                return;
              }
              const prefix = Number(m[5]);
              if (prefix < 0 || prefix > 32) {
                callback(new Error('IPv4 前缀须为 0-32'));
                return;
              }
              callback();
            },
            trigger: 'blur',
          },
        ],
        client_cidr6: [
          {
            validator: (rule, value, callback) => {
              if (!value) {
                callback();
                return;
              }
              if (!this.isValidCIDR6(value).valid) {
                callback(new Error('IPv6 网段格式无效（须为 CIDR，如 2001:db8:10::/64，前缀 0-127）'));
                return;
              }
              callback();
            },
            trigger: 'blur',
          },
        ],
      },
    }
  },
  computed: {
    statTotal() { return this.count },
    statActive() { return this.stats.stats_active || 0 },
    statWithPolicy() { return this.stats.stats_policy || 0 },
    statWithAuth() { return this.stats.stats_multi_auth || 0 },
    // SSO 与凭据步骤(local/ldap/radius)互斥：SSO 由第三方身份提供，不产生登录密码
    authConflictTip() {
      const types = (this.ruleForm.auth_profile.step || []).map(s => s.type)
      const hasSSO = types.some(t => t === 'wxwork' || t === 'feishu')
      const hasCred = types.some(t => t === 'local' || t === 'ldap' || t === 'radius')
      if (hasSSO && hasCred) {
        return 'SSO 认证（企业微信/飞书）不能与本地密码、LDAP、RADIUS 等凭据认证同时使用：SSO 由第三方身份提供，不产生登录密码，无法与需要密码的步骤组合。如需二次验证请改用 OTP。'
      }
      return ''
    },
    // 出网网卡下拉选项：物理网卡清单 + 已保存但当前不在清单中的网卡(改名/移除后仍能显示)
    outDevOptions() {
      const list = [...this.ifaces]
      if (this.ruleForm.out_dev && !list.includes(this.ruleForm.out_dev)) {
        list.unshift(this.ruleForm.out_dev)
      }
      return list
    },
  },
  watch: {
    enableGroupIP(val) {
      if (!val) {
        this.ruleForm.client_cidr = '';
        this.ruleForm.client_start = '';
        this.ruleForm.client_end = '';
        this.ruleForm.client_gateway = '';
        this.ruleForm.client_cidr6 = '';
        this.ruleForm.out_dev = '';
      }
    },
  },
  methods: {
    getStepLabel(type) { return AUTH_LABELS[type] || type; },
    loadIfaces() {
      axios.get('/group/ifaces').then(resp => {
        this.ifaces = (resp.data.data && resp.data.data.ifaces) || [];
      }).catch(() => { });
    },
    // IPv6 CIDR 校验：形如 2001:db8::/64，前缀 0-128；支持 :: 缩写与末段内嵌 IPv4
    isValidCIDR6(input) {
      const parts = (input || '').split('/');
      if (parts.length !== 2) return { valid: false };
      const prefix = parseInt(parts[1], 10);
      if (isNaN(prefix) || prefix < 0 || prefix > 127 || !/^\d+$/.test(parts[1])) {
        return { valid: false };
      }
      const addr = parts[0];
      const dbl = addr.split('::');
      if (dbl.length > 2) return { valid: false };
      const isHextet = h => /^[0-9a-fA-F]{1,4}$/.test(h);
      const isV4 = s => /^((25[0-5]|2[0-4]\d|[01]?\d\d?)\.){3}(25[0-5]|2[0-4]\d|[01]?\d\d?)$/.test(s);
      const countGroups = seg => {
        if (seg === '') return 0;
        const gs = seg.split(':');
        let n = 0;
        for (let i = 0; i < gs.length; i++) {
          const g = gs[i];
          if (g === '') return -1;
          if (i === gs.length - 1 && isV4(g)) { n += 2; continue; }
          if (!isHextet(g)) return -1;
          n += 1;
        }
        return n;
      };
      if (dbl.length === 2) {
        const left = countGroups(dbl[0]);
        const right = countGroups(dbl[1]);
        if (left < 0 || right < 0) return { valid: false };
        if (left + right > 7) return { valid: false };
      } else {
        const n = countGroups(dbl[0]);
        if (n < 0 || n !== 8) return { valid: false };
      }
      return { valid: true };
    },
    loadPolicyList() {
      return axios.get('/policy/names').then(resp => {
        if (resp.data.code === 0) {
          this.policyList = resp.data.data.datas || [];
        }
      }).catch(() => { });
    },
    loadAllPolicyNames() {
      return axios.get('/policy/all_names').then(resp => {
        if (resp.data.code === 0) {
          this.allPolicyNames = resp.data.data.datas || [];
        }
      }).catch(() => { });
    },
    getDefaultPolicyForm() {
      return {
        name: '', note: '', status: 1, allow_lan: true,
        bandwidth_format: '0',
        bandwidth_up_format: '0',
        client_dns: [{ val: '114.114.114.114', note: '' }],
        route_include: [{ val: 'all', note: '' }],
        route_exclude: [],
      };
    },
    openPolicyCreateDialog() {
      this.policyCreateForm = this.getDefaultPolicyForm();
      this.policyCreateDialog = true;
      this.$nextTick(() => {
        if (this.$refs.policyCreateFormRef) this.$refs.policyCreateFormRef.clearValidate();
      });
    },
    policyAddDns() {
      this.policyCreateForm.client_dns.push({ val: '', note: '' });
    },
    policyAddRouteInclude() {
      this.policyCreateForm.route_include.push({ val: '', note: '' });
    },
    policyAddRouteExclude() {
      this.policyCreateForm.route_exclude.push({ val: '', note: '' });
    },
    createPolicy() {
      this.$refs.policyCreateFormRef.validate(valid => {
        if (!valid) return;
        const f = this.policyCreateForm;
        const bw = parseFloat(f.bandwidth_format) || 0;
        const bwUp = parseFloat(f.bandwidth_up_format) || 0;
        const data = {
          name: f.name,
          note: f.note,
          status: f.status,
          allow_lan: f.allow_lan,
          bandwidth: bw > 0 ? Math.round(bw * 125000) : 0, // Mbps → Byte/s (1 Mbps = 125000 Byte/s)
          bandwidth_up: bwUp > 0 ? Math.round(bwUp * 125000) : 0,
          bandwidth_format: f.bandwidth_format,
          bandwidth_up_format: f.bandwidth_up_format,
          client_dns: f.client_dns.filter(d => d.val),
          route_include: f.route_include.filter(r => r.val),
          route_exclude: f.route_exclude.filter(r => r.val),
        };
        axios.post('/policy/set', data).then(resp => {
          if (resp.data.code === 0) {
            this.$message.success('策略创建成功, 已自动选中');
            this.policyCreateDialog = false;
            // 刷新策略下拉列表并自动选中新策略
            this.loadPolicyList().then(() => {
              if (this.policyList.length > 0) {
                const latest = this.policyList.reduce((a, b) => a.id > b.id ? a : b);
                this.ruleForm.policy_id = latest.id;
              }
            });
          } else {
            this.$message.error(resp.data.msg || '策略创建失败');
          }
        }).catch(() => {
          this.$message.error('策略创建失败');
        });
      });
    },
    handleRowCmd(row, cmd) {
      switch (cmd) {
        case 'edit': this.handleEdit(row); break;
        case 'delete':
          this.$confirm('确定要删除该用户组吗？删除后不可恢复。', '删除确认', {
            confirmButtonText: '确定删除',
            cancelButtonText: '取消',
            type: 'warning',
            confirmButtonClass: 'el-button--danger',
          }).then(() => this.handleDel(row)).catch(() => { });
          break;
      }
    },
    handleDel(row) {
      axios.post('/group/del?id=' + row.id).then(resp => {
        if (resp.data.code === 0) {
          this.$message.success(resp.data.msg);
          this.getData(1);
        } else {
          this.$message.error(resp.data.msg);
        }
      }).catch(() => {
        this.$message.error('请求出错');
      });
    },
    handleEdit(row) {
      this.$refs['ruleForm'] && this.$refs['ruleForm'].resetFields();
      this.user_edit_dialog = true;
      if (!row) {
        this.ruleForm = {
          id: undefined, name: '', note: '',
          policy_id: 0, status: 1, split_dns: [],
          auth_profile: { step: [] },
          client_cidr: '', client_start: '', client_end: '', client_gateway: '',
          client_cidr6: '',
          out_dev: '',
        };
        this.enableGroupIP = false;
        this.setAuthData(null);
        return;
      }
      axios.get('/group/detail', { params: { id: row.id } }).then(resp => {
        const d = resp.data.data;
        this.ruleForm = {
          id: d.id, name: d.name, note: d.note,
          policy_id: d.policy_id || 0, status: d.status,
          split_dns: d.split_dns || [],
          auth_profile: { step: [] },
          client_cidr: d.client_cidr || '',
          client_start: d.client_start || '',
          client_end: d.client_end || '',
          client_gateway: d.client_gateway || '',
          client_cidr6: d.client_cidr6 || '',
          out_dev: d.out_dev || '',
        };
        this.enableGroupIP = !!(d.client_cidr && d.client_start && d.client_end && d.client_gateway);
        this.setAuthData(d);
        this.preloadProviders();
      }).catch(() => {
        this.$message.error('请求出错');
      });
    },
    setAuthData(row) {
      if (!row || !row.auth_profile || !row.auth_profile.step || !row.auth_profile.step.length) {
        this.ruleForm.auth_profile = { step: [this._newStep('local')] };
        return;
      }
      this.ruleForm.auth_profile = {
        step: row.auth_profile.step.map(s => this._normalizeStep(s)),
      };
    },
    _normalizeStep(s) {
      const defCfg = this.stepDefaults[s.type] || {};
      return {
        _key: 's' + (this.stepKeyCounter++),
        type: s.type || 'local',
        provider: s.provider || '',
        config: Object.assign({}, defCfg, s.config || {}),
      };
    },
    _newStep(type) {
      return {
        _key: 's' + (this.stepKeyCounter++),
        type, provider: '',
        config: Object.assign({}, this.stepDefaults[type] || {}),
      };
    },
    pageChange(p) { this.getData(p); },
    getData(page) {
      this.page = page;
      this.loading = true;
      axios.get('/group/list', { params: { page } }).then(resp => {
        const rdata = resp.data.data;
        this.tableData = (rdata.datas || []).map(g => {
          // 补充策略名称（用全量列表展示包括停用策略）和认证步骤显示
          let policyName = '';
          if (g.policy_id === 0) {
            policyName = '待配置';
          } else {
            const p = this.allPolicyNames.find(pt => pt.id === g.policy_id);
            policyName = p ? p.name : '';
          }
          const authSteps = (g.auth_profile && g.auth_profile.step) ?
            g.auth_profile.step.map(s => AUTH_LABELS[s.type] || s.type) : [];
          return { ...g, policy_name: policyName, authSteps };
        });
        this.count = rdata.count;
        this.stats = rdata;
        this.loading = false;
      }).catch(() => {
        this.$message.error('请求出错');
        this.loading = false;
      });
    },
    removeDomain(arr, idx) { if (idx >= 0 && idx < arr.length) arr.splice(idx, 1); },
    addDomain(arr) { arr.push({ val: "", note: "" }); },
    submitForm(formName) {
      if (this.isSubmitting) return;
      this.$refs[formName].validate((valid) => {
        if (!valid) return;
        if (this.authConflictTip) {
          this.$message.error(this.authConflictTip);
          return;
        }
        this.isSubmitting = true;
        axios.post('/group/set', this.ruleForm).then(resp => {
          if (resp.data.code === 0) {
            this.$message.success(resp.data.msg);
            this.getData(1);
            this.user_edit_dialog = false;
          } else {
            this.$message.error(resp.data.msg);
          }
        }).catch(() => {
          this.$message.error('请求出错');
        }).finally(() => {
          this.isSubmitting = false;
        });
      });
    },
    addAuthStep() { this.ruleForm.auth_profile.step.push(this._newStep('local')); },
    removeAuthStep(index) {
      if (this.ruleForm.auth_profile.step.length <= 1) {
        this.$message.warning('至少保留一个认证步骤');
        return;
      }
      this.ruleForm.auth_profile.step.splice(index, 1);
    },
    loadProviders(typ) {
      if (typ && this.providerTypes.includes(typ) && this.providerNames[typ].length === 0) {
        axios.get('/provider/names', { params: { type: typ } }).then(resp => {
          if (resp.data.code === 0) {
            this.$set(this.providerNames, typ, resp.data.data.datas || []);
          }
        }).catch(() => { });
      }
    },
    preloadProviders() {
      const types = new Set();
      (this.ruleForm.auth_profile.step || []).forEach(s => {
        if (this.providerTypes.includes(s.type)) types.add(s.type);
      });
      types.forEach(t => this.loadProviders(t));
    },
    onStepTypeChange(index) {
      const step = this.ruleForm.auth_profile.step[index];
      if (!step) return;
      step.config = Object.assign({}, this.stepDefaults[step.type] || {});
      step.provider = '';
      this.$refs['ruleForm'].clearValidate();
      this.loadProviders(step.type);
      // 选择 TLS 证书认证时，检查该组是否有已签发的客户端证书
      if (step.type === 'cert' && this.ruleForm.name) {
        this.checkGroupCerts();
      }
    },
    checkGroupCerts() {
      axios.get('/group/cert_check', { params: { groupname: this.ruleForm.name } }).then(resp => {
        if (resp.data.code === 0 && resp.data.data.cert_count === 0) {
          this.$message.warning(`当前组"${this.ruleForm.name}"尚未签发任何客户端证书，建议先在"系统设置 > 证书设置 > 客户端证书"中生成证书后再启用证书认证`);
        }
      }).catch(() => { });
    },
    closeDialog() { this.user_edit_dialog = false; },
  },
}
</script>

<style scoped>
/* ========== 页面整体 ========== */
.group-page {
  padding: 4px 0;
}

/* ========== 统计卡片 ========== */
.stat-icon-total {
  background: var(--color-primary-bg);
  color: var(--color-primary);
}

.stat-icon-active {
  background: var(--success-bg);
  color: var(--color-success);
}

.stat-icon-policy {
  background: var(--warning-bg);
  color: var(--color-warning);
}

.stat-icon-auth {
  background: var(--danger-bg);
  color: var(--color-danger);
}

/* 表格滚动容器 */
.group-table-wrap {
  overflow-x: auto;
  width: 100%;
}

/* 表格内 */
.group-name {
  font-weight: 600;
  color: var(--text-primary);
  font-size: 13px;
  display: block;
}

.group-note {
  font-size: 12px;
  color: var(--text-secondary);
}

.text-muted {
  color: var(--text-placeholder);
  font-size: 12px;
}

.auth-flow-mini {
  display: flex;
  align-items: center;
  justify-content: center;
  flex-wrap: wrap;
  gap: 2px;
}

/* IP 配置展开区域 */
.ip-config-fields {
  padding-left: 0;
  margin-top: -4px;
}

/* label 旁的说明图标 */
.label-tip-icon {
  color: var(--text-placeholder);
  font-size: 13px;
  margin-left: 2px;
  cursor: help;
}

.label-tip-icon:hover {
  color: var(--text-secondary);
}

.ip-config-row {
  display: flex;
  gap: 20px;
}

.ip-config-col {
  flex: 1;
  min-width: 0;
}

.ip-config-fields .el-form-item {
  margin-bottom: 14px;
}

/* 展开过渡动画 */
.ip-config-slide-enter-active {
  transition: opacity 0.25s ease, transform 0.25s ease;
}

.ip-config-slide-leave-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}

.ip-config-slide-enter,
.ip-config-slide-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}

.auth-arrow {
  font-size: 10px;
  color: var(--color-success);
  margin: 0 2px;
}

.status-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 5px;
  vertical-align: middle;
}

.status-dot-online {
  background: var(--color-success);
  box-shadow: 0 0 0 2px rgba(103, 194, 58, 0.2);
}

.status-dot-offline {
  background: var(--color-danger);
  box-shadow: 0 0 0 2px rgba(245, 108, 108, 0.2);
}

.status-text {
  font-size: 12px;
  vertical-align: middle;
}

.text-success {
  color: var(--color-success);
  font-weight: 500;
}

.text-danger {
  color: var(--color-danger);
  font-weight: 500;
}

.action-more-btn {
  padding: 5px 10px;
  border-radius: 6px;
  font-size: 12px;
  border: 1px solid var(--border-base);
  background: var(--bg-card);
  color: var(--text-regular);
  transition: all 0.2s;
}

.action-more-btn:hover {
  color: var(--color-primary);
  border-color: var(--color-primary-light);
  background: var(--color-primary-bg);
}

.dropdown-danger {
  color: var(--color-danger) !important;
}

.pagination-wrap {
  display: flex;
  justify-content: flex-end;
  padding-top: 16px;
}

/* ========== 编辑弹窗 ========== */
.group-edit-dialog ::v-deep .el-dialog__body {
  padding: 20px 30px 10px;
}

.edit-basic-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 30px;
}

.edit-basic-row .form-item-compact {
  margin-bottom: 16px;
}

.form-tip {
  font-size: 12px;
  color: var(--text-secondary);
  margin-left: 8px;
}

.form-tip-info {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 8px 0 4px;
  padding: 6px 10px;
  background: var(--info-bg);
  border-radius: 6px;
  font-size: 12px;
  color: var(--text-secondary);
}

.form-tip-danger {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 8px 0 4px;
  padding: 6px 10px;
  background: var(--danger-bg);
  border-radius: 6px;
  border: 1px solid var(--danger-bg);
  font-size: 12px;
  color: var(--color-danger);
}

.form-tip-danger i {
  color: var(--color-danger);
}

.form-tip-info span {
  min-width: 0;
  word-break: break-all;
  overflow-wrap: break-word;
}

.form-tip-info i {
  color: var(--text-secondary);
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding-top: 8px;
}

/* ========== 动态列表 ========== */
.dynamic-list {
  width: 100%;
}

.dynamic-item {
  display: flex;
  gap: 10px;
  align-items: center;
  margin-bottom: 8px;
  padding: 6px 8px;
  border-radius: 6px;
  background: var(--bg-header);
  border: 1px solid transparent;
  transition: all 0.15s;
}

.dynamic-item:hover {
  background: var(--color-primary-bg);
  border-color: var(--color-primary-light);
}

.dynamic-col-val {
  flex: 1;
}

.dynamic-col-note {
  flex: 1;
}

.dynamic-col-ops {
  width: 32px;
  flex-shrink: 0;
  display: flex;
  justify-content: center;
}

.dynamic-add-btn {
  margin-top: 4px;
}

/* ========== 认证步骤 ========== */
.auth-step-item {
  margin-bottom: 8px;
}

.step-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  background: var(--bg-hover);
  border: 1px solid var(--border-color);
  border-radius: 6px;
  transition: border-color 0.2s;
}

.step-bar:hover {
  border-color: var(--color-primary);
}

.step-drag-handle {
  cursor: move;
  color: var(--text-placeholder);
  font-size: 16px;
}

.step-drag-handle:hover {
  color: var(--color-primary);
}

.step-num {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--color-primary);
  color: var(--text-inverse);
  font-size: 12px;
  font-weight: 500;
  flex-shrink: 0;
}

.step-type-select {
  flex: 0 1 auto;
  min-width: 100px;
  max-width: 150px;
}

.step-provider-select {
  flex: 1 1 auto;
  max-width: 240px;
}

.pipeline-flow {
  margin-top: 12px;
  padding: 10px 12px;
  background: var(--success-bg);
  border: 1px solid #c2e7b0;
  border-radius: 6px;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
}

.pipeline-label {
  color: var(--color-success);
  font-weight: 500;
  margin-right: 10px;
  font-size: 13px;
}

.pipeline-tag {
  border-radius: 10px;
  padding: 0 10px;
}

.pipeline-arrow {
  margin: 0 6px;
  color: var(--color-success);
  font-weight: bold;
}

.step-hint {
  background: var(--warning-bg);
  border: 1px solid #f5dab1;
  border-radius: 6px;
  padding: 8px 14px;
  margin-top: 12px;
  font-size: 12px;
  color: var(--color-warning);
  display: flex;
  align-items: center;
  gap: 8px;
}

.step-hint i {
  color: var(--color-warning);
  font-size: 15px;
  flex-shrink: 0;
}

/* 响应式 */
@media (max-width: 768px) {
  .edit-basic-row {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 880px) {
  .group-table-wrap ::v-deep .col-ops {
    min-width: 180px;
  }
}

@media (max-width: 600px) {
  .group-table-wrap ::v-deep .col-ops {
    min-width: 140px;
  }
}

/* ========== 快速新建策略弹窗 ========== */
.dialog-title-wrap {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 16px;
  font-weight: 600;
}

.dialog-title-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: linear-gradient(135deg, var(--color-primary), #337ecc);
  color: var(--text-inverse);
  font-size: 16px;
}

/* --- 分区容器 --- */
.pquick-section {
  margin-bottom: 16px;
}

.pquick-section-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 10px;
}

.pquick-section-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--color-primary);
  flex-shrink: 0;
}

.pquick-section-dot--dns {
  background: var(--color-success);
}

.pquick-section-dot--route {
  background: var(--color-warning);
}

/* --- 卡片容器 --- */
.pquick-card {
  background: var(--bg-card);
  border: 1px solid var(--border-color-light);
  border-radius: 10px;
  padding: 14px 16px 10px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

/* --- 基础信息网格 --- */
.policy-quick-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 6px 28px;
  background: var(--bg-card);
  border: 1px solid var(--border-color-light);
  border-radius: 10px;
  padding: 14px 18px 6px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

.pquick-col-full {
  grid-column: 1 / -1;
}

.pquick-field-hint {
  font-size: 12px;
  color: var(--text-secondary);
  margin-left: 10px;
  vertical-align: middle;
  display: inline-flex;
  align-items: center;
}

/* --- 区块标签 --- */
.section-label {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 600;
  color: var(--color-primary);
  margin-bottom: 8px;
  padding: 6px 10px;
  border-radius: 6px;
  background: var(--color-primary-bg);
  border: 1px solid var(--color-primary-light);
}

.section-label-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 4px;
  background: var(--color-primary);
  color: var(--text-inverse);
  font-size: 11px;
}

.section-label-icon--exc {
  background: var(--color-danger);
}

/* --- 动态列表行 --- */
.valdata-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
  padding: 4px 6px 4px 4px;
  border-radius: 8px;
  transition: background 0.15s, box-shadow 0.15s;
}

.valdata-row:hover {
  background: var(--bg-hover);
  box-shadow: 0 0 0 1px var(--color-primary-bg) inset;
}

.valdata-index {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 6px;
  background: var(--bg-hover);
  color: var(--text-secondary);
  font-size: 11px;
  font-weight: 600;
  flex-shrink: 0;
}

.valdata-index--inc {
  background: var(--color-primary-bg);
  color: var(--color-primary);
}

.valdata-index--exc {
  background: var(--danger-bg);
  color: var(--color-danger);
}

.valdata-input-main {
  flex: 1.5;
}

.valdata-input-note {
  flex: 1;
}

.valdata-del-btn {
  flex-shrink: 0;
  width: 28px;
  height: 28px;
  padding: 0;
  color: var(--text-placeholder);
  font-size: 15px;
  border-radius: 6px;
  transition: all 0.15s;
}

.valdata-del-btn:hover {
  color: var(--color-danger);
  background: var(--danger-bg);
}

.valdata-del-btn.is-disabled {
  color: var(--border-color);
}

/* --- 添加按钮行 --- */
.valdata-add-row {
  padding: 6px 0 2px;
}

.valdata-add-row .el-button--dashed {
  border-style: dashed;
  color: var(--text-secondary);
  font-size: 12px;
  border-color: var(--border-base);
  transition: all 0.2s;
}

.valdata-add-row .el-button--dashed:hover {
  color: var(--color-primary);
  border-color: var(--color-primary);
  background: var(--color-primary-bg);
}

/* --- 底部提示 --- */
.pquick-footer-hint {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 4px;
  padding: 8px 12px;
  background: var(--bg-hover);
  border-radius: 8px;
  font-size: 12px;
  color: var(--text-secondary);
}

.pquick-footer-hint i {
  color: var(--color-warning);
  font-size: 14px;
}

.pquick-footer-hint span {
  min-width: 0;
  word-break: break-all;
  overflow-wrap: break-word;
}

/* --- 弹窗 footer --- */
.dialog-footer-v2 {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 8px;
}

/* 全局弹窗微调 */
::v-deep .policy-create-dialog {
  border-radius: 12px;
  overflow: hidden;
}

::v-deep .policy-create-dialog .el-dialog__header {
  padding: 16px 20px;
  background: var(--bg-header);
  border-bottom: 1px solid var(--border-color-light);
}

::v-deep .policy-create-dialog .el-dialog__body {
  padding: 18px 20px 12px;
}

::v-deep .policy-create-dialog .el-dialog__footer {
  padding: 0 20px 16px;
}

/* 响应式：对话框及表单提示 */
@media (max-width: 900px) {
  .edit-basic-row {
    grid-template-columns: 1fr;
  }

  .form-tip-info {
    word-break: break-all;
    overflow-wrap: break-word;
    max-width: 100%;
    box-sizing: border-box;
  }

  /* 快速新建策略弹窗 */
  .policy-quick-grid {
    grid-template-columns: 1fr;
  }

  .valdata-row {
    flex-wrap: wrap;
  }

  .valdata-input-main,
  .valdata-input-note {
    flex: 1 1 120px;
    min-width: 100px;
  }
}

@media (max-width: 640px) {
  .group-edit-dialog ::v-deep .el-form-item {
    display: block;
  }

  .group-edit-dialog ::v-deep .el-form-item__label {
    width: auto !important;
    text-align: left;
    padding-bottom: 4px;
    line-height: 1.4;
  }

  .group-edit-dialog ::v-deep .el-form-item__content {
    margin-left: 0 !important;
    display: block;
  }

  .form-tip-info {
    font-size: 11px;
    padding: 4px 8px;
  }

  .auth-step-item {
    flex-wrap: wrap;
  }

  .auth-step-item .step-type-select {
    flex: 1 1 120px;
  }
}
</style>
