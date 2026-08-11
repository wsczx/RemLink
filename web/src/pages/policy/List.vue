<template>
  <div class="policy-page">
    <!-- 顶部概览卡片 -->
    <div class="stats-row" :style="{ gridTemplateColumns: `repeat(${cardColumns}, 1fr)` }">
      <div class="stat-card">
        <div class="stat-icon stat-icon-total">
          <i class="el-icon-s-order"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ statTotal }}</div>
          <div class="stat-label">策略总数</div>
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
      <div class="stat-card" v-if="showFakeDNS">
        <div class="stat-icon stat-icon-dns">
          <i class="el-icon-connection"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ statFakeDNS }}</div>
          <div class="stat-label">启用FakeDNS</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-icon-bandwidth">
          <i class="el-icon-odometer"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ statBandwidth }}</div>
          <div class="stat-label">带宽限速</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-icon-quota">
          <i class="el-icon-data-line"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ statQuota }}</div>
          <div class="stat-label">流量配额</div>
        </div>
      </div>
    </div>

    <!-- 策略表格 -->
    <el-card class="table-card" shadow="never" v-loading="loading">
      <div slot="header" class="card-header">
        <span class="card-title"><i class="el-icon-s-management"></i> 策略列表</span>
        <el-button size="small" type="primary" icon="el-icon-plus" @click="handleEdit('')">
          添加策略
        </el-button>
      </div>

      <el-table ref="multipleTable" :data="tableData" border stripe highlight-current-row style="width:100%"
        :header-cell-style="{ background: 'var(--bg-header)', color: 'var(--text-primary)', fontWeight: '600', fontSize: '13px' }">
        <el-table-column sortable prop="id" label="ID" width="70" align="center"></el-table-column>
        <el-table-column prop="name" label="策略名称" min-width="140" sortable>
          <template slot-scope="scope">
            <div class="policy-name-cell">
              <span class="policy-name">{{ scope.row.name }}</span>
              <span v-if="scope.row.note" class="policy-note">{{ scope.row.note }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="allow_lan" label="本地网络" width="95" align="center" sortable>
          <template slot-scope="scope">
            <el-switch v-model="scope.row.allow_lan" disabled></el-switch>
          </template>
        </el-table-column>

        <el-table-column prop="bandwidth" label="下行" width="100" align="center" sortable>
          <template slot-scope="scope">
            <span v-if="scope.row.bandwidth > 0" class="bandwidth-badge">
              {{ convertBandwidth(scope.row.bandwidth, 'BYTE', 'Mbps') }} Mbps
            </span>
            <span v-else class="bandwidth-unlimited">不限</span>
          </template>
        </el-table-column>

        <el-table-column prop="bandwidth_up" label="上行" width="100" align="center" sortable>
          <template slot-scope="scope">
            <span v-if="scope.row.bandwidth_up > 0" class="bandwidth-badge">
              {{ convertBandwidth(scope.row.bandwidth_up, 'BYTE', 'Mbps') }} Mbps
            </span>
            <span v-else class="bandwidth-unlimited">不限</span>
          </template>
        </el-table-column>

        <el-table-column prop="traffic_quota" label="流量配额" width="130" align="center" sortable>
          <template slot-scope="scope">
            <span v-if="scope.row.traffic_quota > 0" class="quota-badge">
              {{ convertTraffic(scope.row.traffic_quota, 'BYTE', 'GB') }} GB
              <span class="quota-reset">{{ resetLabel(scope.row.traffic_reset) }}</span>
            </span>
            <span v-else class="bandwidth-unlimited">不限</span>
          </template>
        </el-table-column>

        <el-table-column v-if="showFakeDNS" prop="enable_fakedns" label="FakeDNS" width="100" align="center" sortable>
          <template slot-scope="scope">
            <el-tag v-if="scope.row.enable_fakedns" type="success" size="small" effect="plain">启用</el-tag>
            <el-tag v-else type="info" size="small" effect="plain">禁用</el-tag>
          </template>
        </el-table-column>

        <el-table-column label="引用" align="center" width="130">
          <template slot-scope="scope">
            <div class="ref-cell">
              <span class="ref-item">
                <i class="el-icon-s-custom"></i>
                <strong>{{ scope.row.user_count || 0 }}</strong>
                <span class="ref-label">用户</span>
              </span>
              <span class="ref-divider">|</span>
              <span class="ref-item">
                <i class="el-icon-s-order"></i>
                <strong>{{ scope.row.group_count || 0 }}</strong>
                <span class="ref-label">组</span>
              </span>
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="status" label="状态" width="90" align="center" sortable>
          <template slot-scope="scope">
            <span v-if="scope.row.status === 1" class="status-dot status-dot-online"></span>
            <span v-else class="status-dot status-dot-offline"></span>
            <span class="status-text" :class="scope.row.status === 1 ? 'text-success' : 'text-danger'">
              {{ scope.row.status === 1 ? '启用' : '停用' }}
            </span>
          </template>
        </el-table-column>

        <el-table-column prop="updated_at" label="更新时间" :formatter="tableDateFormat" width="165"
          sortable></el-table-column>

        <el-table-column label="操作" width="100" fixed="right" align="center">
          <template slot-scope="scope">
            <el-dropdown trigger="click" @command="(cmd) => handleRowCmd(scope.row, cmd)">
              <el-button size="mini" class="action-more-btn">
                操作<i class="el-icon-arrow-down el-icon--right"></i>
              </el-button>
              <el-dropdown-menu slot="dropdown">
                <el-dropdown-item command="edit" icon="el-icon-edit">编辑策略</el-dropdown-item>
                <el-dropdown-item command="copy" icon="el-icon-document-copy">复制策略</el-dropdown-item>
                <el-dropdown-item command="applyGroups" icon="el-icon-s-order">应用到组</el-dropdown-item>
                <el-dropdown-item command="applyUsers" icon="el-icon-s-custom">应用到用户</el-dropdown-item>
                <el-dropdown-item command="delete" icon="el-icon-delete" divided
                  class="dropdown-danger">删除策略</el-dropdown-item>
              </el-dropdown-menu>
            </el-dropdown>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination-wrap">
        <el-pagination background layout="total, prev, pager, next" :pager-count="9" :page-size="10"
          @current-change="pageChange" :current-page="page" :total="count">
        </el-pagination>
      </div>
    </el-card>

    <!-- 编辑弹窗 -->
    <el-dialog :close-on-click-modal="false" :title="isEdit ? '编辑策略' : '新增策略'" :visible.sync="editDialog" width="880px"
      @close="closeDialog" top="4vh" class="policy-edit-dialog">
      <el-form :model="ruleForm" :rules="rules" ref="ruleForm" label-width="100px" class="ruleForm">
        <div class="edit-basic-row">
          <el-form-item label="策略ID" prop="id" v-if="isEdit" class="form-item-compact">
            <el-input v-model="ruleForm.id" disabled></el-input>
          </el-form-item>
          <el-form-item label="策略名称" prop="name" class="form-item-compact">
            <el-input v-model="ruleForm.name" placeholder="请输入策略名称"></el-input>
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

        <PolicyForm ref="policyForm" v-model="ruleForm" :showSaveBtn="false" @open-ip-editor="openIpListDialog"
          @open-acl-editor="openAclEditor" />

        <el-divider></el-divider>

        <div class="dialog-footer">
          <el-button @click="closeDialog">取 消</el-button>
          <el-button type="primary" icon="el-icon-check" @click="submitForm('ruleForm')">保存策略</el-button>
        </div>
      </el-form>
    </el-dialog>

    <!-- 路由批量编辑弹窗 -->
    <el-dialog :close-on-click-modal="false" title="批量编辑路由" :visible.sync="ipListDialog" width="680px"
      class="valgin-dialog" center>
      <el-form ref="ipEditForm" label-width="80px">
        <el-form-item label="路由表" prop="ip_list">
          <el-input type="textarea" :rows="12" v-model="ipEditForm.ip_list"
            placeholder="每行一条路由，格式：192.168.1.0/24,备注&#10;或简写：192.168.1.0/24"></el-input>
          <div class="route-count-hint">
            <i class="el-icon-info"></i>
            当前共 <b>{{ ipEditForm.ip_list.trim() === '' ? 0 : ipEditForm.ip_list.trim().split('\n').length }}</b> 条路由
            （AnyConnect客户端最多支持 <b>{{ this.maxRouteRows }}</b> 条）
          </div>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="ipEdit()" :loading="ipEditLoading" icon="el-icon-check">更新路由</el-button>
          <el-button @click="ipListDialog = false">取 消</el-button>
        </el-form-item>
      </el-form>
    </el-dialog>

    <!-- ACL 批量编辑弹窗 -->
    <el-dialog :close-on-click-modal="false" title="批量编辑 ACL 规则" :visible.sync="aclEditDialog" width="760px"
      class="valgin-dialog" center>
      <el-form ref="aclEditForm" label-width="80px">
        <div class="acl-edit-hint">
          <i class="el-icon-info"></i>
          每行一条规则，格式：<code>动作,CIDR地址,协议,端口,备注</code>。动作：allow/deny，协议：all/tcp/udp/icmp，端口为0表示不限。
        </div>
        <el-form-item label="ACL规则" prop="acl_list">
          <el-input type="textarea" :rows="14" v-model="aclEditForm.acl_list"
            placeholder="allow,192.168.1.0/24,tcp,80,公司内网&#10;deny,10.0.0.0/8,all,0,禁止内部网段"></el-input>
          <div class="route-count-hint">
            <i class="el-icon-info"></i>
            当前共 <b>{{aclEditForm.acl_list.trim() === '' ? 0 : aclEditForm.acl_list.trim().split('\n').filter(l =>
              l.trim()).length}}</b> 条规则
          </div>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="aclEdit()" :loading="aclEditLoading" icon="el-icon-check">更新 ACL</el-button>
          <el-button @click="aclEditDialog = false">取 消</el-button>
        </el-form-item>
      </el-form>
    </el-dialog>

    <!-- 应用到组 -->
    <el-dialog :close-on-click-modal="false" title="应用策略到用户组" :visible.sync="applyGroupsDialog" width="580px" center
      class="apply-dialog">
      <div class="apply-intro">
        <i class="el-icon-link"></i>
        将策略 <el-tag type="warning" size="small" effect="plain">{{ applyPolicyName }}</el-tag> 应用到以下用户组：
      </div>
      <div v-if="allGroups.length > 0" class="apply-list">
        <el-checkbox-group v-model="selectedGroupIds">
          <div v-for="g in allGroups" :key="g.id" class="apply-item">
            <el-checkbox :label="g.id">
              <span class="apply-item-name">{{ g.name }}</span>
              <el-tag v-if="g.policy_id === applyPolicyId" size="mini" type="success" effect="plain">当前策略</el-tag>
            </el-checkbox>
          </div>
        </el-checkbox-group>
      </div>
      <div v-else class="apply-empty">
        <i class="el-icon-folder-opened"></i>
        <p>暂无可用的用户组</p>
      </div>
      <span slot="footer">
        <el-button @click="applyGroupsDialog = false">取消</el-button>
        <el-button type="primary" @click="doApplyToGroups" :loading="applyLoading" icon="el-icon-check">确认应用</el-button>
      </span>
    </el-dialog>

    <!-- 应用到用户 -->
    <el-dialog :close-on-click-modal="false" title="应用策略到用户" :visible.sync="applyUsersDialog" width="680px" center
      class="apply-dialog">
      <div class="apply-intro">
        <i class="el-icon-link"></i>
        将策略 <el-tag type="warning" size="small" effect="plain">{{ applyPolicyName }}</el-tag> 应用到以下用户：
      </div>
      <el-input v-model="userSearch" placeholder="搜索用户名或昵称..." size="small" prefix-icon="el-icon-search" clearable
        class="apply-search"></el-input>
      <div v-if="filteredUsers.length > 0" class="apply-list apply-list-scroll">
        <el-checkbox-group v-model="selectedUserIds">
          <div v-for="u in filteredUsers" :key="u.id" class="apply-item">
            <el-checkbox :label="u.id">
              <span class="apply-item-name">{{ u.username }}</span>
              <span class="apply-item-sub">({{ u.nickname }})</span>
              <el-tag v-if="u.policy_id === applyPolicyId" size="mini" type="success" effect="plain">当前策略</el-tag>
            </el-checkbox>
          </div>
        </el-checkbox-group>
      </div>
      <div v-else class="apply-empty">
        <i class="el-icon-search"></i>
        <p>暂无匹配的用户</p>
      </div>
      <span slot="footer">
        <el-button @click="applyUsersDialog = false">取消</el-button>
        <el-button type="primary" @click="doApplyToUsers" :loading="applyLoading" icon="el-icon-check">确认应用</el-button>
      </span>
    </el-dialog>
  </div>
</template>

<script>
import axios from "axios";
import PolicyForm from "@/components/PolicyForm";

export default {
  name: "PolicyList",
  components: { PolicyForm },
  created() {
    this.$emit('update:route_path', this.$route.path)
    this.$emit('update:route_name', ['策略管理'])
  },
  mounted() {
    this.getData(1);
    this.loadGroups();
    this.loadUsers();
    window.addEventListener('resize', this.onResize);
  },
  beforeDestroy() {
    window.removeEventListener('resize', this.onResize);
  },
  data() {
    return {
      loading: false,
      page: 1,
      tableData: [],
      count: 10,
      maxRouteRows: 2500,
      editDialog: false,
      isEdit: false,
      ruleForm: this.getDefaultForm(),
      rules: {
        name: [
          { required: true, message: '请输入策略名称', trigger: 'blur' },
          { max: 60, message: '长度小于 60 个字符', trigger: 'blur' }
        ],
        bandwidth_format: [
          { required: true, message: '请输入下行带宽', trigger: 'blur' },
        ],
        bandwidth_up_format: [
          { required: true, message: '请输入上行带宽', trigger: 'blur' },
        ],
        traffic_quota_format: [
          { required: true, message: '请输入流量配额', trigger: 'blur' },
        ],
      },
      // IP编辑
      ipListDialog: false,
      ipEditForm: { ip_list: "", type: "" },
      ipEditLoading: false,
      // ACL编辑
      aclEditDialog: false,
      aclEditForm: { acl_list: "" },
      aclEditLoading: false,
      // 反向应用
      applyPolicyId: 0,
      applyPolicyName: '',
      applyGroupsDialog: false,
      applyUsersDialog: false,
      applyLoading: false,
      allGroups: [],
      selectedGroupIds: [],
      allUsers: [],
      selectedUserIds: [],
      userSearch: '',
      // 视口宽度，用于统计卡片列数自适应（避免手机端卡片被压成极窄一列）
      windowWidth: typeof window !== 'undefined' ? window.innerWidth : 1280,
    }
  },
  computed: {
    statTotal() { return this.count },
    statActive() { return this.tableData.filter(r => r.status === 1).length },
    statFakeDNS() { return this.tableData.filter(r => r.enable_fakedns).length },
    statBandwidth() { return this.tableData.filter(r => r.bandwidth > 0 || r.bandwidth_up > 0).length },
    statQuota() { return this.tableData.filter(r => r.traffic_quota > 0).length },
    cardColumns() {
      // 根据视口宽度自适应列数，避免窄屏下卡片被压得过窄
      if (this.windowWidth <= 768) return 2
      if (this.windowWidth <= 1200) return 3
      return this.showFakeDNS ? 5 : 4
    },
    filteredUsers() {
      if (!this.userSearch) return this.allUsers;
      const s = this.userSearch.toLowerCase();
      return this.allUsers.filter(u =>
        u.username.toLowerCase().includes(s) ||
        (u.nickname && u.nickname.toLowerCase().includes(s))
      );
    }
  },
  methods: {
    onResize() {
      this.windowWidth = window.innerWidth;
    },
    getDefaultForm() {
      return {
        id: 0,
        name: '',
        note: '',
        status: 1,
        bandwidth: 0,
        bandwidth_format: '0',
        bandwidth_up: 0,
        bandwidth_up_format: '0',
        traffic_quota: 0,
        traffic_quota_format: '0',
        traffic_reset: '',
        allow_lan: true,
        client_dns: [{ val: '114.114.114.114', note: '默认dns' }],
        route_include: [{ val: 'all', note: '默认全局代理' }],
        route_exclude: [],
        link_acl: [],
        ds_include_domains: '',
        ds_exclude_domains: '',
        enable_fakedns: false,
        fake_dns_upstream: '',
        fake_dns_include: '',
        fake_dns_exclude: '',
        prefer_ipv6: false,
      }
    },
    getData(page) {
      this.page = page
      this.loading = true;
      axios.get('/policy/list', { params: { page } }).then(resp => {
        const rdata = resp.data.data || {};
        this.tableData = rdata.datas || [];
        this.count = rdata.count || 0;
        this.loading = false;
      }).catch(() => {
        this.$message.error('请求出错');
        this.loading = false;
      });
    },
    loadGroups() {
      axios.get('/group/list', { params: { page: 1, page_size: 9999 } }).then(resp => {
        this.allGroups = (resp.data.data.datas || []).map(g => ({
          id: g.id, name: g.name, policy_id: g.policy_id
        }));
      }).catch(() => { });
    },
    loadUsers() {
      axios.get('/user/list', { params: { page: 1, page_size: 9999 } }).then(resp => {
        this.allUsers = (resp.data.data.datas || []).map(u => ({
          id: u.id, username: u.username, nickname: u.nickname, policy_id: u.policy_id
        }));
      }).catch(() => { });
    },
    pageChange(p) { this.getData(p) },

    // 统一的行操作入口
    handleRowCmd(row, cmd) {
      switch (cmd) {
        case 'edit': this.handleEdit(row); break;
        case 'copy': this.handleCopy(row); break;
        case 'applyGroups': this.handleApply(row, 'groups'); break;
        case 'applyUsers': this.handleApply(row, 'users'); break;
        case 'delete':
          this.$confirm('确定要删除该策略吗？删除后不可恢复。', '删除确认', {
            confirmButtonText: '确定删除',
            cancelButtonText: '取消',
            type: 'warning',
            confirmButtonClass: 'el-button--danger',
          }).then(() => this.handleDel(row)).catch(() => { });
          break;
      }
    },

    handleEdit(row) {
      this.$refs['ruleForm'] && this.$refs['ruleForm'].resetFields();
      this.editDialog = true;
      if (!row) {
        this.isEdit = false;
        this.ruleForm = this.getDefaultForm();
        return;
      }
      this.isEdit = true;
      axios.get('/policy/detail', { params: { id: row.id } }).then(resp => {
        const d = resp.data.data;
        d.bandwidth_format = this.convertBandwidth(d.bandwidth, 'BYTE', 'Mbps').toString();
        d.bandwidth_up_format = this.convertBandwidth((d.bandwidth_up || 0), 'BYTE', 'Mbps').toString();
        d.traffic_quota_format = this.convertTraffic((d.traffic_quota || 0), 'BYTE', 'GB').toString();
        if (!d.traffic_reset) d.traffic_reset = '';
        this.ruleForm = d;
        if (d.client_dns === null) this.ruleForm.client_dns = [{ val: '114.114.114.114', note: '默认dns' }];
        if (d.route_include === null) this.ruleForm.route_include = [{ val: 'all', note: '默认全局代理' }];
        if (d.route_exclude === null) this.ruleForm.route_exclude = [];
        if (d.link_acl === null) this.ruleForm.link_acl = [];
      }).catch(() => {
        this.$message.error('请求出错');
      });
    },
    handleCopy(row) {
      this.$confirm(`确定要复制策略 "${row.name}" 吗？`, '复制确认', {
        confirmButtonText: '确定复制',
        type: 'info',
      }).then(() => {
        axios.post('/policy/copy?id=' + row.id).then(resp => {
          if (resp.data.code === 0) {
            this.$message.success('策略已复制');
            this.getData(1);
          } else {
            this.$message.error(resp.data.msg);
          }
        }).catch(() => {
          this.$message.error('请求出错');
        });
      }).catch(() => { });
    },
    handleApply(row, cmd) {
      this.applyPolicyId = row.id;
      this.applyPolicyName = row.name;
      this.selectedGroupIds = [];
      this.selectedUserIds = [];
      this.userSearch = '';
      if (cmd === 'groups') {
        this.applyGroupsDialog = true;
        this.loadGroups();
      } else {
        this.applyUsersDialog = true;
        this.loadUsers();
      }
    },
    doApplyToGroups() {
      if (this.selectedGroupIds.length === 0) {
        this.$message.warning('请选择至少一个用户组');
        return;
      }
      this.applyLoading = true;
      axios.post('/policy/apply_to_groups', {
        policy_id: this.applyPolicyId,
        group_ids: this.selectedGroupIds,
      }).then(resp => {
        this.applyLoading = false;
        if (resp.data.code === 0) {
          this.$message.success(resp.data.msg);
          this.applyGroupsDialog = false;
          this.getData(this.page);
        } else {
          this.$message.error(resp.data.msg);
        }
      }).catch(() => {
        this.applyLoading = false;
        this.$message.error('请求出错');
      });
    },
    doApplyToUsers() {
      if (this.selectedUserIds.length === 0) {
        this.$message.warning('请选择至少一个用户');
        return;
      }
      this.applyLoading = true;
      axios.post('/policy/apply_to_users', {
        policy_id: this.applyPolicyId,
        user_ids: this.selectedUserIds,
      }).then(resp => {
        this.applyLoading = false;
        if (resp.data.code === 0) {
          this.$message.success(resp.data.msg);
          this.applyUsersDialog = false;
          this.getData(this.page);
        } else {
          this.$message.error(resp.data.msg);
        }
      }).catch(() => {
        this.applyLoading = false;
        this.$message.error('请求出错');
      });
    },
    handleDel(row) {
      axios.post('/policy/del?id=' + row.id).then(resp => {
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
    submitForm(formName) {
      this.$refs[formName].validate((valid) => {
        if (!valid) return false;
        // 单位转换：Mbps → Byte/s，GB → 字节
        this.ruleForm.bandwidth = this.convertBandwidth(this.ruleForm.bandwidth_format, 'Mbps', 'BYTE');
        this.ruleForm.bandwidth_up = this.convertBandwidth(this.ruleForm.bandwidth_up_format, 'Mbps', 'BYTE');
        const quotaGb = parseFloat(this.ruleForm.traffic_quota_format) || 0;
        this.ruleForm.traffic_quota = quotaGb > 0 ? Math.round(quotaGb * 1024 * 1024 * 1024) : 0;
        if (this.ruleForm.traffic_quota === 0) this.ruleForm.traffic_reset = '';
        else if (!this.ruleForm.traffic_reset) this.ruleForm.traffic_reset = 'monthly';
        axios.post('/policy/set', this.ruleForm).then(resp => {
          if (resp.data.code === 0) {
            this.$message.success(resp.data.msg);
            this.getData(1);
            this.editDialog = false;
          } else {
            this.$message.error(resp.data.msg);
          }
        }).catch(() => {
          this.$message.error('请求出错');
        });
      });
    },
    openIpListDialog(type) {
      this.ipListDialog = true;
      this.ipEditForm.type = type;
      this.ipEditForm.ip_list = this.ruleForm[type].map(item =>
        item.val + (item.note ? "," + item.note : "")
      ).join("\n");
    },
    ipEdit() {
      this.ipEditLoading = true;
      let ipList = [];
      if (this.ipEditForm.ip_list.trim() !== "") {
        ipList = this.ipEditForm.ip_list.trim().split("\n");
      }
      let arr = [];
      for (let i = 0; i < ipList.length; i++) {
        let item = ipList[i];
        if (item.trim() === "") continue;
        let ip = item.split(",");
        if (ip.length > 2) {
          ip[1] = ip.slice(1).join(",");
        }
        let note = ip[1] ? ip[1] : "";
        if (this.ipEditForm.type == "route_include" && ip[0] == "all") {
          arr.push({ val: ip[0], note: note });
          continue;
        }
        let valid = this.isValidCIDR(ip[0]);
        if (!valid.valid) {
          this.$message.error("CIDR格式错误，建议 " + ip[0] + " 改为 " + valid.suggestion);
          this.ipEditLoading = false;
          return;
        }
        arr.push({ val: ip[0], note: note });
      }
      this.ruleForm[this.ipEditForm.type] = arr;
      this.ipEditLoading = false;
      this.ipListDialog = false;
    },
    openAclEditor() {
      this.aclEditDialog = true;
      this.aclEditForm.acl_list = (this.ruleForm.link_acl || []).map(item =>
        [item.action || 'allow', item.val, item.protocol || 'all', item.port || '0', item.note || ''].join(",")
      ).join("\n");
    },
    aclEdit() {
      this.aclEditLoading = true;
      const lines = this.aclEditForm.acl_list.trim();
      if (!lines) {
        this.ruleForm.link_acl = [];
        this.aclEditLoading = false;
        this.aclEditDialog = false;
        return;
      }
      const arr = [];
      const rawLines = lines.split("\n");
      for (let i = 0; i < rawLines.length; i++) {
        const line = rawLines[i].trim();
        if (!line) continue;
        const parts = line.split(",");
        const action = (parts[0] || 'allow').trim();
        if (action !== 'allow' && action !== 'deny') {
          this.$message.error(`第${i + 1}行动作无效：${action}，请使用 allow 或 deny`);
          this.aclEditLoading = false;
          return;
        }
        arr.push({
          action: action,
          val: (parts[1] || '').trim(),
          protocol: (parts[2] || 'all').trim(),
          port: (parts[3] || '0').trim(),
          note: parts.length > 4 ? parts.slice(4).join(",").trim() : '',
        });
      }
      this.ruleForm.link_acl = arr;
      this.aclEditLoading = false;
      this.aclEditDialog = false;
    },
    isValidCIDR(input) {
      // IPv6 CIDR：含冒号即按 v6 处理，仅做基本格式校验（不做网络位归一建议）
      if (input.indexOf(":") !== -1) {
        return this.isValidCIDR6(input);
      }
      const cidrRegex = /^((25[0-5]|2[0-4]\d|[01]?\d\d?)\.){3}(25[0-5]|2[0-4]\d|[01]?\d\d?)\/([12]?\d|3[0-2])$/;
      if (!cidrRegex.test(input)) {
        return { valid: false, suggestion: null };
      }
      const [ip, mask] = input.split('/');
      const maskNum = parseInt(mask);
      const ipParts = ip.split('.').map(part => parseInt(part));
      const binaryIP = ipParts.map(part => part.toString(2).padStart(8, '0')).join('');
      for (let i = maskNum; i < 32; i++) {
        if (binaryIP[i] === '1') {
          const binaryNetworkPart = binaryIP.substring(0, maskNum).padEnd(32, '0');
          const networkIPParts = [];
          for (let j = 0; j < 4; j++) {
            const octet = binaryNetworkPart.substring(j * 8, (j + 1) * 8);
            networkIPParts.push(parseInt(octet, 2));
          }
          const suggestedIP = networkIPParts.join('.');
          return { valid: false, suggestion: `${suggestedIP}/${mask}` };
        }
      }
      return { valid: true, suggestion: null };
    },
    // IPv6 CIDR 校验：形如 2001:db8::/64，前缀 0-128；支持 :: 缩写与末段内嵌 IPv4
    isValidCIDR6(input) {
      const parts = input.split("/");
      if (parts.length !== 2) {
        return { valid: false, suggestion: null };
      }
      const prefix = parseInt(parts[1], 10);
      if (isNaN(prefix) || prefix < 0 || prefix > 128 || !/^\d+$/.test(parts[1])) {
        return { valid: false, suggestion: null };
      }
      const addr = parts[0];
      const dbl = addr.split("::");
      if (dbl.length > 2) {
        return { valid: false, suggestion: null };
      }
      const isHextet = h => /^[0-9a-fA-F]{1,4}$/.test(h);
      const isV4 = s => /^((25[0-5]|2[0-4]\d|[01]?\d\d?)\.){3}(25[0-5]|2[0-4]\d|[01]?\d\d?)$/.test(s);
      const countGroups = seg => {
        if (seg === "") return 0;
        const gs = seg.split(":");
        let n = 0;
        for (let i = 0; i < gs.length; i++) {
          const g = gs[i];
          if (g === "") return -1;
          if (i === gs.length - 1 && isV4(g)) { n += 2; continue; }
          if (!isHextet(g)) return -1;
          n += 1;
        }
        return n;
      };
      if (dbl.length === 2) {
        const l = countGroups(dbl[0]);
        const r = countGroups(dbl[1]);
        if (l < 0 || r < 0 || l + r > 7) {
          return { valid: false, suggestion: null };
        }
        return { valid: true, suggestion: null };
      }
      const total = countGroups(addr);
      if (total !== 8) {
        return { valid: false, suggestion: null };
      }
      return { valid: true, suggestion: null };
    },
    closeDialog() {
      this.editDialog = false;
    },
    // 带宽单位换算，BYTE 此处代表 bits（1 Byte = 8 bits）
    // 后端 Policy.Bandwidth 单位为 Byte/s，换算公式: Byte/s × 8 ÷ 10^6 = Mbps
    convertBandwidth(bandwidth, fromUnit, toUnit) {
      const units = { bps: 1, Kbps: 1000, Mbps: 1000000, Gbps: 1000000000, BYTE: 8 };
      const result = bandwidth * units[fromUnit] / units[toUnit];
      return parseFloat(result.toFixed(2));
    },
    // 流量单位换算，基于 1024（IEC 标准）
    convertTraffic(bytes, fromUnit, toUnit) {
      const units = { BYTE: 1, KB: 1024, MB: 1024 * 1024, GB: 1024 * 1024 * 1024, TB: 1024 * 1024 * 1024 * 1024 };
      const result = bytes * units[fromUnit] / units[toUnit];
      return parseFloat(result.toFixed(2));
    },
    resetLabel(period) {
      switch (period) {
        case 'daily': return '/日';
        case 'weekly': return '/周';
        case 'monthly': return '/月';
        default: return '';
      }
    },
    tableDateFormat(row, column, cellValue) {
      if (!cellValue) return '';
      const d = new Date(cellValue);
      return d.getFullYear() + '-' +
        String(d.getMonth() + 1).padStart(2, '0') + '-' +
        String(d.getDate()).padStart(2, '0') + ' ' +
        String(d.getHours()).padStart(2, '0') + ':' +
        String(d.getMinutes()).padStart(2, '0') + ':' +
        String(d.getSeconds()).padStart(2, '0');
    },
  },
}
</script>

<style scoped>
/* ========== 页面整体 ========== */
.policy-page {
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

.stat-icon-dns {
  background: var(--warning-bg);
  color: var(--color-warning);
}

.stat-icon-bandwidth {
  background: var(--danger-bg);
  color: var(--color-danger);
}

.stat-icon-quota {
  background: var(--info-bg);
  color: var(--color-primary);
}

/* ========== 表格内样式 ========== */
.policy-name-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.policy-name {
  font-weight: 600;
  color: var(--text-primary);
  font-size: 13px;
}

.policy-note {
  font-size: 12px;
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 180px;
}

.bandwidth-badge {
  display: inline-block;
  padding: 2px 10px;
  background: var(--danger-bg);
  color: var(--color-danger);
  border-radius: 12px;
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
}

.bandwidth-unlimited {
  color: var(--text-placeholder);
  font-size: 12px;
}

.quota-badge {
  display: inline-block;
  padding: 2px 10px;
  background: var(--info-bg);
  color: var(--color-primary);
  border-radius: 12px;
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
}

.quota-reset {
  font-size: 11px;
  opacity: 0.7;
  margin-left: 2px;
}

.ref-cell {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  font-size: 12px;
  color: var(--text-regular);
}

.ref-item {
  display: flex;
  align-items: center;
  gap: 3px;
}

.ref-item i {
  font-size: 14px;
  color: var(--text-secondary);
}

.ref-item strong {
  color: var(--text-primary);
}

.ref-label {
  color: var(--text-secondary);
  font-size: 11px;
}

.ref-divider {
  color: var(--border-base);
}

/* 状态指示 */
.status-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 4px;
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

/* ========== 编辑弹窗 ========== */
.edit-basic-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 30px;
}

.edit-basic-row .form-item-compact {
  margin-bottom: 16px;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding-top: 8px;
}

::v-deep .policy-edit-dialog .el-dialog__body {
  padding: 20px 30px 10px;
}

/* 路由批量编辑 */
.route-count-hint {
  margin-top: 8px;
  padding: 8px 12px;
  background: var(--info-bg);
  border-radius: 6px;
  font-size: 12px;
  color: var(--text-secondary);
  display: flex;
  align-items: center;
  gap: 6px;
}

.route-count-hint i {
  color: var(--text-secondary);
}

.route-count-hint b {
  color: var(--color-primary);
}

/* ACL 批量编辑提示 */
.acl-edit-hint {
  margin-bottom: 14px;
  padding: 10px 14px;
  background: var(--info-bg);
  border: 1px solid var(--border-color);
  border-radius: 8px;
  font-size: 13px;
  color: var(--text-regular);
  line-height: 1.8;
  display: flex;
  align-items: flex-start;
  gap: 8px;
}

.acl-edit-hint i {
  color: var(--text-secondary);
  font-size: 15px;
  margin-top: 2px;
  flex-shrink: 0;
}

.acl-edit-hint code {
  background: #e6e8eb;
  padding: 1px 6px;
  border-radius: 4px;
  font-size: 12px;
  color: var(--text-primary);
}

/* ========== 应用弹窗 ========== */
.apply-dialog ::v-deep .el-dialog__body {
  padding: 20px 24px;
}

.apply-intro {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  background: var(--bg-hover);
  border-radius: 8px;
  margin-bottom: 16px;
  font-size: 13px;
  color: var(--text-regular);
}

.apply-intro i {
  color: var(--color-primary);
  font-size: 16px;
}

.apply-search {
  margin-bottom: 12px;
}

.apply-list {
  max-height: 380px;
  overflow-y: auto;
}

.apply-list-scroll {
  max-height: 360px;
}

.apply-item {
  padding: 8px 10px;
  border-radius: 6px;
  transition: background 0.15s;
}

.apply-item:hover {
  background: var(--bg-hover);
}

.apply-item ::v-deep .el-checkbox__label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.apply-item-name {
  font-weight: 500;
  color: var(--text-primary);
}

.apply-item-sub {
  font-size: 12px;
  color: var(--text-secondary);
}

.apply-empty {
  text-align: center;
  padding: 40px 20px;
  color: var(--text-placeholder);
}

.apply-empty i {
  font-size: 40px;
  display: block;
  margin-bottom: 10px;
}

.apply-empty p {
  font-size: 14px;
  margin: 0;
}

/* 响应式 */
@media (max-width: 900px) {
  .edit-basic-row {
    grid-template-columns: 1fr;
  }

  /* 编辑弹窗宽度随屏幕缩小 */
  .policy-edit-dialog ::v-deep .el-dialog {
    width: 95% !important;
    margin: 0 auto;
  }

  .policy-edit-dialog ::v-deep .el-dialog__body {
    padding: 16px 16px 8px;
  }

  /* 统计卡片改为3列 */
  .stats-row {
    grid-template-columns: repeat(3, 1fr) !important;
  }
}

@media (max-width: 600px) {

  /* 统计卡片改为2列 */
  .stats-row {
    grid-template-columns: repeat(2, 1fr) !important;
  }

  .stat-card {
    padding: 10px 8px;
  }

  .stat-icon {
    width: 32px;
    height: 32px;
    font-size: 16px;
  }

  .stat-value {
    font-size: 18px;
  }

  .stat-label {
    font-size: 11px;
  }
}

@media (max-width: 720px) {

  /* 编辑弹窗表单标签上置 */
  .policy-edit-dialog ::v-deep .el-form-item {
    display: block;
    margin-bottom: 14px;
  }

  .policy-edit-dialog ::v-deep .el-form-item__label {
    width: auto !important;
    text-align: left;
    padding-bottom: 4px;
    line-height: 1.4;
    float: none;
  }

  .policy-edit-dialog ::v-deep .el-form-item__content {
    margin-left: 0 !important;
    display: block;
  }

  .dialog-footer {
    flex-direction: column;
    gap: 8px;
  }

  .dialog-footer .el-button {
    width: 100%;
    margin-left: 0 !important;
  }

  /* 批量编辑弹窗 */
  .valgin-dialog ::v-deep .el-dialog {
    width: 95% !important;
  }

  .apply-dialog ::v-deep .el-dialog {
    width: 95% !important;
  }
}

@media (max-width: 520px) {
  .policy-edit-dialog ::v-deep .el-dialog {
    width: 98% !important;
  }

  .policy-edit-dialog ::v-deep .el-dialog__body {
    padding: 12px 10px 6px;
  }
}
</style>

<style>
/* 批量编辑弹窗：内容区可滚动，确保不会超出屏幕（非 scoped，dialog 渲染在 body 下） */
.valgin-dialog .el-dialog {
  max-height: calc(100vh - 120px);
  display: flex;
  flex-direction: column;
}

.valgin-dialog .el-dialog__header {
  flex-shrink: 0;
}

.valgin-dialog .el-dialog__body {
  flex: 1;
  overflow-y: auto;
  min-height: 0;
  padding-bottom: 10px;
}

.valgin-dialog .el-dialog__footer {
  flex-shrink: 0;
}
</style>
