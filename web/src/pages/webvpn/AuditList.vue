<template>
  <div class="webvpn-audit">
    <el-card class="table-card" shadow="never" v-loading="loading">
      <div slot="header" class="card-header">
        <span class="card-title"><i class="el-icon-document"></i> WebVPN 访问审计</span>
        <div class="card-actions">
          <el-input v-model="filters.username" placeholder="用户名" size="small" clearable class="search-input" />
          <el-input v-model="filters.app_name" placeholder="应用名" size="small" clearable class="search-input" />
          <el-date-picker v-model="dateRange" type="daterange" size="small" range-separator="至" start-placeholder="开始日期"
            end-placeholder="结束日期" value-format="yyyy-MM-dd" style="width: 240px" />
          <el-button size="small" icon="el-icon-search" type="primary" @click="handleSearch">查询</el-button>
          <el-button size="small" icon="el-icon-refresh" @click="handleReset">重置</el-button>
          <el-button size="small" icon="el-icon-download" @click="handleExport">导出 CSV</el-button>
        </div>
      </div>

      <el-table :data="tableData" stripe border style="width: 100%"
        :header-cell-style="{ background: 'var(--bg-header)', color: 'var(--text-primary)', fontWeight: '600', fontSize: '13px' }">
        <el-table-column prop="id" label="ID" width="70" align="center" sortable />
        <el-table-column prop="username" label="用户" width="120" show-overflow-tooltip sortable>
          <template slot-scope="scope">{{ scope.row.username || '-' }}</template>
        </el-table-column>
        <el-table-column prop="group_name" label="用户组" width="110" show-overflow-tooltip sortable />
        <el-table-column prop="app_name" label="应用" width="120" show-overflow-tooltip sortable />
        <el-table-column prop="method" label="方法" width="80" align="center" sortable>
          <template slot-scope="scope">
            <el-tag size="mini" effect="plain">{{ scope.row.method }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="path" label="路径" min-width="220" show-overflow-tooltip sortable />
        <el-table-column prop="status_code" label="状态码" width="90" align="center" sortable>
          <template slot-scope="scope">
            <el-tag :type="scope.row.status_code >= 400 ? 'danger' : 'success'" size="mini" effect="plain">
              {{ scope.row.status_code }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="risk_level" label="风险" width="90" align="center" sortable>
          <template slot-scope="scope">
            <el-tag
              :type="scope.row.risk_level === 2 ? 'danger' : scope.row.risk_level === 1 ? 'warning' : 'info'"
              size="mini" effect="plain">
              {{ scope.row.risk_level === 2 ? '高危' : scope.row.risk_level === 1 ? '可疑' : '正常' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="bytes_sent" label="字节数" width="110" align="right" sortable>
          <template slot-scope="scope">{{ formatBytes(scope.row.bytes_sent) }}</template>
        </el-table-column>
        <el-table-column prop="client_ip" label="客户端 IP" width="140" sortable />
        <el-table-column prop="created_at" label="时间" width="170" sortable />
        <el-table-column label="操作" width="100" align="center" fixed="right">
          <template slot-scope="scope">
            <div class="col-ops">
              <el-button type="text" size="mini" @click="handleKick(scope.row)">踢出会话</el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination-wrap" v-if="total > pageSize">
        <el-pagination background layout="total, prev, pager, next, jumper" :total="total" :page-size="pageSize"
          :current-page="currentPage" @current-change="handlePageChange" />
      </div>
    </el-card>
  </div>
</template>

<script>
import axios from 'axios'

export default {
  name: 'WebVpnAuditList',
  data() {
    return {
      loading: false,
      tableData: [],
      currentPage: 1,
      pageSize: 15,
      total: 0,
      dateRange: '',
      filters: {
        username: '',
        app_name: ''
      }
    }
  },
  created() {
    this.$emit('update:route_path', this.$route.path)
    this.$emit('update:route_name', ['WebVPN', 'WebVPN 审计'])
    this.getList()
  },
  methods: {
    getList() {
      this.loading = true
      const params = {
        page: this.currentPage,
        page_size: this.pageSize,
        username: this.filters.username,
        app_name: this.filters.app_name
      }
      if (this.dateRange && this.dateRange.length === 2) {
        params.start_date = this.dateRange[0] + ' 00:00:00'
        params.end_date = this.dateRange[1] + ' 23:59:59'
      }
      axios.get('/webvpn/audit/list', { params }).then(resp => {
        if (resp.data.code === 0) {
          const d = resp.data.data || {}
          this.tableData = d.datas || []
          this.total = d.count || 0
        } else {
          this.$message.error(resp.data.msg || '获取审计失败')
        }
      }).catch(() => {
        this.$message.error('网络错误')
      }).finally(() => {
        this.loading = false
      })
    },
    handleSearch() {
      this.currentPage = 1
      this.getList()
    },
    handleReset() {
      this.filters = { username: '', app_name: '' }
      this.dateRange = ''
      this.currentPage = 1
      this.getList()
    },
    handlePageChange(page) {
      this.currentPage = page
      this.getList()
    },
    handleExport() {
      const params = {
        username: this.filters.username,
        app_name: this.filters.app_name
      }
      if (this.dateRange && this.dateRange.length === 2) {
        params.start_date = this.dateRange[0] + ' 00:00:00'
        params.end_date = this.dateRange[1] + ' 23:59:59'
      }
      const query = Object.keys(params)
        .filter(k => params[k] !== '' && params[k] !== undefined && params[k] !== null)
        .map(k => `${encodeURIComponent(k)}=${encodeURIComponent(params[k])}`)
        .join('&')
      window.location.href = '/webvpn/audit/export' + (query ? '?' + query : '')
    },
    handleKick(row) {
      this.$confirm(`确认踢出用户「${row.username}」的全部 WebVPN 会话？`, '提示', {
        type: 'warning'
      }).then(() => {
        axios.post('/webvpn/session/kick', { username: row.username }).then(resp => {
          if (resp.data.code === 0) {
            this.$message.success('已踢出该用户的所有会话')
            this.getList()
          } else {
            this.$message.error(resp.data.msg || '操作失败')
          }
        }).catch(() => this.$message.error('网络错误'))
      }).catch(() => { })
    },
    formatBytes(n) {
      n = Number(n) || 0
      if (n < 1024) return n + ' B'
      if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB'
      return (n / 1024 / 1024).toFixed(1) + ' MB'
    }
  }
}
</script>

<style scoped>
/* 平板及更窄（含横屏 pad）：筛选行横向换行，字段自适应拉伸，
   避免日期选择器等固定宽度控件溢出挤掉其它控件 */
@media (max-width: 1024px) {
  /* 标题与筛选栏上下排列，避免横屏下标题和一堆按钮挤在同一行 */
  .card-header {
    flex-direction: column;
    align-items: stretch;
    gap: 10px;
  }

  .card-actions {
    flex-direction: row;
    flex-wrap: wrap;
    align-items: center;
    width: 100%;
    gap: 8px;
  }

  .card-actions .search-input,
  .card-actions .el-date-editor {
    flex: 1 1 140px;
    width: auto !important;
    min-width: 0;
  }

  .card-actions .el-button {
    margin-left: 0;
  }
}

/* 手机端：分页条隐藏冗余项并允许横向滚动，避免末页「下一页」被裁切 */
@media (max-width: 600px) {
  .pagination-wrap {
    justify-content: flex-start;
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
  }

  .pagination-wrap ::v-deep .el-pagination {
    white-space: nowrap;
  }

  .pagination-wrap ::v-deep .el-pagination__total,
  .pagination-wrap ::v-deep .el-pagination__jump {
    display: none;
  }
}
</style>