<template>
  <div class="online-page">
    <!-- 统计卡片 -->
    <div class="stats-row">
      <div class="stat-card">
        <div class="stat-icon stat-icon-online">
          <i class="el-icon-user-solid"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ statOnline }}</div>
          <div class="stat-label">当前在线</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-icon-mobile">
          <i class="el-icon-mobile-phone"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ statMobile }}</div>
          <div class="stat-label">移动端</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-icon-desktop">
          <i class="el-icon-s-platform"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ statDesktop }}</div>
          <div class="stat-label">桌面端</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-icon-sleeper">
          <i class="el-icon-moon"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ statSleeper }}</div>
          <div class="stat-label">休眠用户</div>
        </div>
      </div>
    </div>

    <!-- 在线用户表格 -->
    <el-card class="table-card" shadow="never" v-loading="loading">
      <div slot="header" class="card-header">
        <span class="card-title"><i class="el-icon-user-solid"></i> 在线用户列表</span>
        <div class="card-actions">
          <span class="auto-refresh-tip">每 10 秒自动刷新</span>
          <el-select v-model="searchCate" size="small" style="width:110px" @change="handleSearch">
            <el-option label="用户名" value="username"></el-option>
            <el-option label="登录组" value="group"></el-option>
            <el-option label="MAC地址" value="mac_addr"></el-option>
            <el-option label="IP地址" value="ip"></el-option>
            <el-option label="远端地址" value="remote_addr"></el-option>
          </el-select>
          <el-input v-model="searchText" placeholder="搜索..." size="small" prefix-icon="el-icon-search" clearable
            class="search-input" @input="handleSearch" @clear="handleSearch" />
          <span class="toggle-label">显示休眠：</span>
          <el-switch v-model="showSleeper" size="small" @change="handleSearch" />
        </div>
      </div>

      <div class="online-table-wrap">
        <el-table ref="multipleTable" :data="tableData" stripe highlight-current-row border style="width:100%"
          :header-cell-style="{ background: '#fafafa', color: '#303133', fontWeight: '600', fontSize: '13px' }">
          <el-table-column sortable type="index" label="#" width="50" align="center"></el-table-column>
          <el-table-column prop="username" label="用户名" width="120" sortable>
            <template slot-scope="scope">
              <span class="online-username">{{ userLabel(scope.row.username, scope.row.nickname) }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="group" label="登录组" width="100" align="center" sortable>
            <template slot-scope="scope">
              <el-tag v-if="scope.row.group" size="mini" effect="plain">{{ scope.row.group }}</el-tag>
              <span v-else class="text-muted">-</span>
            </template>
          </el-table-column>
          <el-table-column prop="mac_addr" label="MAC地址" width="150" show-overflow-tooltip sortable></el-table-column>
          <el-table-column prop="unique_mac" label="唯一MAC" width="85" align="center" sortable>
            <template slot-scope="scope">
              <el-tag v-if="scope.row.unique_mac" type="success" size="mini" effect="plain">是</el-tag>
              <el-tag v-else type="info" size="mini" effect="plain">否</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="ip" label="IP地址" width="140" sortable></el-table-column>
          <el-table-column prop="remote_addr" label="远端地址" width="140" show-overflow-tooltip sortable></el-table-column>
          <el-table-column prop="transport_protocol" label="传输协议" width="90" align="center" sortable>
            <template slot-scope="scope">
              <el-tag size="mini" effect="plain" type="info">{{ scope.row.transport_protocol || '-' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="客户端" width="80" align="center">
            <template slot-scope="scope">
              <i v-if="scope.row.client === 'mobile'" class="el-icon-mobile-phone client-mobile" title="移动端"></i>
              <i v-else class="el-icon-s-platform client-desktop" title="桌面端"></i>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="90" align="center">
            <template slot-scope="scope">
              <el-tag v-if="scope.row.is_active" type="success" size="mini" effect="dark">在线</el-tag>
              <el-tag v-else type="info" size="mini" effect="plain">休眠</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="实时上行/下行" width="180" align="center">
            <template slot-scope="scope">
              <span class="bw-up">{{ scope.row.bandwidth_up }}</span>
              <span class="bw-divider">/</span>
              <span class="bw-down">{{ scope.row.bandwidth_down }}</span>
            </template>
          </el-table-column>
          <el-table-column label="总量上行/下行" width="190" align="center">
            <template slot-scope="scope">
              <span class="bw-total-up">{{ scope.row.bandwidth_up_all }}</span>
              <span class="bw-divider">/</span>
              <span class="bw-total-down">{{ scope.row.bandwidth_down_all }}</span>
            </template>
          </el-table-column>
          <el-table-column label="流量配额" width="160" align="center">
            <template slot-scope="scope">
              <span v-if="scope.row.traffic_quota" class="quota-cell">
                <span class="quota-used">{{ scope.row.traffic_used }}</span>
                <span class="quota-divider">/</span>
                <span class="quota-total">{{ scope.row.traffic_quota }}</span>
                <span class="quota-reset" v-if="scope.row.traffic_reset">{{ resetLabel(scope.row.traffic_reset)
                }}</span>
              </span>
              <span v-else class="text-muted">不限</span>
            </template>
          </el-table-column>
          <el-table-column prop="last_login" label="登录时间" :formatter="tableDateFormat" min-width="165"
            sortable></el-table-column>
          <el-table-column label="操作" width="120" class-name="col-ops" min-width="120" align="center">
            <template slot-scope="scope">
              <el-dropdown trigger="click" @command="(cmd) => handleRowCmd(scope.row, cmd)">
                <el-button size="mini" class="action-more-btn">
                  操作<i class="el-icon-arrow-down el-icon--right"></i>
                </el-button>
                <el-dropdown-menu slot="dropdown">
                  <el-dropdown-item command="reline" icon="el-icon-refresh"
                    :disabled="!scope.row.is_active">重连</el-dropdown-item>
                  <el-dropdown-item command="offline" icon="el-icon-switch-button" divided
                    :disabled="!scope.row.is_active" class="dropdown-danger">下线</el-dropdown-item>
                </el-dropdown-menu>
              </el-dropdown>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </el-card>
  </div>
</template>

<script>
import axios from "axios";
import userLabel from "@/mixins/userLabel";

export default {
  name: "Online",
  mixins: [userLabel],
  created() {
    this.$emit('update:route_path', this.$route.path)
    this.$emit('update:route_name', ['用户管理', '在线用户'])
  },
  mounted() {
    this.getData();
    const timer = setInterval(() => this.getData(), 10000);
    this.$once('hook:beforeDestroy', () => clearInterval(timer));
  },
  data() {
    return {
      loading: true,
      tableData: [],
      searchCate: 'username',
      searchText: '',
      showSleeper: false,
    }
  },
  computed: {
    // 当前在线：仅统计真实在线（is_active=true）的会话
    statOnline() { return this.tableData.filter(r => r.is_active).length },
    // 全部展示行数（含休眠态）
    statTotal() { return this.tableData.length },
    statMobile() { return this.tableData.filter(r => r.is_active && r.client === 'mobile').length },
    statDesktop() { return this.tableData.filter(r => r.is_active && r.client !== 'mobile').length },
    // 休眠用户：已掉线但仍在等待清理/重连
    statSleeper() { return this.tableData.filter(r => !r.is_active).length },
  },
  methods: {
    handleRowCmd(row, cmd) {
      switch (cmd) {
        case 'reline':
          axios.post('/user/reline?token=' + row.token).then(resp => {
            if (resp.data.code === 0) { this.$message.success(resp.data.msg); this.getData(); }
            else { this.$message.error(resp.data.msg); }
          }).catch(() => { this.$message.error('请求出错'); });
          break;
        case 'offline':
          this.$confirm('确定要将该用户下线吗？', '下线确认', {
            confirmButtonText: '确定', cancelButtonText: '取消',
            type: 'warning', confirmButtonClass: 'el-button--danger',
          }).then(() => {
            axios.post('/user/offline?token=' + row.token).then(resp => {
              if (resp.data.code === 0) { this.$message.success(resp.data.msg); this.getData(); }
              else { this.$message.error(resp.data.msg); }
            }).catch(() => { this.$message.error('请求出错'); });
          });
          break;
      }
    },
    handleSearch() { this.getData(); },
    tableDateFormat(row, col, val) {
      if (!val) return '';
      const d = new Date(val);
      return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' +
        String(d.getDate()).padStart(2, '0') + ' ' + String(d.getHours()).padStart(2, '0') + ':' +
        String(d.getMinutes()).padStart(2, '0') + ':' + String(d.getSeconds()).padStart(2, '0');
    },
    resetLabel(period) {
      switch (period) {
        case 'daily': return '/日';
        case 'weekly': return '/周';
        case 'monthly': return '/月';
        default: return '';
      }
    },
    getData() {
      axios.get('/user/online', {
        params: { search_cate: this.searchCate, search_text: this.searchText, show_sleeper: this.showSleeper, one_offline: false }
      }).then(resp => {
        this.tableData = resp.data.data.datas || [];
        this.loading = false;
      }).catch(() => { this.$message.error('请求出错'); this.loading = false; });
    },
  },
}
</script>

<style scoped>
/* ========== 页面整体 ========== */
.online-page {
  padding: 4px 0;
}

/* ========== 统计卡片 ========== */
.stat-icon-online {
  background: var(--color-primary-bg);
  color: var(--color-primary);
}

.stat-icon-mobile {
  background: var(--warning-bg);
  color: var(--color-warning);
}

.stat-icon-desktop {
  background: var(--success-bg);
  color: var(--color-success);
}

.stat-icon-sleeper {
  background: var(--info-bg);
  color: var(--color-info);
}

/* 页面特有 */
.auto-refresh-tip {
  font-size: 11px;
  color: var(--text-placeholder);
  margin-right: 4px;
}

.search-input {
  width: 170px;
}

.toggle-label {
  font-size: 12px;
  color: var(--text-secondary);
}

/* 表格内 */
.online-username {
  font-weight: 600;
  color: var(--text-primary);
  font-size: 13px;
}

.text-muted {
  color: var(--text-placeholder);
  font-size: 12px;
}

.client-mobile {
  font-size: 20px;
  color: var(--color-danger);
}

.client-desktop {
  font-size: 20px;
  color: var(--color-primary);
}

.bw-up,
.bw-down {
  font-size: 12px;
  font-weight: 500;
}

.bw-up {
  color: var(--color-success);
}

.bw-down {
  color: var(--text-secondary);
}

.bw-total-up,
.bw-total-down {
  font-size: 12px;
  font-weight: 600;
}

.bw-total-up {
  color: var(--color-success);
}

.bw-total-down {
  color: var(--text-regular);
}

.bw-divider {
  color: var(--border-base);
  margin: 0 4px;
  font-size: 11px;
}

.quota-cell {
  display: inline-flex;
  align-items: center;
  gap: 2px;
}

.quota-used {
  color: var(--color-primary);
  font-weight: 600;
  font-size: 12px;
}

.quota-total {
  color: var(--text-secondary);
  font-size: 12px;
}

.quota-reset {
  color: var(--text-placeholder);
  font-size: 11px;
  margin-left: 2px;
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

/* 表格滚动容器 */
.online-table-wrap {
  overflow-x: auto;
  width: 100%;
}

/* 响应式 */
@media (max-width: 1100px) {
  .online-table-wrap ::v-deep .col-ops {
    min-width: 200px;
  }
}

@media (max-width: 1024px) {
  .card-header {
    flex-direction: column;
    align-items: stretch;
    flex-wrap: wrap;
    gap: 8px;
  }

  .card-actions {
    flex-wrap: wrap;
    width: 100%;
    gap: 8px;
  }

  .card-actions .search-input,
  .card-actions .el-select,
  .card-actions .filter-select {
    flex: 1 1 140px;
    width: auto !important;
    min-width: 0;
    margin-left: 0;
  }

  .auto-refresh-tip {
    display: none;
  }

  .toggle-label {
    font-size: 11px;
  }
}

@media (max-width: 600px) {
  .online-table-wrap ::v-deep .col-ops {
    min-width: 140px;
  }
}
</style>
