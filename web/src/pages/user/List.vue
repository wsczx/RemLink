<template>
  <div class="user-page">
    <!-- 统计卡片 -->
    <div class="stats-row">
      <div class="stat-card">
        <div class="stat-icon stat-icon-total">
          <i class="el-icon-s-custom"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ statTotal }}</div>
          <div class="stat-label">用户总数</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-icon-local">
          <i class="el-icon-user-solid"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ statLocal }}</div>
          <div class="stat-label">本地用户</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-icon-external">
          <i class="el-icon-connection"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ statExternal }}</div>
          <div class="stat-label">外部用户</div>
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
    </div>

    <!-- 用户表格 -->
    <el-card class="table-card" shadow="never" v-loading="loading">
      <div slot="header" class="card-header">
        <span class="card-title"><i class="el-icon-s-custom"></i> 用户列表</span>
        <div class="card-actions">
          <el-input v-model="searchData" placeholder="用户名/姓名/邮箱/手机号/类型" size="small" prefix-icon="el-icon-search"
            clearable class="search-input" @keydown.enter.native="handleSearch" @clear="handleSearch" />
          <el-select v-model="filterType" placeholder="类型" size="small" clearable class="filter-select"
            @change="handleSearch">
            <el-option label="本地" value="local"></el-option>
            <el-option label="LDAP" value="ldap"></el-option>
            <el-option label="RADIUS" value="radius"></el-option>
            <el-option label="企微" value="wxwork"></el-option>
            <el-option label="飞书" value="feishu"></el-option>
            <el-option label="钉钉" value="dingtalk"></el-option>
          </el-select>
          <el-select v-model="filterGroup" placeholder="用户组" size="small" clearable class="filter-select" filterable
            @change="handleSearch">
            <el-option v-for="g in groupNames" :key="g" :label="g" :value="g"></el-option>
          </el-select>
          <el-select v-model="filterStatus" placeholder="状态" size="small" clearable class="filter-select"
            @change="handleSearch">
            <el-option label="启用" value="1"></el-option>
            <el-option label="停用" value="0"></el-option>
          </el-select>
          <el-upload accept=".xlsx,.xls" :http-request="upLoadUser" :limit="1" :show-file-list="false"
            class="inline-upload">
            <el-button size="small" icon="el-icon-upload2">批量导入</el-button>
          </el-upload>
          <el-button size="small" icon="el-icon-download" @click="downloadTemplate">下载模版</el-button>
          <el-dropdown size="small" :disabled="multipleSelection.length === 0" @command="handleBatchCmd">
            <el-button size="small" :disabled="multipleSelection.length === 0">
              批量操作<i class="el-icon-arrow-down el-icon--right"></i>
            </el-button>
            <el-dropdown-menu slot="dropdown">
              <el-dropdown-item command="email" icon="el-icon-message">批量邮件</el-dropdown-item>
              <el-dropdown-item command="delete" icon="el-icon-delete" divided>批量删除</el-dropdown-item>
            </el-dropdown-menu>
          </el-dropdown>
          <el-button size="small" type="primary" icon="el-icon-plus" @click="handleEdit('')">
            添加用户
          </el-button>
        </div>
      </div>

      <div class="user-table-wrap">
        <el-table ref="multipleTable" :data="tableData" stripe highlight-current-row border style="width:100%"
          :header-cell-style="{ background: 'var(--bg-header)', color: 'var(--text-primary)', fontWeight: '600', fontSize: '13px' }"
          @selection-change="handleSelectionChange">
          <el-table-column type="selection" width="50" align="center"></el-table-column>
          <el-table-column sortable prop="id" label="ID" width="65" align="center"></el-table-column>
          <el-table-column prop="type" label="类型" width="75" align="center">
            <template slot-scope="scope">
              <el-tag v-if="scope.row.type === 'local'" type="success" size="small" effect="plain">本地</el-tag>
              <el-tag v-else-if="scope.row.type === 'ldap'" type="warning" size="small" effect="plain">LDAP</el-tag>
              <el-tag v-else-if="scope.row.type === 'radius'" type="info" size="small" effect="plain">RADIUS</el-tag>
              <el-tag v-else-if="scope.row.type === 'wxwork'" size="small" effect="plain">企微</el-tag>
              <el-tag v-else-if="scope.row.type === 'feishu'" size="small" effect="plain">飞书</el-tag>
              <el-tag v-else-if="scope.row.type === 'dingtalk'" size="small" effect="plain">钉钉</el-tag>
              <el-tag v-else size="small" effect="plain">{{ scope.row.type }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="username" label="用户名" min-width="120" show-overflow-tooltip>
            <template slot-scope="scope">
              <span class="user-name">{{ userLabel(scope.row.username, scope.row.nickname) }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="email" label="邮箱" min-width="160" show-overflow-tooltip></el-table-column>
          <el-table-column prop="phone" label="手机号" width="140" show-overflow-tooltip></el-table-column>
          <el-table-column prop="otp_secret" label="OTP" width="110" align="center">
            <template slot-scope="scope">
              <el-button v-if="!scope.row.disable_otp && scope.row.otp_secret" type="text" size="mini"
                @click="getOtpImg(scope.row)">
                <i class="el-icon-view"></i>
                {{ scope.row.otp_secret.substring(0, 6) }}
              </el-button>
              <el-tag v-else-if="scope.row.disable_otp" type="info" size="mini">已禁用</el-tag>
              <span v-else class="text-muted">-</span>
            </template>
          </el-table-column>
          <el-table-column prop="groups" label="用户组" min-width="150" show-overflow-tooltip>
            <template slot-scope="scope">
              <div v-if="scope.row.groups && scope.row.groups.length" class="group-tags">
                <el-tag v-for="g in scope.row.groups" :key="g" size="mini" effect="plain" class="group-tag">{{ g
                }}</el-tag>
              </div>
              <span v-else class="text-muted">-</span>
            </template>
          </el-table-column>
          <el-table-column prop="status" label="状态" width="95" align="center">
            <template slot-scope="scope">
              <el-tag :type="getStatusTagType(scope.row)" :class="getStatusTagClass(scope.row)" size="small"
                disable-transitions>
                {{ getStatusLabel(scope.row) }}
              </el-tag>
              <el-tag v-if="scope.row.change_pwd" type="warning" size="mini" class="force-pwd-tag"
                disable-transitions>需改密</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="已用流量" width="120" align="center">
            <template slot-scope="scope">
              <span v-if="scope.row.traffic_used > 0" class="traffic-used">{{ formatTraffic(scope.row.traffic_used)
              }}</span>
              <span v-else class="text-muted">-</span>
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
                  <el-dropdown-item command="edit" icon="el-icon-edit">编辑用户</el-dropdown-item>
                  <el-dropdown-item command="otp" icon="el-icon-view"
                    v-if="!scope.row.disable_otp">查看OTP</el-dropdown-item>
                  <el-dropdown-item command="resetTraffic" icon="el-icon-refresh-right">重置流量配额</el-dropdown-item>
                  <el-dropdown-item command="delete" icon="el-icon-delete" divided
                    class="dropdown-danger">删除用户</el-dropdown-item>
                </el-dropdown-menu>
              </el-dropdown>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <div class="pagination-wrap">
        <el-pagination background layout="total,sizes,prev,pager,next,jumper" :pager-count="9" :current-page="page"
          :page-sizes="[10, 20, 50, 100, 200, 500]" :page-size="pageSize" :total="count" @size-change="handleSizeChange"
          @current-change="pageChange" />
      </div>
    </el-card>

    <!-- OTP 密钥弹窗 -->
    <el-dialog title="OTP 密钥" :visible.sync="otpImgData.visible" width="400px" center top="5vh">
      <div class="otp-dialog-content">
        <div class="otp-user-info">{{ otpImgData.username }} - {{ otpImgData.nickname }}</div>
        <img :src="otpImgData.base64Img" alt="otp-qr" class="otp-qr-img" />
        <div class="otp-tip">使用 Google Authenticator 或类似应用扫描二维码</div>
      </div>
    </el-dialog>

    <!-- 编辑弹窗 -->
    <el-dialog :close-on-click-modal="false" :title="ruleForm.id ? '编辑用户' : '新增用户'" :visible.sync="user_edit_dialog"
      width="750px" top="4vh" @close="disVisible" class="user-edit-dialog">
      <el-form :model="ruleForm" :rules="rules" ref="ruleForm" label-width="100px">
        <div class="section-label"><i class="el-icon-user"></i> 基本信息</div>
        <div class="edit-basic-row">
          <el-form-item label="用户ID" prop="id" v-if="ruleForm.id" class="form-item-compact">
            <el-input v-model="ruleForm.id" disabled size="small"></el-input>
          </el-form-item>
          <el-form-item label="类型" prop="type" class="form-item-compact">
            <el-input v-model="ruleForm.type" disabled size="small"></el-input>
          </el-form-item>
          <el-form-item label="用户名" prop="username" class="form-item-compact">
            <el-input v-model="ruleForm.username" :disabled="ruleForm.id > 0" placeholder="请输入用户名"
              size="small"></el-input>
          </el-form-item>
          <el-form-item label="密码" prop="pin_code" class="form-item-compact">
            <el-input v-model="ruleForm.pin_code" :disabled="isExternalUser" placeholder="留空由系统自动生成"
              size="small"></el-input>
          </el-form-item>
          <el-form-item label="姓名" prop="nickname" class="form-item-compact">
            <el-input v-model="ruleForm.nickname" :disabled="isExternalUser" placeholder="请输入姓名"
              size="small"></el-input>
          </el-form-item>
          <el-form-item label="邮箱" prop="email" class="form-item-compact">
            <el-input v-model="ruleForm.email" placeholder="请输入邮箱" size="small"></el-input>
          </el-form-item>
          <el-form-item label="手机号" prop="phone" class="form-item-compact">
            <el-input v-model="ruleForm.phone" placeholder="用于短信接收OTP验证码" size="small"></el-input>
          </el-form-item>
        </div>

        <div class="section-label"><i class="el-icon-setting"></i> 高级配置</div>
        <div class="edit-adv-section">
          <!-- MTU -->
          <div class="edit-adv-row">
            <el-form-item label="MTU" prop="mtu" class="form-item-compact">
              <el-input-number v-model="ruleForm.mtu" :min="0" :max="1500" size="small"></el-input-number>
              <el-tooltip content="设为 0 则使用全局默认值" placement="top">
                <i class="el-icon-question help-icon"></i>
              </el-tooltip>
            </el-form-item>
          </div>
          <!-- OTP 独占一行，秘钥框拉长对齐 -->
          <el-form-item label="OTP验证" class="form-item-compact form-item-full form-item-otp">
            <div class="otp-row">
              <el-switch v-model="ruleForm.disable_otp" :active-value="false" :inactive-value="true"
                active-color="#13ce66" inactive-color="#909399" />
              <el-input v-if="!ruleForm.disable_otp" v-model="ruleForm.otp_secret" placeholder="留空由系统自动生成" size="small"
                class="otp-secret-input" />
              <el-tooltip content="开启OTP后用户密码为密码+动态码双因素认证" placement="top">
                <i class="el-icon-question help-icon"></i>
              </el-tooltip>
            </div>
          </el-form-item>
          <!-- 下次登录强制修改密码（仅本地用户生效） -->
          <el-form-item label="强制改密" class="form-item-compact form-item-full">
            <div class="otp-row">
              <el-switch v-model="ruleForm.change_pwd" :disabled="isExternalUser" active-color="#e6a23c"
                inactive-color="#909399" />
              <el-tooltip content="开启后该用户下次登录时必须先修改密码（仅本地用户生效）" placement="top">
                <i class="el-icon-question help-icon"></i>
              </el-tooltip>
              <span v-if="ruleForm.change_pwd" class="force-pwd-tip">下次登录强制修改密码</span>
              <span v-else-if="isExternalUser" class="text-muted">外部用户无需本地密码</span>
            </div>
          </el-form-item>
          <!-- 过期时间 + 发送邮件 一行 -->
          <div class="edit-adv-row">
            <el-form-item label="过期时间" prop="limittime" class="form-item-compact">
              <el-date-picker v-model="ruleForm.limittime" type="datetime" size="small" format="yyyy-MM-dd HH:mm"
                placeholder="选择日期时间" :picker-options="pickerOptions" :disabled="isExternalUser" style="width:200px" />
            </el-form-item>
            <el-form-item label="发送邮件" prop="send_email" class="form-item-compact">
              <el-switch v-model="ruleForm.send_email" />
            </el-form-item>
          </div>
          <!-- 状态 单独一行 -->
          <el-form-item label="状态" prop="status" class="form-item-compact form-item-full">
            <el-radio-group v-model="ruleForm.status" :disabled="isExternalUser" size="small" class="user-status-radio">
              <el-radio-button :label="1" class="status-radio-enabled"><i class="el-icon-circle-check"></i>
                启用</el-radio-button>
              <el-radio-button :label="0" class="status-radio-disabled"><i class="el-icon-remove"></i>
                停用</el-radio-button>
            </el-radio-group>
          </el-form-item>
        </div>

        <div class="section-label"><i class="el-icon-s-grid"></i> 用户组与策略</div>
        <div class="edit-bottom-row">
          <el-form-item label="用户组" prop="groups" class="form-item-groups">
            <div class="group-picker" :class="{ 'group-picker--disabled': isExternalUser }">
              <div class="group-picker-header">
                <span class="group-picker-title">已选择 {{ ruleForm.groups.length }} / {{ groupNames.length }}</span>
              </div>
              <div v-if="groupNames.length" class="group-options">
                <button v-for="item in groupNames" :key="item" type="button" class="group-option"
                  :class="{ 'group-option--active': ruleForm.groups.includes(item) }" :title="item"
                  :disabled="isExternalUser" @click="toggleGroup(item)">
                  <span class="group-option-name">{{ item }}</span>
                  <span class="group-option-check">
                    <i class="el-icon-check"></i>
                  </span>
                </button>
              </div>
              <div v-else class="group-picker-empty">
                <i class="el-icon-info"></i> 暂无可用用户组，请先创建
              </div>
            </div>
          </el-form-item>
          <el-form-item label="个人策略" prop="policy_id">
            <el-select v-model="ruleForm.policy_id" placeholder="跟随组策略" style="width:200px" clearable>
              <el-option :key="0" label="跟随组策略（默认）" :value="0"></el-option>
              <el-option v-for="pt in policyList" :key="pt.id" :label="pt.name" :value="pt.id"></el-option>
            </el-select>
            <div class="form-tip form-tip-info"><i class="el-icon-info"></i> 选择后将覆盖用户组的策略，留空则使用组策略。</div>
          </el-form-item>
        </div>

        <div class="dialog-footer">
          <el-button @click="disVisible" icon="el-icon-close">取消</el-button>
          <el-button type="primary" icon="el-icon-check" :loading="isSubmitting" :disabled="isSubmitting"
            @click="submitForm('ruleForm')">保存用户</el-button>
        </div>
      </el-form>
    </el-dialog>
  </div>
</template>

<script>
import axios from "axios";
import userLabel from "@/mixins/userLabel";

export default {
  name: "UserList",
  mixins: [userLabel],
  created() {
    this.$emit('update:route_path', this.$route.path)
    this.$emit('update:route_name', ['用户管理', '用户列表'])
  },
  mounted() {
    this.getGroups();
    this.loadPolicyList();
    this.getData(1);
  },
  data() {
    return {
      loading: false,
      page: 1,
      pageSize: 10,
      groupNames: [],
      policyList: [],
      tableData: [],
      count: 0,
      // 统计卡片（后端按当前筛选条件全量聚合，不受分页影响）
      statsTotal: 0,
      statsLocal: 0,
      statsExternal: 0,
      statsActive: 0,
      statsDisable: 0,
      multipleSelection: [],
      pickerOptions: {
        disabledDate(time) {
          return time.getTime() < Date.now();
        }
      },
      searchData: '',
      filterType: '',
      filterGroup: '',
      filterStatus: '',
      otpImgData: { visible: false, username: '', nickname: '', base64Img: '' },
      ruleForm: {
        id: 0,
        type: 'local',
        username: '',
        nickname: '',
        email: '',
        phone: '',
        pin_code: '',
        limittime: null,
        disable_otp: false,
        otp_secret: '',
        change_pwd: true,
        send_email: true,
        status: 1,
        mtu: 0,
        policy_id: 0,
        groups: [],
      },
      isSubmitting: false,
      rules: {
        username: [
          { required: true, message: '请输入用户名', trigger: 'blur' },
          { max: 50, message: '长度不超过 50 个字符', trigger: 'blur' }
        ],
        nickname: [
          { required: true, message: '请输入用户姓名', trigger: 'blur' }
        ],
        email: [
          { required: true, message: '请输入用户邮箱', trigger: 'blur' },
          { type: 'email', message: '请输入正确的邮箱地址', trigger: ['blur', 'change'] }
        ],
        pin_code: [{ min: 6, message: 'PIN码至少 6 个字符', trigger: 'blur' }],
        groups: [{ type: 'array', required: true, message: '请至少选择一个组', trigger: 'change' }],
      },
    }
  },
  computed: {
    // 统计卡片按"当前筛选条件下全量数据"聚合，由后端返回，不受当前分页影响
    statTotal() { return this.statsTotal },
    statLocal() { return this.statsLocal },
    statExternal() { return this.statsExternal },
    statActive() { return this.statsActive },
    isExternalUser() {
      return this.ruleForm.type === 'ldap' || this.ruleForm.type === 'wxwork' || this.ruleForm.type === 'feishu';
    },
  },
  methods: {
    formatTraffic(bytes) {
      if (!bytes || bytes <= 0) return '-'
      const GB = 1024 * 1024 * 1024
      const MB = 1024 * 1024
      const KB = 1024
      if (bytes >= GB) return (bytes / GB).toFixed(2) + ' GB'
      if (bytes >= MB) return (bytes / MB).toFixed(2) + ' MB'
      if (bytes >= KB) return (bytes / KB).toFixed(2) + ' KB'
      return bytes + ' B'
    },
    isUserExpired(row) {
      return row.status === 1 && row.limittime && new Date(row.limittime) < new Date();
    },
    getStatusTagType(row) {
      if (row.status === 0) return 'danger';
      if (this.isUserExpired(row)) return 'warning';
      return 'success';
    },
    getStatusTagClass(row) {
      if (row.status === 0) return 'user-status-tag user-status-tag--disabled';
      if (this.isUserExpired(row)) return 'user-status-tag user-status-tag--expired';
      return 'user-status-tag user-status-tag--enabled';
    },
    getStatusLabel(row) {
      if (row.status === 0) return '停用';
      if (this.isUserExpired(row)) return '过期';
      return '启用';
    },
    handleRowCmd(row, cmd) {
      switch (cmd) {
        case 'edit': this.handleEdit(row); break;
        case 'otp': this.getOtpImg(row); break;
        case 'resetTraffic':
          this.$confirm(`确定要重置用户 "${row.username}" 的流量配额计数吗？`, '重置确认', {
            confirmButtonText: '确定重置',
            cancelButtonText: '取消',
            type: 'warning',
          }).then(() => {
            axios.post('/user/reset_traffic', null, { params: { username: row.username } }).then(resp => {
              if (resp.data.code === 0) {
                this.$message.success('流量配额已重置');
              } else {
                this.$message.error(resp.data.msg);
              }
            }).catch(() => this.$message.error('请求出错'));
          }).catch(() => { });
          break;
        case 'delete':
          this.$confirm('确定要删除该用户吗？删除后不可恢复。', '删除确认', {
            confirmButtonText: '确定删除',
            cancelButtonText: '取消',
            type: 'warning',
            confirmButtonClass: 'el-button--danger',
          }).then(() => this.handleDel(row)).catch(() => { });
          break;
      }
    },
    handleBatchCmd(cmd) {
      switch (cmd) {
        case 'email': this.batchSendEmail(); break;
        case 'delete': this.batchDelete(); break;
      }
    },
    upLoadUser(item) {
      const formData = new FormData();
      formData.append("file", item.file);
      axios.post('/user/uploaduser', formData, {
        headers: { 'Content-Type': 'multipart/form-data' }
      }).then(resp => {
        if (resp.data.code === 0) {
          this.$message.success(resp.data.data);
          this.getData(1);
        } else {
          this.$message.error(resp.data.msg);
          this.getData(1);
        }
      });
    },
    downloadTemplate() {
      axios({ method: 'get', url: '/user/uploaduser_template', responseType: 'blob' })
        .then(res => {
          const link = document.createElement('a');
          const url = window.URL.createObjectURL(new Blob([res.data]));
          link.href = url;
          link.download = '批量添加用户模版.xlsx';
          link.click();
          window.URL.revokeObjectURL(url);
        })
        .catch(() => {
          this.$message.error('下载模版失败');
        });
    },
    getOtpImg(row) {
      this.otpImgData.visible = true;
      axios.get('/user/otp_qr', { params: { id: row.id, b64: '1' } }).then(resp => {
        this.otpImgData.username = row.username;
        this.otpImgData.nickname = row.nickname;
        this.otpImgData.base64Img = 'data:image/png;base64,' + resp.data;
      }).catch(() => {
        this.$message.error('请求出错');
      });
    },
    handleDel(row) {
      axios.post('/user/del?id=' + row.id).then(resp => {
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
        this.ruleForm = this.getDefaultForm();
        return;
      }
      axios.get('/user/detail', { params: { id: row.id } }).then(resp => {
        const data = resp.data.data;
        data.send_email = false;
        data.limittime = data.limittime ? new Date(data.limittime) : null;
        this.ruleForm = data;
      }).catch(() => {
        this.$message.error('请求出错');
      });
    },
    handleSearch() {
      this.getData(1, this.searchData);
    },
    handleSelectionChange(val) {
      this.multipleSelection = val;
    },
    batchSendEmail() {
      if (this.multipleSelection.length === 0) {
        this.$message.warning('请选择要发送邮件的用户');
        return;
      }
      this.$confirm('确定要给选中的用户发送账号邮件吗？', '批量发送邮件', {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning'
      }).then(() => {
        const userIds = this.multipleSelection.map(u => u.id);
        axios.post('/user/batch/send_email', { user_ids: userIds }).then(resp => {
          if (resp.data.code === 0) {
            this.$message.success('批量发送邮件成功');
          } else {
            this.$message.error(resp.data.msg);
          }
        }).catch(() => this.$message.error('批量发送邮件失败'));
      });
    },
    batchDelete() {
      if (this.multipleSelection.length === 0) {
        this.$message.warning('请选择要删除的用户');
        return;
      }
      this.$confirm('确定要删除选中的用户吗？此操作不可恢复！', '批量删除', {
        confirmButtonText: '确定删除',
        cancelButtonText: '取消',
        type: 'danger'
      }).then(() => {
        const userIds = this.multipleSelection.map(u => u.id);
        axios.post('/user/batch/delete', { user_ids: userIds }).then(resp => {
          const data = resp.data || {};
          if (data.code === 0) {
            this.$message.success(data.msg || '批量删除成功');
            this.getData(this.page);
            this.$refs.multipleTable.clearSelection();
          } else {
            // 部分成功/失败：后端 msg 已含成功/失败/跳过数量，直接提示
            this.$message({ type: 'warning', message: data.msg || '批量删除失败', duration: 4000 });
            this.getData(this.page);
            this.$refs.multipleTable.clearSelection();
          }
        }).catch(() => this.$message.error('批量删除失败'));
      });
    },
    pageChange(p) {
      this.getData(p, this.searchData);
    },
    getData(page, prefix) {
      this.page = page;
      this.loading = true;
      axios.get('/user/list', {
        params: {
          page,
          prefix: prefix || '',
          type: this.filterType || '',
          group: this.filterGroup || '',
          status: this.filterStatus || '',
          page_size: this.pageSize
        }
      }).then(resp => {
        const data = resp.data.data || {};
        this.tableData = data.datas || [];
        this.count = data.count || 0;
        this.statsTotal = data.stats_total || 0;
        this.statsLocal = data.stats_local || 0;
        this.statsExternal = data.stats_external || 0;
        this.statsActive = data.stats_active || 0;
        this.statsDisable = data.stats_disable || 0;
      }).catch(() => {
        this.$message.error('请求出错');
      }).finally(() => {
        this.loading = false;
      });
    },
    handleSizeChange(val) {
      this.pageSize = val;
      this.getData(1, this.searchData);
    },
    getDefaultForm() {
      return {
        id: 0,
        type: 'local',
        username: '',
        nickname: '',
        email: '',
        phone: '',
        pin_code: '',
        limittime: null,
        disable_otp: false,
        otp_secret: '',
        change_pwd: true,
        send_email: true,
        status: 1,
        mtu: 0,
        policy_id: 0,
        groups: [],
      };
    },
    getGroups() {
      axios.get('/group/names').then(resp => {
        this.groupNames = resp.data.data.datas || [];
      }).catch(() => {
        this.$message.error('请求出错');
      });
    },
    toggleGroup(name) {
      const idx = this.ruleForm.groups.indexOf(name);
      if (idx >= 0) {
        this.ruleForm.groups.splice(idx, 1);
      } else {
        this.ruleForm.groups.push(name);
      }
    },
    loadPolicyList() {
      axios.get('/policy/names').then(resp => {
        if (resp.data.code === 0) {
          this.policyList = resp.data.data.datas || [];
        }
      }).catch(() => { });
    },
    submitForm(formName) {
      this.$refs[formName].validate((valid) => {
        if (!valid) return false;
        if (this.isSubmitting) return;
        this.isSubmitting = true;
        axios.post('/user/set', this.ruleForm).then(resp => {
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
  },
}
</script>

<style scoped>
/* ========== 页面整体 ========== */
.user-page {
  padding: 4px 0;
}

/* ========== 统计卡片 ========== */
.stat-icon-total {
  background: var(--color-primary-bg);
  color: var(--color-primary);
}

.stat-icon-local {
  background: var(--success-bg);
  color: var(--color-success);
}

.stat-icon-ldap {
  background: var(--warning-bg);
  color: var(--color-warning);
}

.stat-icon-external {
  background: var(--primary-bg);
  color: var(--color-primary);
}

.stat-icon-active {
  background: var(--danger-bg);
  color: var(--color-danger);
}

/* 页面特有 */
.inline-upload {
  display: inline-block;
}

/* ========== 表格内样式 ========== */
.user-name {
  font-weight: 600;
  color: var(--text-primary);
  font-size: 13px;
}

.user-nickname {
  font-size: 12px;
  color: var(--text-secondary);
  margin-left: 4px;
}

.group-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 3px;
}

.group-tag {
  font-size: 11px !important;
}

.text-muted {
  color: var(--text-placeholder);
  font-size: 12px;
}

.user-status-tag {
  min-width: 44px;
}

.force-pwd-tag {
  display: block;
  width: fit-content;
  margin: 4px auto 0;
}

.user-status-tag--enabled {
  color: var(--color-success) !important;
  background: var(--success-bg) !important;
  border-color: #c2e7b0 !important;
}

.user-status-tag--disabled {
  color: var(--color-danger) !important;
  background: var(--danger-bg) !important;
  border-color: #fbc4c4 !important;
}

.user-status-tag--expired {
  color: var(--color-warning) !important;
  background: var(--warning-bg) !important;
  border-color: #f5dab1 !important;
}

/* 操作按钮 */
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

/* 分页 */
.pagination-wrap {
  display: flex;
  justify-content: flex-end;
  padding-top: 16px;
}

/* 表格滚动容器 */
.user-table-wrap {
  overflow-x: auto;
  width: 100%;
}

/* ========== OTP 弹窗 ========== */
.otp-dialog-content {
  text-align: center;
}

.otp-user-info {
  font-size: 14px;
  color: var(--text-primary);
  margin-bottom: 12px;
}

.otp-qr-img {
  max-width: 200px;
  border: 1px solid var(--border-color-light);
  border-radius: 8px;
  padding: 8px;
}

.otp-tip {
  font-size: 12px;
  color: var(--text-secondary);
  margin-top: 10px;
}

/* ========== 编辑弹窗 ========== */
.user-edit-dialog ::v-deep .el-dialog__body {
  padding: 20px 30px 10px;
}

.section-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
  margin: 4px 0 14px;
  padding-left: 10px;
  border-left: 3px solid var(--color-primary);
}

.section-label i {
  margin-right: 6px;
  color: var(--color-primary);
  font-size: 14px;
}

.help-icon {
  color: var(--text-placeholder);
  margin-left: 6px;
  cursor: pointer;
  font-size: 15px;
  vertical-align: -1px;
}

.help-icon:hover {
  color: var(--color-primary);
}

.edit-basic-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 30px;
}

.edit-adv-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 30px;
}

.edit-adv-row .form-item-compact {
  margin-bottom: 16px;
}

.edit-adv-section .form-item-compact {
  margin-bottom: 16px;
}

.form-item-full {
  grid-column: 1 / -1;
}

/* 用户组多选 */
.form-item-groups {
  margin-bottom: 16px;
}

.group-picker {
  width: 100%;
  max-width: 560px;
  border: 1px solid var(--border-base);
  border-radius: 6px;
  background: var(--bg-card);
  box-sizing: border-box;
  overflow: hidden;
}

.group-picker-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 30px;
  padding: 0 12px;
  border-bottom: 1px solid var(--border-color-light);
  background: var(--bg-header);
}

.group-picker-title {
  font-size: 12px;
  color: var(--text-regular);
  font-weight: 500;
}

.group-options {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(92px, 1fr));
  gap: 6px;
  padding: 8px;
  max-height: 116px;
  overflow-y: auto;
}

.group-option {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  min-height: 28px;
  padding: 5px 8px;
  border: 1px solid var(--border-base);
  border-radius: 14px;
  background: var(--bg-header);
  color: var(--text-regular);
  cursor: pointer;
  box-sizing: border-box;
  font: inherit;
  text-align: left;
  transition: border-color .2s ease, background .2s ease, color .2s ease;
}

.group-option:hover {
  border-color: #95d475;
  background: var(--success-bg);
}

.group-option--active {
  border-color: var(--color-success);
  background: var(--color-success);
  color: var(--text-inverse);
}

.group-option-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  font-weight: 500;
}

.group-option-check {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  border: 1px solid var(--border-base);
  background: var(--bg-card);
  color: transparent;
  font-size: 10px;
  flex-shrink: 0;
}

.group-option--active .group-option-check {
  border-color: var(--text-inverse);
  background: var(--bg-card);
  color: var(--color-success);
}

.group-picker-empty {
  padding: 18px 12px;
  text-align: center;
  color: var(--text-secondary);
  font-size: 12px;
}

.group-picker-empty i {
  margin-right: 4px;
}

.group-picker--disabled {
  background: var(--bg-hover);
}

.group-picker--disabled .group-option {
  cursor: not-allowed;
  opacity: .65;
}

.form-tip {
  font-size: 12px;
  color: var(--text-secondary);
  margin-left: 8px;
}

.user-status-radio ::v-deep .el-radio-button__inner {
  min-width: 72px;
}

.user-status-radio ::v-deep .status-radio-enabled .el-radio-button__inner {
  color: var(--color-success);
  border-color: #c2e7b0;
  background: var(--success-bg);
}

.user-status-radio ::v-deep .status-radio-disabled .el-radio-button__inner {
  color: var(--color-danger);
  border-color: #fbc4c4;
  background: var(--danger-bg);
}

.user-status-radio ::v-deep .status-radio-expired .el-radio-button__inner {
  color: var(--color-warning);
  border-color: #f5dab1;
  background: var(--warning-bg);
}

.user-status-radio ::v-deep .status-radio-enabled.is-active .el-radio-button__inner {
  color: var(--text-inverse);
  border-color: var(--color-success);
  background: var(--color-success);
  box-shadow: -1px 0 0 0 var(--color-success);
}

.user-status-radio ::v-deep .status-radio-disabled.is-active .el-radio-button__inner {
  color: var(--text-inverse);
  border-color: var(--color-danger);
  background: var(--color-danger);
  box-shadow: -1px 0 0 0 var(--color-danger);
}

.user-status-radio ::v-deep .status-radio-expired.is-active .el-radio-button__inner {
  color: var(--text-inverse);
  border-color: var(--color-warning);
  background: var(--color-warning);
  box-shadow: -1px 0 0 0 var(--color-warning);
}

.otp-row {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 40px;
}

.otp-secret-input {
  flex: 1;
}

.force-pwd-tip {
  font-size: 12px;
  color: var(--color-warning);
  font-weight: 500;
}

.otp-row ::v-deep .el-switch {
  line-height: 1;
}

.form-item-otp ::v-deep .el-form-item__label {
  line-height: 40px;
  padding-top: 0;
}

.form-tip-info {
  display: block;
  margin: 4px 0 0;
  padding: 6px 10px;
  background: var(--info-bg);
  border-radius: 6px;
  font-size: 12px;
  color: var(--text-secondary);
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding-top: 8px;
}

/* 响应式 */
@media (max-width: 768px) {

  .edit-basic-row,
  .edit-adv-row {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 1024px) {
  .user-table-wrap ::v-deep .col-ops {
    min-width: 200px;
  }

  /* 标题与筛选栏改为上下排列，避免横屏/竖屏下标题和一堆按钮挤在同一行 */
  .card-header {
    flex-direction: column;
    align-items: stretch;
    gap: 10px;
  }

  .card-actions {
    flex-wrap: wrap;
    width: 100%;
    gap: 8px;
  }

  .card-actions .search-input,
  .card-actions .filter-select {
    flex: 1 1 140px;
    width: auto !important;
    min-width: 0;
    margin-left: 0;
  }
}

@media (max-width: 600px) {
  .user-table-wrap ::v-deep .col-ops {
    min-width: 140px;
  }

  .card-actions {
    flex-direction: row;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
  }

  .card-actions .search-input,
  .card-actions .filter-select {
    flex: 1 1 140px;
    width: auto !important;
    margin-left: 0;
  }

  /* 分页条在手机端隐藏冗余项并允许横向滚动，避免末页「下一页」等按钮被裁切 */
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
}
</style>
