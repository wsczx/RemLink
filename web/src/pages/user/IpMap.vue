<template>
  <div class="ipmap-page">
    <!-- 统计卡片 -->
    <div class="stats-row">
      <div class="stat-card">
        <div class="stat-icon stat-icon-total">
          <i class="el-icon-connection"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ statTotal }}</div>
          <div class="stat-label">映射总数</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-icon-keep">
          <i class="el-icon-circle-check"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ statKeep }}</div>
          <div class="stat-label">IP保留</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-icon-mac">
          <i class="el-icon-document"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ statUniqueMac }}</div>
          <div class="stat-label">唯一MAC</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-icon-user">
          <i class="el-icon-s-custom"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ statBoundUser }}</div>
          <div class="stat-label">已绑定用户</div>
        </div>
      </div>
    </div>

    <!-- IP映射表格 -->
    <el-card class="table-card" shadow="never" v-loading="loading">
      <div slot="header" class="card-header">
        <span class="card-title"><i class="el-icon-connection"></i> IP映射列表</span>
        <div class="card-actions">
          <el-button v-if="multipleSelection.length" size="small" type="danger" icon="el-icon-delete"
            @click="handleBatchDel">批量删除 ({{ multipleSelection.length }})</el-button>
          <el-button size="small" type="primary" icon="el-icon-plus" @click="handleEdit('')">
            添加映射
          </el-button>
        </div>
      </div>

      <div class="filter-bar">
        <el-input v-model="filters.ip" placeholder="IP地址" size="mini" clearable class="filter-item"
          @keyup.enter.native="onSearch" @clear="onSearch"></el-input>
        <el-input v-model="filters.mac" placeholder="MAC地址" size="mini" clearable class="filter-item"
          @keyup.enter.native="onSearch" @clear="onSearch"></el-input>
        <el-input v-model="filters.username" placeholder="姓名或用户名" size="mini" clearable class="filter-item"
          @keyup.enter.native="onSearch" @clear="onSearch"></el-input>
        <el-input v-model="filters.group" placeholder="组/出口" size="mini" clearable class="filter-item"
          @keyup.enter.native="onSearch" @clear="onSearch"></el-input>
        <el-select v-model="filters.keep" placeholder="保留" size="mini" clearable class="filter-item">
          <el-option label="已保留" value="1"></el-option>
          <el-option label="未保留" value="0"></el-option>
        </el-select>
        <el-button size="mini" type="primary" icon="el-icon-search" @click="onSearch">查询</el-button>
        <el-button size="mini" icon="el-icon-refresh" @click="resetFilters">重置</el-button>
      </div>

      <div class="ipmap-table-wrap">
        <el-table ref="multipleTable" :data="tableData" stripe highlight-current-row border style="width:100%"
          @selection-change="handleSelectionChange"
          :header-cell-style="{ background: '#fafafa', color: '#303133', fontWeight: '600', fontSize: '13px' }">
          <el-table-column type="selection" width="45" align="center"></el-table-column>
          <el-table-column sortable prop="id" label="ID" width="65" align="center"></el-table-column>
          <el-table-column prop="ip_addr" label="IP地址" width="180" show-overflow-tooltip sortable>
            <template slot-scope="scope">
              <span class="ip-addr">{{ scope.row.ip_addr }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="mac_addr" label="MAC地址" width="160" show-overflow-tooltip sortable></el-table-column>
          <el-table-column prop="unique_mac" label="唯一MAC" width="85" align="center" sortable>
            <template slot-scope="scope">
              <el-tag v-if="scope.row.unique_mac" type="success" size="mini" effect="plain">是</el-tag>
              <el-tag v-else type="info" size="mini" effect="plain">否</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="group" label="组/出口" width="120" sortable>
            <template slot-scope="scope">
              <span v-if="scope.row.group" class="group-tag">{{ scope.row.group }}</span>
              <span v-else class="text-muted">全局</span>
            </template>
          </el-table-column>
          <el-table-column prop="username" label="用户名" min-width="120" show-overflow-tooltip sortable>
            <template slot-scope="scope">
              <span v-if="scope.row.username" class="bound-user">{{ userLabel(scope.row.username, scope.row.nickname)
              }}</span>
              <span v-else class="text-muted">未绑定</span>
            </template>
          </el-table-column>
          <el-table-column prop="keep" label="IP保留" width="85" align="center" sortable>
            <template slot-scope="scope">
              <el-switch disabled v-model="scope.row.keep" active-color="#13ce66" />
            </template>
          </el-table-column>
          <el-table-column prop="note" label="备注" min-width="120" show-overflow-tooltip sortable></el-table-column>
          <el-table-column prop="last_login" label="最后登录" :formatter="tableDateFormat" width="165"
            sortable></el-table-column>
          <el-table-column label="操作" width="110" class-name="col-ops" min-width="110" align="center">
            <template slot-scope="scope">
              <el-dropdown trigger="click" @command="(cmd) => handleRowCmd(scope.row, cmd)">
                <el-button size="mini" class="action-more-btn">
                  操作<i class="el-icon-arrow-down el-icon--right"></i>
                </el-button>
                <el-dropdown-menu slot="dropdown">
                  <el-dropdown-item command="edit" icon="el-icon-edit">编辑映射</el-dropdown-item>
                  <el-dropdown-item command="delete" icon="el-icon-delete" divided
                    class="dropdown-danger">删除映射</el-dropdown-item>
                </el-dropdown-menu>
              </el-dropdown>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <div class="pagination-wrap">
        <el-pagination background layout="prev,pager,next" :pager-count="9" @current-change="pageChange"
          :total="count" />
      </div>
    </el-card>

    <!-- 编辑弹窗 -->
    <el-dialog :close-on-click-modal="false" :title="ruleForm.id ? '编辑 IP 映射' : '新增 IP 映射'"
      :visible.sync="user_edit_dialog" width="580px" top="6vh" @close="disVisible" class="ipmap-edit-dialog">
      <el-form :model="ruleForm" :rules="rules" ref="ruleForm" label-width="100px">
        <el-form-item label="ID" prop="id" v-if="ruleForm.id">
          <el-input v-model="ruleForm.id" disabled></el-input>
        </el-form-item>
        <el-form-item label="IP地址" prop="ip_addr">
          <el-input v-model="ruleForm.ip_addr" placeholder="请输入IP地址，如 192.168.1.100"></el-input>
        </el-form-item>
        <el-form-item label="MAC地址" prop="mac_addr">
          <el-input v-model="ruleForm.mac_addr" placeholder="请输入MAC地址，如 00:11:22:33:44:55"></el-input>
        </el-form-item>
        <el-form-item label="用户名" prop="username">
          <el-input v-model="ruleForm.username" placeholder="选填，绑定用户名"></el-input>
        </el-form-item>
        <el-form-item label="备注" prop="note">
          <el-input v-model="ruleForm.note" placeholder="备注信息"></el-input>
        </el-form-item>
        <el-form-item label="IP保留" prop="keep">
          <el-switch v-model="ruleForm.keep" active-color="#13ce66"></el-switch>
          <span class="form-tip">开启后该IP将保留不分配给其他设备</span>
        </el-form-item>
        <el-divider></el-divider>
        <div class="dialog-footer">
          <el-button @click="disVisible">取消</el-button>
          <el-button type="primary" icon="el-icon-check" @click="submitForm('ruleForm')">保存映射</el-button>
        </div>
      </el-form>
    </el-dialog>
  </div>
</template>

<script>
import axios from "axios";
import userLabel from "@/mixins/userLabel";

export default {
  name: "IpMap",
  mixins: [userLabel],
  created() {
    this.$emit('update:route_path', this.$route.path)
    this.$emit('update:route_name', ['用户管理', 'IP映射'])
  },
  mounted() { this.getData(1); },
  data() {
    return {
      loading: false,
      tableData: [], count: 0,
      filters: { ip: '', mac: '', username: '', group: '', keep: '' },
      multipleSelection: [],
      ruleForm: { status: 1, groups: [] },
      rules: {
        mac_addr: [{ required: true, message: '请输入MAC地址', trigger: 'blur' }],
        ip_addr: [{ required: true, message: '请输入IP地址', trigger: 'blur' }],
        username: [{ max: 50, message: '长度不超过 50 个字符', trigger: 'blur' }],
        status: [{ required: true }],
      },
    }
  },
  computed: {
    statTotal() { return this.count },
    statKeep() { return this.tableData.filter(r => r.keep).length },
    statUniqueMac() { return new Set(this.tableData.map(r => r.mac_addr)).size },
    statBoundUser() { return new Set(this.tableData.filter(r => r.username).map(r => r.username)).size },
  },
  methods: {
    handleRowCmd(row, cmd) {
      switch (cmd) {
        case 'edit': this.handleEdit(row); break;
        case 'delete':
          this.$confirm('确定要删除该IP映射吗？', '删除确认', {
            confirmButtonText: '确定删除', cancelButtonText: '取消',
            type: 'warning', confirmButtonClass: 'el-button--danger',
          }).then(() => this.handleDel(row)).catch(() => { });
          break;
      }
    },
    tableDateFormat(row, col, val) {
      if (!val) return '';
      const d = new Date(val);
      return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' +
        String(d.getDate()).padStart(2, '0') + ' ' + String(d.getHours()).padStart(2, '0') + ':' +
        String(d.getMinutes()).padStart(2, '0') + ':' + String(d.getSeconds()).padStart(2, '0');
    },
    getData(p) {
      this.loading = true;
      const params = { page: p };
      if (this.filters.ip) params.ip = this.filters.ip;
      if (this.filters.mac) params.mac = this.filters.mac;
      if (this.filters.username) params.username = this.filters.username;
      if (this.filters.group) params.group = this.filters.group;
      if (this.filters.keep !== '') params.keep = this.filters.keep;
      axios.get('/user/ip_map/list', { params }).then(resp => {
        const data = resp.data.data || {};
        this.tableData = data.datas || [];
        this.count = data.count || 0;
      }).catch(() => {
        this.$message.error('请求出错');
      }).finally(() => {
        this.loading = false;
      });
    },
    onSearch() { this.getData(1); },
    resetFilters() {
      this.filters = { ip: '', mac: '', username: '', group: '', keep: '' };
      this.getData(1);
    },
    handleSelectionChange(val) { this.multipleSelection = val; },
    handleBatchDel() {
      if (this.multipleSelection.length === 0) return;
      const ids = this.multipleSelection.map(r => r.id);
      this.$confirm(`确定要批量删除选中的 ${ids.length} 条 IP 映射吗？删除后不可恢复。`, '批量删除确认', {
        confirmButtonText: '确定删除', cancelButtonText: '取消',
        type: 'warning', confirmButtonClass: 'el-button--danger',
      }).then(() => {
        axios.post('/user/ip_map/batch_del', { ids }).then(resp => {
          if (resp.data.code === 0) {
            this.$message.success(resp.data.msg);
            this.multipleSelection = [];
            this.getData(1);
          } else {
            this.$message.error(resp.data.msg);
          }
        }).catch(() => {
          this.$message.error('请求出错');
        });
      }).catch(() => { });
    },
    pageChange(p) { this.getData(p); },
    handleEdit(row) {
      this.$refs['ruleForm'] && this.$refs['ruleForm'].resetFields();
      this.user_edit_dialog = true;
      if (!row) {
        this.ruleForm = { status: 1, groups: [] };
        return;
      }
      axios.get('/user/ip_map/detail', { params: { id: row.id } }).then(resp => {
        this.ruleForm = resp.data.data;
      }).catch(() => {
        this.$message.error('请求出错');
      });
    },
    handleDel(row) {
      axios.post('/user/ip_map/del?id=' + row.id).then(resp => {
        if (resp.data.code === 0) {
          this.$message.success(resp.data.msg);
          this.getData(1);
        } else { this.$message.error(resp.data.msg); }
      }).catch(() => {
        this.$message.error('请求出错');
      });
    },
    submitForm(formName) {
      this.$refs[formName].validate((valid) => {
        if (!valid) return false;
        axios.post('/user/ip_map/set', this.ruleForm).then(resp => {
          if (resp.data.code === 0) {
            this.$message.success(resp.data.msg);
            this.getData(1);
            this.user_edit_dialog = false;
          } else { this.$message.error(resp.data.msg); }
        }).catch(() => {
          this.$message.error('请求出错');
        });
      });
    },
  },
}
</script>

<style scoped>
/* ========== 页面整体 ========== */
.ipmap-page {
  padding: 4px 0;
}

/* ========== 统计卡片 ========== */
.stat-icon-total {
  background: var(--color-primary-bg);
  color: var(--color-primary);
}

.stat-icon-keep {
  background: #f0f9eb;
  color: #67c23a;
}

.stat-icon-mac {
  background: #fdf6ec;
  color: #e6a23c;
}

.stat-icon-user {
  background: #ecf5ff;
  color: #409eff;
}

/* 表格卡片（全局已提供） */

/* 表格内 */
.ip-addr {
  font-weight: 600;
  color: #303133;
  font-size: 13px;
  font-family: monospace;
}

.group-tag {
  font-weight: 500;
  color: #409eff;
  font-size: 12px;
  background: #ecf5ff;
  padding: 2px 8px;
  border-radius: 4px;
}

.bound-user {
  font-weight: 500;
  color: #303133;
}

.text-muted {
  color: #c0c4cc;
  font-size: 12px;
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
  border-color: #c6e2ff;
  background: var(--color-primary-bg);
}

.dropdown-danger {
  color: #f56c6c !important;
}

.pagination-wrap {
  display: flex;
  justify-content: flex-end;
  padding-top: 16px;
}

/* ========== 编辑弹窗 ========== */
.ipmap-edit-dialog ::v-deep .el-dialog__body {
  padding: 16px 24px 10px;
}

.form-tip {
  font-size: 12px;
  color: #909399;
  margin-left: 8px;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding-top: 8px;
}

/* 表格滚动容器 */
.ipmap-table-wrap {
  overflow-x: auto;
  width: 100%;
}

/* 筛选栏 */
.filter-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  padding: 12px 14px;
  margin-bottom: 14px;
  background: var(--bg-card);
  border: 1px solid var(--border-color-light);
  border-radius: 8px;
}

.filter-bar .filter-item {
  width: 150px;
}

@media (max-width: 600px) {
  .filter-bar .filter-item {
    width: calc(50% - 4px);
  }
}

/* 响应式 */
@media (max-width: 880px) {
  .ipmap-table-wrap ::v-deep .col-ops {
    min-width: 180px;
  }
}

@media (max-width: 600px) {
  .ipmap-table-wrap ::v-deep .col-ops {
    min-width: 140px;
  }
}
</style>
