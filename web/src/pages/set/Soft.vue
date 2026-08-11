<template>
  <el-card class="soft-page">
    <div class="page-head">
      <div>
        <div class="page-title">软件配置</div>
        <div class="page-subtitle">服务运行参数、客户端 Profile 与重启操作</div>
      </div>
      <el-button type="warning" size="small" icon="el-icon-refresh" :loading="restart_loading"
        @click="restartService">重启服务</el-button>
    </div>

    <div class="notice-stack">
      <el-alert v-for="(w, idx) in system_warnings" :key="'warn-' + idx" :title="warningMessage(w)"
        :type="warningLevel(w)" show-icon :closable="false" style="margin-top:8px">
      </el-alert>
    </div>

    <div class="summary-row">
      <div class="summary-item">
        <div class="summary-icon-wrapper summary-icon-group"><i class="el-icon-folder-opened"></i></div>
        <span class="summary-label">配置分组</span>
        <strong>{{ group_count }}</strong>
      </div>
      <div class="summary-item">
        <div class="summary-icon-wrapper summary-icon-item"><i class="el-icon-s-operation"></i></div>
        <span class="summary-label">配置项</span>
        <strong>{{ filtered_soft_data.length }}</strong>
      </div>
      <div class="summary-item">
        <div class="summary-icon-wrapper summary-icon-restart"><i class="el-icon-refresh"></i></div>
        <span class="summary-label">重启生效</span>
        <strong>{{ restart_total }}</strong>
      </div>
      <div class="summary-item">
        <div class="summary-icon-wrapper summary-icon-immediate"><i class="el-icon-check"></i></div>
        <span class="summary-label">立即生效</span>
        <strong>{{ immediate_total }}</strong>
      </div>
    </div>

    <el-tabs v-model="active_tab" class="soft-tabs">
      <el-tab-pane name="config">
        <span slot="label"><i class="el-icon-setting"></i> 服务配置</span>
        <div class="toolbar">
          <div class="toolbar-left">
            <el-input v-model="search_keyword" clearable size="small" prefix-icon="el-icon-search"
              placeholder="搜索配置名或说明" class="search-input">
            </el-input>
            <el-select v-model="effect_filter" size="small" class="effect-select">
              <el-option label="全部配置" value="all"></el-option>
              <el-option label="重启生效" value="restart"></el-option>
              <el-option label="立即生效" value="immediate"></el-option>
            </el-select>
          </div>
          <div class="toolbar-right">
            <el-tag size="mini" type="info">{{ filtered_soft_data.length }} 项</el-tag>
          </div>
        </div>

        <!-- 分组展示 -->
        <div v-if="grouped_data.length === 0" class="empty-hint">
          <i class="el-icon-info"></i> 没有匹配的配置项
        </div>
        <el-collapse v-else v-model="active_groups" class="group-collapse">
          <el-collapse-item v-for="grp in grouped_data" :key="grp.name" :name="grp.name">
            <template slot="title">
              <div class="group-title">
                <span class="group-name">{{ grp.name }}</span>
                <span class="group-count">{{ grp.items.length }} 项</span>
              </div>
            </template>
            <div class="group-items">
              <!-- IPv4 网络配置合并卡片 -->
              <div v-if="grp.name === '虚拟网络'" class="ipv4-config-card">
                <div class="ipv4-config-grid">
                  <div class="ipv4-config-item">
                    <label>网段</label>
                    <el-input v-model="ipv4_edit.ipv4_cidr" size="mini" placeholder="如 192.168.90.0/24"></el-input>
                  </div>
                  <div class="ipv4-config-item">
                    <label>网关</label>
                    <el-input v-model="ipv4_edit.ipv4_gateway" size="mini" placeholder="如 192.168.90.1"></el-input>
                  </div>
                  <div class="ipv4-config-item">
                    <label>起始地址</label>
                    <el-input v-model="ipv4_edit.ipv4_start" size="mini" placeholder="如 192.168.90.100"></el-input>
                  </div>
                  <div class="ipv4-config-item">
                    <label>结束地址</label>
                    <el-input v-model="ipv4_edit.ipv4_end" size="mini" placeholder="如 192.168.90.200"></el-input>
                  </div>
                </div>
                <div class="ipv4-config-footer">
                  <el-tag size="mini" type="warning">重启生效</el-tag>
                  <el-button type="primary" size="mini" :loading="ipv4_saving" @click="saveIPv4Config">保存</el-button>
                </div>
              </div>
              <!-- 其他配置项（跳过 IPv4 四个字段） -->
              <div
                v-for="item in grp.items.filter(i => ['ipv4_cidr', 'ipv4_gateway', 'ipv4_start', 'ipv4_end'].indexOf(i.name) < 0)"
                :key="item.name" class="config-item" :class="{ 'config-item-full': item.multiline }">
                <div class="config-item-info">
                  <div class="config-item-usage">{{ item.usage }}</div>
                  <div class="config-item-name">{{ item.name }}</div>
                </div>
                <div class="config-item-value">
                  <el-switch v-if="item.type === 'bool'" v-model="item.edit_data"
                    :disabled="item.readonly || item.saving">
                  </el-switch>
                  <el-input-number v-else-if="item.type === 'int'" v-model="item.edit_data"
                    :disabled="item.readonly || item.saving" size="mini" controls-position="right">
                  </el-input-number>
                  <el-input v-else-if="item.multiline" v-model="item.edit_data" type="textarea" :rows="6"
                    :disabled="item.readonly || item.saving" size="mini" placeholder="多个用逗号隔开或者每行一个,支持单IP和CIDR范围">
                  </el-input>
                  <div v-else-if="item.options && Object.keys(item.options).length > 2" style="width:100%">
                    <el-select v-model="item.edit_data" :disabled="item.readonly || item.saving" size="mini"
                      style="width:100%">
                      <el-option v-for="(val, label) in item.options" :key="val" :label="label" :value="val">
                      </el-option>
                    </el-select>
                    <div v-if="item.edit_data && !Object.values(item.options).includes(item.edit_data)"
                      class="config-item-hint">
                      主网卡设置错误，请选择正确的物理网卡
                    </div>
                  </div>
                  <el-radio-group v-else-if="item.options" v-model="item.edit_data"
                    :disabled="item.readonly || item.saving" size="mini" style="display:flex;width:100%">
                    <el-radio-button v-for="(val, label) in item.options" :key="val" :label="val"
                      style="flex:1;text-align:center">
                      {{ label }}
                    </el-radio-button>
                  </el-radio-group>
                  <el-input v-else v-model="item.edit_data" :type="item.sensitive ? 'password' : 'text'"
                    :placeholder="item.sensitive ? '留空表示不修改' : ''" :disabled="item.readonly || item.saving" size="mini">
                    <i v-if="item.readonly" slot="prefix" class="el-icon-lock"
                      style="color:var(--text-placeholder)"></i>
                  </el-input>
                </div>
                <div class="config-item-effect">
                  <el-tag v-if="item.restart" size="mini" type="warning">重启</el-tag>
                  <el-tag v-else size="mini" type="success">立即</el-tag>
                </div>
                <div class="config-item-action">
                  <template v-if="item.readonly && item.name === 'db_type'">
                    <el-button type="warning" size="mini" icon="el-icon-connection"
                      @click="openDbSwitchWizard">切换</el-button>
                  </template>
                  <template v-else-if="item.readonly && item.name === 'db_source'">
                    <span></span>
                  </template>
                  <template v-else>
                    <el-button type="primary" size="mini" :loading="item.saving"
                      @click="saveConfig(item)">保存</el-button>
                  </template>
                </div>
              </div>
            </div>
          </el-collapse-item>
        </el-collapse>
      </el-tab-pane>
      <el-tab-pane name="profile">
        <span slot="label"><i class="el-icon-document"></i> Profile</span>
        <div class="profile-toolbar">
          <div>
            <div class="section-title">客户端 Profile</div>
            <div class="hash-line">SHA1 {{ profile_hash }}</div>
          </div>
          <el-button type="primary" size="small" icon="el-icon-check" :loading="profile_saving" @click="saveProfile">保存
            Profile</el-button>
        </div>
        <el-input type="textarea" v-model="profile_content" :rows="24" spellcheck="false">
        </el-input>
      </el-tab-pane>
      <el-tab-pane name="backup">
        <span slot="label"><i class="el-icon-upload2"></i> 数据备份</span>
        <div class="toolbar">
          <div class="toolbar-left">
            <el-button type="primary" size="small" icon="el-icon-plus" @click="showCreateBackup">创建备份</el-button>
          </div>
          <div class="toolbar-right">
            <el-tag size="mini" type="info">{{ backups.length }} 个备份</el-tag>
          </div>
        </div>
        <div v-if="backups.length === 0" class="empty-hint">
          <i class="el-icon-info"></i> 暂无备份文件，点击「创建备份」按钮进行备份
        </div>
        <el-table v-else :data="backups" size="small" class="backup-table">
          <el-table-column prop="name" label="文件名" min-width="280" sortable>
            <el-table-column prop="type" label="类型" width="80" sortable>
              <template slot-scope="scope">
                <el-tag v-if="scope.row.type === 'full'" size="mini" type="primary">全量</el-tag>
                <el-tag v-else size="mini">配置</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="大小" width="100">
              <template slot-scope="scope">
                {{ formatSize(scope.row.size) }}
              </template>
            </el-table-column>
            <el-table-column prop="mod_time" label="时间" width="160" sortable>
            </el-table-column>
            <el-table-column label="操作" width="160">
              <template slot-scope="scope">
                <el-button type="text" size="mini" @click="restoreBackup(scope.row)">还原</el-button>
                <el-button type="text" size="mini" style="color:var(--color-danger)"
                  @click="deleteBackup(scope.row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>

    <!-- 创建备份弹出框 -->
    <el-dialog title="创建备份" :visible.sync="backup_dialog" width="580px" :close-on-click-modal="false" append-to-body>
      <el-form label-width="90px" size="small">
        <el-form-item label="备份类型">
          <el-radio-group v-model="backup_type">
            <el-radio label="config">仅配置备份（软件设置项）</el-radio>
            <el-radio label="full">全量备份（业务数据 + 配置）</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item v-if="backup_type === 'full'">
          <el-alert title="以下表数据量可能非常大，备份前请确认。默认已排除，如需要可勾选包含。" type="warning" :closable="false" show-icon
            style="margin-bottom:12px">
          </el-alert>
          <div class="table-check-group">
            <div v-for="tbl in table_sizes" :key="tbl.table" class="table-check-item">
              <el-checkbox v-model="tbl.checked" :disabled="tbl.group === 'business'" @change="onTableCheckChange">
                <span class="check-label">{{ tbl.name }}</span>
                <span class="check-name">({{ tbl.table }})</span>
              </el-checkbox>
              <span class="check-count-wrapper">
                <el-tag size="mini"
                  :type="tbl.group === 'business' ? 'success' : tbl.group === 'log' ? 'danger' : 'warning'">
                  {{ formatNumber(tbl.rows) }} 行
                </el-tag>
              </span>
            </div>
          </div>
        </el-form-item>
      </el-form>
      <span slot="footer">
        <el-button size="small" @click="backup_dialog = false">取消</el-button>
        <el-button type="primary" size="small" :loading="backup_saving" @click="execBackup">确认备份</el-button>
      </span>
    </el-dialog>

    <!-- 数据库切换向导 -->
    <el-dialog title="数据库切换向导" :visible.sync="db_switch_dialog" width="620px" @close="resetDbSwitchWizard"
      :close-on-click-modal="false" append-to-body>
      <!-- 步骤条 -->
      <el-steps :active="db_switch_step" align-center finish-status="success" style="margin-bottom:20px">
        <el-step title="配置连接" icon="el-icon-edit"></el-step>
        <el-step title="测试连通" icon="el-icon-link"></el-step>
        <el-step title="数据迁移" icon="el-icon-upload2"></el-step>
        <el-step title="确认执行" icon="el-icon-check"></el-step>
      </el-steps>

      <!-- 步骤内容 -->
      <div class="db-switch-body">
        <!-- Step 0：当前配置 + 新连接信息 -->
        <div v-show="db_switch_step === 0">
          <el-alert title="当前数据库连接信息（只读）" type="info" :closable="false" show-icon style="margin-bottom:16px">
            <template slot="default">
              <p style="margin:4px 0">类型：<code>{{ currentDbType }}</code></p>
              <p style="margin:0">连接：<code style="word-break:break-all">{{ currentDbSource }}</code></p>
            </template>
          </el-alert>
          <el-form label-width="100px" size="small">
            <el-form-item label="新数据库类型">
              <el-select v-model="db_switch_form.db_type" placeholder="选择数据库类型" style="width:100%">
                <el-option label="SQLite3" value="sqlite3"></el-option>
                <el-option label="MySQL" value="mysql"></el-option>
                <el-option label="PostgreSQL" value="postgres"></el-option>
                <el-option label="MS SQL Server" value="mssql"></el-option>
              </el-select>
            </el-form-item>
            <el-form-item label="新连接字符串">
              <el-input v-model="db_switch_form.db_source"
                placeholder="如 ./conf/remlink_new.db 或 user:pass@tcp(host:3306)/dbname">
              </el-input>
              <div class="form-tip">SQLite 填写文件路径；MySQL/Postgres/MSSQL 填写 DSN 连接串</div>
            </el-form-item>
          </el-form>
        </div>

        <!-- Step 1：测试连接 -->
        <div v-show="db_switch_step === 1">
          <div class="db-switch-test-section">
            <div class="test-result-box" v-if="db_switch_test_result">
              <el-alert :title="db_switch_test_result" :type="db_switch_test_ok ? 'success' : 'error'" :closable="false"
                show-icon>
              </el-alert>
            </div>
            <el-button type="primary" :loading="db_switch_testing" @click="testDbConnection" style="width:100%">
              {{ db_switch_testing ? '连接测试中...' : '测试连接' }}
            </el-button>
            <div class="form-tip" style="text-align:center;margin-top:8px">
              系统将使用新参数尝试连接数据库，不会修改任何配置
            </div>
          </div>
        </div>

        <!-- Step 2：选择迁移方式 -->
        <div v-show="db_switch_step === 2">
          <el-radio-group v-model="db_switch_form.migration" style="width:100%">
            <div class="migration-option" @click="db_switch_form.migration = 'auto_migrate'">
              <el-radio label="auto_migrate" class="migration-radio"></el-radio>
              <div class="migration-content">
                <div class="migration-title">
                  <el-tag size="mini" type="primary">推荐</el-tag>
                  <strong>自动迁移数据</strong>
                </div>
                <div class="migration-desc">
                  先备份当前数据库，然后将数据迁移到新数据库。迁移完成后自动写入配置并重启服务。
                </div>
              </div>
            </div>
            <div class="migration-option" @click="db_switch_form.migration = 'backup_only'">
              <el-radio label="backup_only" class="migration-radio"></el-radio>
              <div class="migration-content">
                <div class="migration-title"><strong>仅备份不迁移</strong></div>
                <div class="migration-desc">
                  创建当前数据库的全量备份文件，然后切换到新数据库启动。新库将从空数据库开始运行，备份文件可按需手动还原。
                </div>
              </div>
            </div>
            <div class="migration-option" @click="db_switch_form.migration = 'none'">
              <el-radio label="none" class="migration-radio"></el-radio>
              <div class="migration-content">
                <div class="migration-title"><strong>跳过备份</strong></div>
                <div class="migration-desc">
                  不做任何备份和迁移，直接切换到新数据库。新库将以空数据库启动（会触发自动初始化）。
                </div>
              </div>
            </div>
          </el-radio-group>
        </div>

        <!-- Step 3：确认执行 -->
        <div v-show="db_switch_step === 3">
          <el-alert title="即将执行数据库切换，请确认以下信息：" type="warning" :closable="false" show-icon style="margin-bottom:16px">
          </el-alert>
          <el-form label-width="100px" size="small" class="confirm-form">
            <el-form-item label="新数据库类型">
              <code>{{ db_switch_form.db_type }}</code>
            </el-form-item>
            <el-form-item label="新连接字符串">
              <code style="word-break:break-all">{{ db_switch_form.db_source }}</code>
            </el-form-item>
            <el-form-item label="数据迁移方式">
              <el-tag size="small" v-if="db_switch_form.migration === 'auto_migrate'" type="primary">自动迁移数据</el-tag>
              <el-tag size="small" v-else-if="db_switch_form.migration === 'backup_only'" type="warning">仅备份不迁移</el-tag>
              <el-tag size="small" v-else type="danger">跳过备份</el-tag>
            </el-form-item>
          </el-form>
          <el-alert title="重要提醒" type="error" :closable="false" show-icon style="margin-bottom:8px">
            <ul class="alert-list">
              <li>切换后服务将<strong>自动重启</strong>，所有在线连接会中断</li>
              <li>请勿删除 <code>conf/db.json</code> 文件，否则重启后会回到默认 SQLite</li>
              <li>备份文件在 <code>conf/backup/</code> 目录，切换失败可手动处理</li>
            </ul>
          </el-alert>
        </div>
      </div>

      <span slot="footer">
        <el-button v-if="db_switch_step > 0" size="small" @click="db_switch_step--">上一步</el-button>
        <el-button v-if="db_switch_step < 3" size="small" type="primary" @click="dbSwitchNextStep"
          :disabled="dbSwitchNextDisabled">
          {{ db_switch_step === 0 ? '下一步' : db_switch_step === 1 ? '选择迁移方式' : '确认信息' }}
        </el-button>
        <el-button v-if="db_switch_step === 3" size="small" type="primary" :loading="db_switch_executing"
          @click="execDbSwitch">
          确认切换
        </el-button>
        <el-button size="small" @click="db_switch_dialog = false">取消</el-button>
      </span>
    </el-dialog>
    <!-- 数据库切换后提示手动还原 -->
    <el-dialog title="还原数据" :visible.sync="restore_prompt_dialog" width="460px" append-to-body>
      <p style="font-size:14px;line-height:1.8;">
        数据库已切换到新库，系统启动时检测到备份文件。是否需要从备份文件还原数据？
      </p>
      <span slot="footer">
        <el-button size="small" @click="restore_prompt_dialog = false">知道了</el-button>
        <el-button size="small" type="primary" @click="goToRestore">去还原</el-button>
      </span>
    </el-dialog>
  </el-card>
</template>

<script>
import axios from "axios";

export default {
  name: "Soft",
  created() {
    this.$emit('update:route_path', this.$route.path)
    this.$emit('update:route_name', ['系统设置', '软件配置'])
  },
  mounted() {
    this.getSoftInfo()
    this.getSoftStatus()
    this.getProfile()
    this.checkDbSwitchRestore()
  },
  data() {
    return {
      active_tab: 'config',
      active_groups: [],
      soft_data: [],
      ipv4_edit: { ipv4_cidr: '', ipv4_gateway: '', ipv4_start: '', ipv4_end: '' },
      ipv4_saving: false,
      search_keyword: '',
      effect_filter: 'all',
      system_warnings: [],
      profile_content: '',
      profile_hash: '',
      profile_saving: false,
      restart_loading: false,
      // 备份相关
      backups: [],
      backup_dialog: false,
      backup_type: 'full',
      backup_saving: false,
      table_sizes: [],
      // 数据库切换向导
      db_switch_dialog: false,
      db_switch_step: 0,
      db_switch_testing: false,
      db_switch_test_ok: false,
      db_switch_test_result: '',
      db_switch_executing: false,
      db_switch_form: {
        db_type: '',
        db_source: '',
        migration: 'auto_migrate',
      },
      // 切换后提示手动还原
      restore_prompt_dialog: false,
    }
  },

  computed: {
    group_count() {
      var groups = new Set()
      this.soft_data.forEach(item => {
        if (item.group) groups.add(item.group)
      })
      return groups.size
    },
    restart_total() {
      return this.soft_data.filter(item => item.restart).length
    },
    immediate_total() {
      return this.soft_data.filter(item => !item.restart).length
    },
    filtered_soft_data() {
      var keyword = this.search_keyword.trim().toLowerCase()
      return this.soft_data.filter(item => {
        if (this.effect_filter === 'restart' && !item.restart) {
          return false
        }
        if (this.effect_filter === 'immediate' && item.restart) {
          return false
        }
        if (!keyword) {
          return true
        }
        return [item.name, item.usage].some(val => {
          return String(val || '').toLowerCase().indexOf(keyword) !== -1
        })
      })
    },
    grouped_data() {
      var groups = {}
      this.filtered_soft_data.forEach(item => {
        var g = item.group || '其他'
        if (!groups[g]) {
          groups[g] = []
        }
        groups[g].push(item)
      })
      // 按固定顺序排列分组
      var order = ['基础信息', '服务监听', '数据库', '虚拟网络', '连接控制', '日志/调试', '安全/认证', '防暴破', '锁定策略', '门户设置']
      var result = []
      order.forEach(name => {
        if (groups[name]) {
          result.push({ name: name, items: groups[name] })
          delete groups[name]
        }
      })
      // 剩余分组按字母排序
      Object.keys(groups).sort().forEach(name => {
        result.push({ name: name, items: groups[name] })
      })
      return result
    },
    // 当前数据库配置（从已加载的 soft_data 中提取）
    currentDbType() {
      var item = this.soft_data.find(function (i) { return i.name === 'db_type' })
      return item ? item.data : ''
    },
    currentDbSource() {
      var item = this.soft_data.find(function (i) { return i.name === 'db_source' })
      return item ? item.data : ''
    },
    // 数据库切换向导-下一步按钮是否禁用
    dbSwitchNextDisabled() {
      if (this.db_switch_step === 0) {
        return !this.db_switch_form.db_type || !this.db_switch_form.db_source
      }
      if (this.db_switch_step === 1) {
        return !this.db_switch_test_ok
      }
      // step 2: migration always has default value
      return false
    },
  },

  watch: {
    grouped_data: {
      immediate: true,
      handler(val) {
        // 默认展开所有分组
        if (val.length > 0 && this.active_groups.length === 0) {
          this.active_groups = val.map(g => g.name)
        }
      }
    },
    active_tab(val) {
      if (val === 'backup') {
        this.loadBackups()
      }
    },
  },

  methods: {
    // 提取后端返回的错误信息，避免调试时丢失具体原因
    showApiError(err, fallback) {
      var msg = ''
      try {
        msg = err.response && err.response.data && err.response.data.msg
      } catch (e) { /* ignore */ }
      this.$message.error(msg || fallback || '请求出错')
    },
    getSoftInfo() {
      axios.get('/set/soft', {}).then(resp => {
        var data = resp.data
        this.soft_data = data.data.map(item => {
          item.edit_data = item.sensitive ? '' : item.data
          item.saving = false
          return item
        });
        // 同步 IPv4 字段到合并编辑区
        var ipv4Names = ['ipv4_cidr', 'ipv4_gateway', 'ipv4_start', 'ipv4_end']
        this.soft_data.forEach(function (item) {
          if (ipv4Names.indexOf(item.name) >= 0) {
            this.ipv4_edit[item.name] = item.data
          }
        }.bind(this))
      }).catch((err) => {
        this.showApiError(err, '哦，请求出错');
      });
    },
    getSoftStatus() {
      axios.get('/set/soft/status', {}).then(resp => {
        var data = resp.data.data
        this.system_warnings = data.warnings || []
      }).catch((err) => {
        this.showApiError(err, '状态获取失败');
      });
    },
    warningMessage(w) {
      return typeof w === 'string' ? w : (w && w.message) || ''
    },
    warningLevel(w) {
      return typeof w === 'string' ? 'error' : (w && w.level) || 'error'
    },
    getProfile() {
      axios.get('/set/profile', {}).then(resp => {
        var data = resp.data.data
        this.profile_content = data.content
        this.profile_hash = data.hash
      }).catch((err) => {
        this.showApiError(err, 'Profile 获取失败');
      });
    },
    saveProfile() {
      if (!this.profile_content) {
        this.$message.warning('Profile 内容不能为空')
        return
      }
      this.profile_saving = true
      axios.post('/set/profile/edit', {
        content: this.profile_content,
      }).then(resp => {
        this.profile_hash = resp.data.data.hash
        this.$message.success('Profile 已保存')
      }).catch((err) => {
        this.showApiError(err, 'Profile 保存失败');
      }).finally(() => {
        this.profile_saving = false
      })
    },
    restartService() {
      this.$confirm('服务将重启，当前连接会短暂中断。', '重启服务', {
        confirmButtonText: '重启',
        cancelButtonText: '取消',
        type: 'warning'
      }).then(() => {
        this.restart_loading = true
        axios.post('/set/restart').then(() => {
          this.$message.success('重启指令已发送，页面将自动刷新')
          this.waitForRestart()
        }).catch((err) => {
          this.restart_loading = false
          this.showApiError(err, '重启失败');
        })
      }).catch(() => { /* 用户取消 */ })
    },
    saveConfig(row) {
      if (row.sensitive && !row.edit_data) {
        this.$message.warning('敏感配置为空，不会保存')
        return
      }
      row.saving = true
      axios.post('/set/soft/edit', {
        name: row.name,
        data: row.edit_data,
      }).then(resp => {
        if (resp.data.code !== 0) {
          this.$message.error(resp.data.msg || '保存失败')
          return
        }
        var result = resp.data.data
        if (result.restart) {
          this.$message.warning('保存成功，重启服务后生效')
        } else {
          this.$message.success('保存成功')
        }
        this.getSoftInfo()
        this.getSoftStatus()
      }).catch((err) => {
        this.showApiError(err, '保存失败');
      }).finally(() => {
        row.saving = false
      })
    },
    saveIPv4Config() {
      this.ipv4_saving = true
      axios.post('/set/soft/ipv4', this.ipv4_edit).then(resp => {
        if (resp.data.code !== 0) {
          this.$message.error(resp.data.msg || '保存失败')
          return
        }
        var result = resp.data.data
        if (result.restart) {
          this.$message.warning('IPv4 网络配置保存成功，重启服务后生效')
        } else {
          this.$message.success('IPv4 网络配置保存成功')
        }
        this.getSoftInfo()
        this.getSoftStatus()
      }).catch((err) => {
        this.showApiError(err, '保存失败');
      }).finally(() => {
        this.ipv4_saving = false
      })
    },
    // ========== 数据备份 ==========
    loadBackups() {
      axios.get('/set/db/backups').then(resp => {
        this.backups = resp.data.data || []
      }).catch((err) => {
        this.showApiError(err, '获取备份列表失败')
      })
    },
    loadTableSizes() {
      return axios.get('/set/db/table_sizes').then(resp => {
        var list = resp.data.data || []
        // 默认业务表选中，日志/统计表未选中
        this.table_sizes = list.map(function (t) {
          return {
            table: t.table,
            name: t.name,
            rows: t.rows,
            group: t.group,
            checked: t.group === 'business'
          }
        })
      }).catch((err) => {
        this.showApiError(err, '获取表信息失败')
      })
    },
    getCheckedTables() {
      return this.table_sizes.filter(function (t) { return t.checked }).map(function (t) { return t.table })
    },
    onTableCheckChange() {
      // 全量备份时动态计算选中表
    },
    showCreateBackup() {
      this.backup_type = 'full'
      this.loadTableSizes().then(() => {
        this.backup_dialog = true
      })
    },
    execBackup() {
      this.backup_saving = true
      var payload = { type: this.backup_type }
      if (this.backup_type === 'full') {
        payload.include_tables = this.getCheckedTables()
      }
      axios.post('/set/db/backup', payload).then(() => {
        this.$message.success('备份创建成功')
        this.backup_dialog = false
        this.loadBackups()
      }).catch((err) => {
        this.showApiError(err, '备份创建失败')
      }).finally(() => {
        this.backup_saving = false
      })
    },
    // ========== 数据库切换向导 ==========
    openDbSwitchWizard() {
      // 预填当前数据库信息
      this.db_switch_form.db_type = this.currentDbType
      this.db_switch_form.db_source = this.currentDbSource
      this.db_switch_form.migration = 'auto_migrate'
      this.resetDbSwitchState()
      this.db_switch_dialog = true
    },
    resetDbSwitchState() {
      this.db_switch_step = 0
      this.db_switch_testing = false
      this.db_switch_test_ok = false
      this.db_switch_test_result = ''
      this.db_switch_executing = false
    },
    resetDbSwitchWizard() {
      this.resetDbSwitchState()
      this.db_switch_form.db_type = ''
      this.db_switch_form.db_source = ''
      this.db_switch_form.migration = 'auto_migrate'
    },
    testDbConnection() {
      if (!this.db_switch_form.db_type || !this.db_switch_form.db_source) {
        this.$message.warning('请先填写数据库类型和连接字符串')
        return
      }
      this.db_switch_testing = true
      this.db_switch_test_ok = false
      this.db_switch_test_result = ''
      axios.post('/set/db/test_connection', {
        db_type: this.db_switch_form.db_type,
        db_source: this.db_switch_form.db_source,
      }).then(resp => {
        var data = resp.data
        if (data && data.code === 0) {
          this.db_switch_test_ok = true
          this.db_switch_test_result = '连接测试成功，数据库可达'
        } else {
          this.db_switch_test_ok = false
          this.db_switch_test_result = '连接测试失败：' + ((data && data.msg) || '未知错误')
        }
      }).catch((err) => {
        this.db_switch_test_ok = false
        var msg = ''
        try { msg = err.response && err.response.data && err.response.data.msg } catch (e) { /* ignore */ }
        this.db_switch_test_result = '连接测试失败：' + (msg || err.message || '未知错误')
      }).finally(() => {
        this.db_switch_testing = false
      })
    },
    dbSwitchNextStep() {
      if (this.dbSwitchNextDisabled) return
      this.db_switch_step++
    },
    execDbSwitch() {
      this.db_switch_executing = true
      axios.post('/set/db/switch', {
        db_type: this.db_switch_form.db_type,
        db_source: this.db_switch_form.db_source,
        migration: this.db_switch_form.migration,
      }).then(resp => {
        var data = resp.data
        if (data && data.code === 0) {
          // 标记数据库刚刚切换，下次页面加载时提示手动还原
          try { localStorage.setItem('remlink_db_switched', '1') } catch (e) { /* ignore */ }
          this.$message.success('数据库切换成功，服务即将重启，页面将自动刷新')
          this.db_switch_dialog = false
          this.resetDbSwitchWizard()
          // 轮询等待服务恢复后自动刷新页面
          this.waitForRestart()
        } else {
          this.$message.error('数据库切换失败：' + ((data && data.msg) || '未知错误'))
        }
      }).catch((err) => {
        this.showApiError(err, '数据库切换失败')
      }).finally(() => {
        this.db_switch_executing = false
      })
    },
    waitForRestart() {
      // 轮询等待服务重启完成后自动刷新页面
      var maxRetries = 60 // 最多等 2 分钟（60 × 2s）
      var interval = null
      var retries = 0
      interval = setInterval(function () {
        retries++
        axios.get('/set/soft/status', { timeout: 3000 }).then(function (/*resp*/) {
          // 服务已恢复，刷新页面
          clearInterval(interval)
          window.location.reload()
        }).catch(function () {
          if (retries >= maxRetries) {
            clearInterval(interval)
          }
        })
      }, 2000)
    },
    checkDbSwitchRestore() {
      // 检测是否为数据库切换后首次访问，提示手动还原
      var flag
      try { flag = localStorage.getItem('remlink_db_switched') } catch (e) { /* ignore */ }
      if (flag !== '1') return
      // 清除标记（只提示一次）
      try { localStorage.removeItem('remlink_db_switched') } catch (e) { /* ignore */ }
      // 检查是否有备份文件
      axios.get('/set/db/backups').then(resp => {
        var list = resp.data.data || []
        if (list.length > 0) {
          this.restore_prompt_dialog = true
        }
      }).catch(() => { })
    },
    goToRestore() {
      this.restore_prompt_dialog = false
      this.active_tab = 'backup'
      this.loadBackups()
    },
    restoreBackup(row) {
      this.$confirm('还原将覆盖当前数据库对应数据，是否继续？', '确认还原', {
        confirmButtonText: '还原',
        cancelButtonText: '取消',
        type: 'warning'
      }).then(() => {
        axios.post('/set/db/restore', { file: row.name }).then(resp => {
          var result = resp.data.data || {}
          this.$message.success('还原成功')
          if (result.needs_restart) {
            this.$message.warning('监听地址/证书等启动期配置需重启服务生效')
          }
          // 还原可能改变了配置，刷新
          this.getSoftInfo()
          this.getSoftStatus()
          this.loadBackups()
        }).catch((err) => {
          this.showApiError(err, '还原失败')
        })
      }).catch(() => { })  // 取消还原
    },
    deleteBackup(row) {
      this.$confirm('确定删除备份文件「' + row.name + '」？', '确认删除', {
        confirmButtonText: '删除',
        cancelButtonText: '取消',
        type: 'warning'
      }).then(() => {
        axios.post('/set/db/backup/delete', { file: row.name }).then(() => {
          this.$message.success('已删除')
          this.loadBackups()
        }).catch((err) => {
          this.showApiError(err, '删除失败')
        })
      }).catch(() => { })  // 取消删除
    },
    formatNumber(n) {
      if (n === null || n === undefined) return '0'
      return n.toLocaleString()
    },
    formatSize(bytes) {
      if (!bytes) return '0 B'
      var units = ['B', 'KB', 'MB', 'GB']
      var i = 0
      var size = bytes
      while (size >= 1024 && i < units.length - 1) {
        size /= 1024
        i++
      }
      return size.toFixed(1) + ' ' + units[i]
    },
  },
}
</script>

<style scoped>
.soft-page {
  border-radius: var(--card-radius);
  border: 1px solid var(--border-color-light);
}

/* IPv4 网络配置合并卡片 */
.ipv4-config-card {
  border: 1px solid var(--color-primary-light);
  border-radius: 6px;
  padding: 12px 14px;
  margin-bottom: 10px;
  background: var(--color-primary-bg);
}

.ipv4-config-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}

.ipv4-config-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.ipv4-config-item label {
  font-size: 12px;
  color: var(--text-regular);
  font-weight: 500;
}

.ipv4-config-footer {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 10px;
  margin-top: 10px;
}

.soft-tabs {
  min-width: 0;
}

.soft-tabs ::v-deep .el-tabs__content {
  padding: 16px 20px;
  overflow-x: auto;
}

.page-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 14px;
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

.notice-stack {
  display: grid;
  gap: 10px;
  margin-bottom: 14px;
}

.summary-row {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}

.summary-item {
  padding: 16px;
  background: linear-gradient(135deg, var(--bg-hover) 0%, var(--bg-card) 100%);
  border: 1px solid var(--border-color-light);
  border-radius: 8px;
  text-align: center;
  transition: box-shadow 0.2s;
}

.summary-icon-wrapper {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  margin-bottom: 8px;
}

.summary-icon-group {
  background: var(--color-primary-bg) !important;
  color: var(--color-primary) !important;
}

.summary-icon-item {
  background: var(--success-bg) !important;
  color: var(--color-success) !important;
}

.summary-icon-restart {
  background: var(--warning-bg) !important;
  color: var(--color-warning) !important;
}

.summary-icon-immediate {
  background: var(--danger-bg) !important;
  color: var(--color-danger) !important;
}

.summary-item:hover {
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
}

.summary-label {
  display: block;
  margin-bottom: 8px;
  font-size: 12px;
  color: var(--text-secondary);
}

.summary-item strong {
  font-size: 24px;
  font-weight: 700;
  color: var(--text-primary);
}

.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
  padding: 12px 16px;
  background: var(--bg-stripe);
  border: 1px solid var(--border-color-light);
  border-radius: 8px;
}

.toolbar-left,
.toolbar-right {
  display: flex;
  align-items: center;
  gap: 10px;
}

.search-input {
  width: 300px;
}

.effect-select {
  width: 130px;
}

.empty-hint {
  text-align: center;
  padding: 40px 0;
  color: var(--text-secondary);
  font-size: 14px;
}

.empty-hint i {
  margin-right: 6px;
}

/* 分组折叠面板 */
.group-collapse {
  border: none;
}

.group-collapse>>>.el-collapse-item {
  margin-bottom: 8px;
  border: 1px solid var(--border-color-light);
  border-radius: 8px;
  overflow: hidden;
}

.group-collapse>>>.el-collapse-item__header {
  padding: 0 16px;
  height: 46px;
  line-height: 46px;
  font-size: 14px;
  background: linear-gradient(135deg, #f0f3fa 0%, var(--bg-stripe) 100%);
  border-bottom: 1px solid var(--border-color-light);
  border-radius: 8px 8px 0 0;
}

.group-collapse>>>.el-collapse-item.is-active .el-collapse-item__header {
  border-radius: 8px 8px 0 0;
}

.group-collapse>>>.el-collapse-item__wrap {
  border-bottom: none;
}

/* 修复：el-collapse 默认 overflow:hidden 会裁剪溢出内容（含保存按钮） */
.group-collapse>>>.el-collapse-item.is-active>.el-collapse-item__wrap {
  overflow: visible;
}

.group-title {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
}

.group-name {
  font-weight: 600;
  color: var(--text-primary);
}

.group-count {
  font-size: 12px;
  color: var(--text-secondary);
}

/* 配置项行 */
.group-items {
  padding: 12px 16px;
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px 24px;
}

.config-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  border: 1px solid var(--border-color-light);
  border-radius: 8px;
  background: var(--bg-card);
  transition: box-shadow 0.2s, border-color 0.2s;
}

.config-item:hover {
  border-color: var(--text-placeholder);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
}

.config-item-full {
  align-items: flex-start;
}

.config-item-info {
  flex: 1 1 0;
  min-width: 0;
  overflow: hidden;
}

.config-item-usage {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
  line-height: 1.4;
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  line-clamp: 2;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  word-break: break-all;
}

.config-item-name {
  margin-top: 2px;
  font-size: 11px;
  color: var(--text-secondary);
  font-family: Menlo, Monaco, Consolas, monospace;
}

.config-item-value {
  width: 170px;
  flex-shrink: 0;
}

.config-item-hint {
  margin-top: 4px;
  font-size: 11px;
  color: var(--color-warning);
  line-height: 1.5;
}

.config-item-effect {
  flex-shrink: 0;
}

.config-item-action {
  flex-shrink: 0;
}

.profile-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.section-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary);
}

.hash-line {
  margin-top: 4px;
  font-size: 12px;
  color: var(--text-secondary);
}

.soft-page>>>.el-textarea__inner {
  font-family: Menlo, Monaco, Consolas, "Courier New", monospace;
  line-height: 1.5;
}

@media (max-width: 1100px) {

  .page-head,
  .toolbar,
  .profile-toolbar {
    align-items: stretch;
    flex-direction: column;
    gap: 10px;
  }

  .summary-row {
    grid-template-columns: repeat(2, 1fr);
  }

  .toolbar-left {
    flex-direction: column;
    align-items: stretch;
  }

  .search-input,
  .effect-select {
    width: 100%;
  }

  .group-items {
    grid-template-columns: 1fr;
  }

  .config-item {
    flex-wrap: wrap;
    align-items: center;
    gap: 6px 10px;
  }

  .config-item-info {
    flex: 1 1 100%;
    min-width: 0;
  }

  .config-item-value {
    flex: 1 1 0;
    width: auto;
    min-width: 0;
    max-width: 220px;
  }

  .config-item-effect,
  .config-item-action {
    flex-shrink: 0;
  }
}

@media (max-width: 1400px) and (min-width: 1101px) {
  .group-items {
    grid-template-columns: 1fr 1fr;
    gap: 6px 16px;
  }

  .config-item {
    padding: 6px 10px;
  }

  .config-item-value {
    width: 140px;
  }
}

/* ====== 备份相关样式 ====== */
.backup-table {
  margin-top: 0;
}

.table-check-group {
  max-height: 320px;
  overflow-y: auto;
  border: 1px solid var(--border-color-light);
  border-radius: 6px;
  padding: 8px 0;
}

.table-check-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 16px;
}

.table-check-item:hover {
  background: var(--bg-hover);
}

.check-label {
  font-size: 13px;
  color: var(--text-primary);
}

.check-name {
  font-size: 11px;
  color: var(--text-secondary);
  font-family: Menlo, Monaco, Consolas, monospace;
}

.check-count-wrapper {
  flex-shrink: 0;
  margin-left: 8px;
}

/* ====== 数据库切换向导样式 ====== */
.db-switch-body {
  min-height: 180px;
}

.db-switch-test-section {
  padding: 20px 0;
}

.test-result-box {
  margin-bottom: 16px;
}

.migration-option {
  display: flex;
  align-items: flex-start;
  padding: 14px 16px;
  margin-bottom: 10px;
  border: 1px solid var(--border-color-light);
  border-radius: 8px;
  cursor: pointer;
  transition: border-color 0.2s, box-shadow 0.2s;
}

.migration-option:hover {
  border-color: var(--color-primary);
  box-shadow: 0 2px 8px rgba(64, 158, 255, 0.1);
}

.migration-radio {
  margin-right: 12px;
  margin-top: 2px;
  flex-shrink: 0;
}

.migration-content {
  flex: 1;
}

.migration-title {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
  font-size: 14px;
}

.migration-desc {
  font-size: 12px;
  color: var(--text-secondary);
  line-height: 1.6;
}

.confirm-form {
  margin-bottom: 8px;
}

.confirm-form code {
  font-family: Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  background: var(--bg-hover);
  padding: 2px 6px;
  border-radius: 3px;
}

.alert-list {
  margin: 4px 0;
  padding-left: 18px;
}

.alert-list li {
  margin-bottom: 4px;
  font-size: 13px;
}

.form-tip {
  margin-top: 6px;
  font-size: 11px;
  color: var(--text-secondary);
}

/* 只读配置项样式 */
.config-item-value .el-input.is-disabled .el-input__inner {
  background-color: var(--bg-hover);
  color: var(--text-secondary);
  cursor: not-allowed;
}
</style>
