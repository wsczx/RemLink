<template>
  <div class="webvpn-app">
    <el-card class="table-card" shadow="never" v-loading="loading">
      <div slot="header" class="card-header">
        <span class="card-title"><i class="el-icon-connection"></i> WebVPN 应用</span>
        <div class="card-actions">
          <el-input v-model="searchName" placeholder="搜索应用名称 / 子域名" size="small" prefix-icon="el-icon-search" clearable
            class="search-input" @keyup.enter.native="handleSearch" @clear="handleSearch" />
          <el-button size="small" icon="el-icon-search" @click="handleSearch">查询</el-button>
          <el-button size="small" type="primary" icon="el-icon-plus" @click="handleEdit('')">
            添加应用
          </el-button>
        </div>
      </div>

      <el-table :data="tableData" stripe border style="width: 100%"
        :header-cell-style="{ background: 'var(--bg-header)', color: 'var(--text-primary)', fontWeight: '600', fontSize: '13px' }">
        <el-table-column prop="id" label="ID" width="70" align="center" sortable />
        <el-table-column prop="name" label="子域名" min-width="140" show-overflow-tooltip sortable>
          <template slot-scope="scope">
            <span class="text-primary">{{ scope.row.name }}</span>
            <span class="text-muted">.{{ webvpnDomain }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="note" label="名称" min-width="140" show-overflow-tooltip sortable />
        <el-table-column prop="backend" label="后端地址" min-width="200" show-overflow-tooltip sortable />
        <el-table-column label="授权用户" min-width="150" show-overflow-tooltip>
          <template slot-scope="scope">
            <template v-if="scope.row.users && scope.row.users.length">
              <el-tag v-for="u in scope.row.users" :key="u" size="mini" effect="plain" class="group-tag">{{ u
                }}</el-tag>
            </template>
            <span v-else class="text-muted">全部用户</span>
          </template>
        </el-table-column>
        <el-table-column label="授权用户组" min-width="150" show-overflow-tooltip>
          <template slot-scope="scope">
            <template v-if="scope.row.groups && scope.row.groups.length">
              <el-tag v-for="g in scope.row.groups" :key="g" size="mini" effect="plain" class="group-tag">{{ g
                }}</el-tag>
            </template>
            <span v-else class="text-muted">全部用户</span>
          </template>
        </el-table-column>
        <el-table-column prop="host_rewrite" label="Host 改写" min-width="140" show-overflow-tooltip sortable>
          <template slot-scope="scope">
            <span v-if="scope.row.host_rewrite" class="text-primary">{{ scope.row.host_rewrite }}</span>
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90" align="center">
          <template slot-scope="scope">
            <el-tag :type="scope.row.status === 1 ? 'success' : 'info'" size="small" effect="plain">
              {{ scope.row.status === 1 ? '启用' : '禁用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="170" align="center" fixed="right">
          <template slot-scope="scope">
            <div class="col-ops">
              <el-button type="text" size="mini" @click="handleEdit(scope.row)">编辑</el-button>
              <el-button type="text" size="mini" @click="handleStatus(scope.row)">
                {{ scope.row.status === 1 ? '禁用' : '启用' }}
              </el-button>
              <el-button type="text" size="mini" class="text-danger" @click="handleDelete(scope.row)">删除</el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination-wrap" v-if="total > pageSize">
        <el-pagination background layout="total, prev, pager, next, jumper" :total="total" :page-size="pageSize"
          :current-page="currentPage" @current-change="handlePageChange" />
      </div>
    </el-card>

    <!-- 新增/编辑对话框 -->
    <el-dialog :title="form.id ? '编辑 WebVPN 应用' : '添加 WebVPN 应用'" :visible.sync="dialogVisible" width="640px"
      @closed="resetForm">
      <el-form ref="form" :model="form" :rules="rules" label-width="100px" size="small">
        <el-form-item label="子域名" prop="name">
          <el-input v-model="form.name" placeholder="如 oa，将生成 oa.wv.example.com">
            <template slot="append">.{{ webvpnDomain }}</template>
          </el-input>
          <div class="form-tip">子域名前缀，唯一标识该应用，仅含字母/数字/短横线。</div>
        </el-form-item>
        <el-form-item label="应用名称" prop="note">
          <el-input v-model="form.note" placeholder="如 OA 系统（门户页展示用）" />
        </el-form-item>
        <el-form-item label="后端地址" prop="backend">
          <el-input v-model="form.backend" placeholder="http://10.0.0.5:8080 或 https://..." />
          <div class="form-tip">内网 Web 应用地址；HTTPS 且自签证书时勾选下方跳过校验。</div>
        </el-form-item>
        <el-form-item label="跳过证书校验" prop="skip_verify">
          <el-switch v-model="form.skip_verify" />
          <span class="form-tip-inline">后端使用自签/内网证书时开启（不校验对端证书）</span>
        </el-form-item>
        <el-form-item label="Host 改写">
          <el-input v-model="form.host_rewrite" placeholder="可选，如 backend.internal（虚拟主机后端需要）" />
        </el-form-item>
        <el-form-item label="授权用户">
          <el-select v-model="form.users" multiple filterable allow-create default-first-option
            placeholder="留空 = 不限制到具体用户" style="width: 100%">
            <el-option v-for="u in allUsers" :key="u" :label="u" :value="u" />
          </el-select>
          <div class="form-tip">用户白名单；留空 + 组留空 = 所有登录用户可访问。</div>
        </el-form-item>
        <el-form-item label="授权用户组">
          <el-select v-model="form.groups" multiple filterable allow-create default-first-option
            placeholder="留空 = 允许全部登录用户" style="width: 100%">
            <el-option v-for="g in allGroups" :key="g" :label="g" :value="g" />
          </el-select>
        </el-form-item>
        <el-form-item label="允许路径">
          <el-select v-model="form.allow_path" multiple filterable allow-create default-first-option
            placeholder="留空 = 全部路径" style="width: 100%">
            <el-option v-for="p in form.allow_path" :key="p" :label="p" :value="p" />
          </el-select>
          <div class="form-tip">路径前缀白名单，如 /api/，仅允许访问匹配前缀。</div>
        </el-form-item>
        <el-form-item label="来源 IP">
          <el-select v-model="form.ip_allow_list" multiple filterable allow-create default-first-option
            placeholder="留空 = 不限制来源" style="width: 100%">
            <el-option v-for="ip in form.ip_allow_list" :key="ip" :label="ip" :value="ip" />
          </el-select>
          <div class="form-tip">客户端公网 IP/CIDR 白名单，限制可访问的出口。</div>
        </el-form-item>
        <el-form-item label="跨站访问">
          <el-switch v-model="form.allowCrossSite" />
          <div class="form-tip">仅在需要跨域页面、接口或 iframe 访问时启用；关闭时同源访问不受影响。</div>
        </el-form-item>
        <el-form-item label="允许跨域来源">
          <el-input v-model="form.corsOriginsText" type="textarea" :rows="3"
            placeholder="每行一个完整地址，如 https://oa.mv.example.com" />
          <div class="form-tip">跨站访问开启后，仅允许精确匹配的 http/https Origin；留空或关闭开关 = 仅同源请求。</div>
        </el-form-item>
        <el-form-item label="状态">
          <el-switch v-model="form.statusOn" active-text="启用" inactive-text="禁用" />
        </el-form-item>
      </el-form>
      <div slot="footer">
        <el-button size="small" @click="dialogVisible = false">取消</el-button>
        <el-button size="small" type="primary" :loading="saving" @click="submitForm">保存</el-button>
      </div>
    </el-dialog>
  </div>
</template>

<script>
import axios from 'axios'

export default {
  name: 'WebVpnAppList',
  data() {
    return {
      loading: false,
      saving: false,
      searchName: '',
      tableData: [],
      currentPage: 1,
      pageSize: 10,
      total: 0,
      webvpnDomain: '',
      allGroups: [],
      allUsers: [],
      dialogVisible: false,
      form: this.emptyForm(),
      rules: {
        name: [
          { required: true, message: '请输入子域名', trigger: 'blur' },
          { pattern: /^[a-zA-Z0-9-]+$/, message: '仅允许字母、数字、短横线', trigger: 'blur' }
        ],
        backend: [
          { required: true, message: '请输入后端地址', trigger: 'blur' }
        ],
        note: [
          { required: true, message: '请输入应用名称', trigger: 'blur' }
        ]
      }
    }
  },
  created() {
    this.$emit('update:route_path', this.$route.path)
    this.$emit('update:route_name', ['WebVPN', 'WebVPN 应用'])
    this.loadDomain()
    this.loadGroups()
    this.loadUsers()
    this.getList()
  },
  methods: {
    emptyForm() {
      return {
        id: 0,
        name: '',
        note: '',
        backend: '',
        host_rewrite: '',
        skip_verify: false,
        users: [],
        groups: [],
        allow_path: [],
        ip_allow_list: [],
        allowCrossSite: false,
        corsOriginsText: '',
        statusOn: true
      }
    },
    resetForm() {
      this.form = this.emptyForm()
      if (this.$refs.form) this.$refs.form.clearValidate()
    },
    loadDomain() {
      axios.get('/webvpn/domain').then(resp => {
        if (resp.data.code === 0) this.webvpnDomain = resp.data.data.domain || ''
      }).catch(() => { })
    },
    loadGroups() {
      axios.get('/group/list?page_size=200').then(resp => {
        if (resp.data.code === 0) {
          this.allGroups = (resp.data.data.datas || []).map(g => g.name)
        }
      }).catch(() => { })
    },
    loadUsers() {
      axios.get('/user/list?page_size=200').then(resp => {
        if (resp.data.code === 0) {
          this.allUsers = (resp.data.data.datas || []).map(u => u.username)
        }
      }).catch(() => { })
    },
    getList() {
      this.loading = true
      const params = {
        page: this.currentPage,
        page_size: this.pageSize,
        name: this.searchName
      }
      axios.get('/webvpn/app/list', { params }).then(resp => {
        if (resp.data.code === 0) {
          const d = resp.data.data || {}
          this.tableData = d.datas || []
          this.total = d.count || 0
        } else {
          this.$message.error(resp.data.msg || '获取列表失败')
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
    handlePageChange(page) {
      this.currentPage = page
      this.getList()
    },
    handleEdit(row) {
      if (row) {
        this.form = {
          id: row.id,
          name: row.name,
          note: row.note,
          backend: row.backend,
          host_rewrite: row.host_rewrite || '',
          skip_verify: !!row.skip_verify,
          users: row.users || [],
          groups: row.groups || [],
          allow_path: row.allow_path || [],
          ip_allow_list: row.ip_allow_list || [],
          allowCrossSite: !!row.allow_cross_site,
          corsOriginsText: (row.cors_allowed_origins || []).join('\n'),
          statusOn: row.status === 1
        }
      } else {
        this.resetForm()
      }
      this.dialogVisible = true
    },
    handleStatus(row) {
      const next = row.status === 1 ? 0 : 1
      const payload = {
        id: row.id,
        name: row.name,
        note: row.note,
        backend: row.backend,
        host_rewrite: row.host_rewrite || '',
        skip_verify: !!row.skip_verify,
        users: row.users || [],
        groups: row.groups || [],
        allow_path: row.allow_path || [],
        ip_allow_list: row.ip_allow_list || [],
        allow_cross_site: !!row.allow_cross_site,
        cors_allowed_origins: this.parseCorsOrigins(row.cors_allowed_origins),
        status: next
      }
      axios.post('/webvpn/app/set', payload).then(resp => {
        if (resp.data.code === 0) {
          this.$message.success(next === 1 ? '已启用' : '已禁用')
          this.getList()
        } else {
          this.$message.error(resp.data.msg || '操作失败')
        }
      }).catch(() => this.$message.error('网络错误'))
    },
    handleDelete(row) {
      this.$confirm(`确认删除应用「${row.note || row.name}」？`, '提示', {
        type: 'warning'
      }).then(() => {
        axios.post('/webvpn/app/del', { id: row.id }).then(resp => {
          if (resp.data.code === 0) {
            this.$message.success('已删除')
            if (this.tableData.length === 1 && this.currentPage > 1) {
              this.currentPage--
            }
            this.getList()
          } else {
            this.$message.error(resp.data.msg || '删除失败')
          }
        }).catch(() => this.$message.error('网络错误'))
      }).catch(() => { })
    },
    parseCorsOrigins(value) {
      const text = Array.isArray(value) ? value.join('\n') : (value || '')
      return text.split(/[\n,]+/).map(item => item.trim()).filter(Boolean)
    },
    submitForm() {
      this.$refs.form.validate(valid => {
        if (!valid) return
        this.saving = true
        const payload = {
          id: this.form.id,
          name: this.form.name,
          note: this.form.note,
          backend: this.form.backend,
          host_rewrite: this.form.host_rewrite,
          skip_verify: this.form.skip_verify,
          users: this.form.users,
          groups: this.form.groups,
          allow_path: this.form.allow_path,
          ip_allow_list: this.form.ip_allow_list,
          allow_cross_site: this.form.allowCrossSite,
          cors_allowed_origins: this.parseCorsOrigins(this.form.corsOriginsText),
          status: this.form.statusOn ? 1 : 0
        }
        axios.post('/webvpn/app/set', payload).then(resp => {
          if (resp.data.code === 0) {
            this.$message.success('已保存')
            this.dialogVisible = false
            this.getList()
          } else {
            this.$message.error(resp.data.msg || '保存失败')
          }
        }).catch(() => this.$message.error('网络错误')).finally(() => {
          this.saving = false
        })
      })
    }
  }
}
</script>

<style scoped>
.group-tag {
  margin: 0 4px 4px 0;
}

.form-tip {
  color: var(--text-muted);
  font-size: 12px;
  line-height: 1.5;
  margin-top: 4px;
}

.form-tip-inline {
  color: var(--text-muted);
  font-size: 12px;
  margin-left: 8px;
}
</style>