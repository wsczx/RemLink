<template>
  <div class="lock-page">
    <!-- 统计卡片 -->
    <div class="stats-row">
      <div class="stat-card">
        <div class="stat-icon stat-icon-total">
          <i class="el-icon-lock"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ statTotal }}</div>
          <div class="stat-label">锁定总数</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-icon-active">
          <i class="el-icon-warning"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ statLocked }}</div>
          <div class="stat-label">当前锁定</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-icon-attempts">
          <i class="el-icon-odometer"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ statMaxAttempts }}</div>
          <div class="stat-label">最大失败次数</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-icon-user">
          <i class="el-icon-s-custom"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ statUniqueUsers }}</div>
          <div class="stat-label">锁定用户数</div>
        </div>
      </div>
    </div>

    <!-- 锁定管理：多节点按节点分块（首块本机，其余各一块）；单节点直接扁平展示 -->
    <el-card class="table-card" shadow="never" v-loading="loading">
      <div slot="header" class="card-header">
        <span class="card-title"><i class="el-icon-lock"></i> 锁定管理</span>
        <div class="card-actions">
          <el-button size="small" type="primary" icon="el-icon-refresh" @click="getLocks">刷新信息</el-button>
        </div>
      </div>

      <div :class="isMultiNode ? 'node-blocks' : 'node-blocks node-blocks--flat'">
        <div v-for="n in nodes" :key="n.node_id" :class="isMultiNode ? 'node-block' : 'node-block node-block--flat'">
          <div class="node-block-head" v-if="isMultiNode">
            <span class="node-block-title">
              <el-tag v-if="n.is_self" type="success" size="mini" effect="plain">本机</el-tag>
              <span class="node-block-name">{{ n.node_name || '本节点' }}</span>
            </span>
            <span class="node-block-count">{{ (n.locks || []).length }} 条</span>
            <el-tag v-if="!n.reachable" type="danger" size="mini">不可达</el-tag>
          </div>
          <el-alert v-if="isMultiNode && !n.reachable" type="error" :closable="false"
            :title="n.error || '节点不可达，无法获取锁定信息'" class="node-block-alert" />
          <el-table v-else :data="n.locks || []" stripe highlight-current-row style="width:100%" border
            :header-cell-style="{ background: 'var(--bg-header)', color: 'var(--text-primary)', fontWeight: '600', fontSize: '13px' }">
            <el-table-column type="index" label="#" width="50" align="center"></el-table-column>
            <el-table-column prop="description" label="描述" min-width="140" sortable>
              <template slot-scope="scope">
                <span class="lock-desc">{{ scope.row.description || '-' }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="username" label="用户名" width="120" sortable>
              <template slot-scope="scope">
                <span class="lock-user">{{ userLabel(scope.row.username, scope.row.nickname) || '-' }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="ip" label="IP地址" width="140" sortable></el-table-column>
            <el-table-column prop="state.locked" label="状态" width="95" align="center" sortable>
              <template slot-scope="scope">
                <span :class="scope.row.state.locked ? 'status-dot-locked' : 'status-dot-free'"
                  class="status-dot"></span>
                <span class="status-text" :class="scope.row.state.locked ? 'text-danger' : 'text-success'">
                  {{ scope.row.state.locked ? '已锁定' : '正常' }}
                </span>
              </template>
            </el-table-column>
            <el-table-column prop="state.attempts" label="失败次数" width="90" align="center" sortable>
              <template slot-scope="scope">
                <span class="attempts-count" :class="scope.row.state.attempts > 3 ? 'attempts-high' : ''">
                  {{ scope.row.state.attempts }}
                </span>
              </template>
            </el-table-column>
            <el-table-column prop="state.lock_time" label="锁定截止" width="165" sortable>
              <template slot-scope="scope">{{ formatDate(scope.row.state.lock_time) }}</template>
            </el-table-column>
            <el-table-column prop="state.lastAttempt" label="最后尝试" width="165" sortable>
              <template slot-scope="scope">{{ formatDate(scope.row.state.lastAttempt) }}</template>
            </el-table-column>
            <el-table-column label="操作" width="90" fixed="right" align="center">
              <template slot-scope="scope">
                <el-button v-if="scope.row.state.locked" size="mini" type="danger" @click="unlock(scope.row, n.node_id)"
                  icon="el-icon-unlock">解锁</el-button>
                <el-button v-else size="mini" type="info" disabled>正常</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </div>
    </el-card>
  </div>
</template>

<script>
import axios from 'axios';
import userLabel from '@/mixins/userLabel';

export default {
  name: 'LockManager',
  mixins: [userLabel],
  created() {
    this.$emit('update:route_path', this.$route.path)
    this.$emit('update:route_name', ['用户管理', '锁定管理'])
    this.getLocks();
  },
  data() { return { loading: false, nodes: [] }; },
  computed: {
    // 全部可达节点的锁定（带节点标记），供顶部统计
    allLocks() {
      const rows = [];
      this.nodes.forEach(n => {
        if (!n.reachable) return;
        (n.locks || []).forEach(l => rows.push(Object.assign({}, l, { node_id: n.node_id })));
      });
      return rows;
    },
    statTotal() { return this.allLocks.length },
    statLocked() { return this.allLocks.filter(l => l.state.locked).length },
    statMaxAttempts() {
      return Math.max(...this.allLocks.map(l => l.state.attempts), 0);
    },
    statUniqueUsers() {
      return new Set(this.allLocks.map(l => l.username).filter(Boolean)).size;
    },
    // 仅多节点（>1 个）才分块展示；单节点恒为 1 个本机节点，应扁平呈现
    isMultiNode() { return this.nodes.length > 1 },
  },
  methods: {
    getLocks() {
      this.loading = true;
      axios.get('/cluster/locks').then(resp => {
        this.nodes = (resp.data && resp.data.data) || [];
        this.loading = false;
      }).catch(() => {
        this.$message.error('无法获取锁定信息');
        this.loading = false;
      });
    },
    unlock(lock, nodeId) {
      this.$confirm('确定要解锁吗？', '解锁确认', {
        confirmButtonText: '确定', cancelButtonText: '取消',
        type: 'warning',
      }).then(() => {
        axios.post('/cluster/unlock', {
          node: nodeId,
          state: { locked: false },
          username: lock.username, ip: lock.ip,
          description: lock.description,
        }).then(() => {
          this.$message.success('解锁成功');
          this.getLocks();
        }).catch(() => {
          this.$message.error('解锁失败');
        });
      }).catch(() => { });
    },
    formatDate(dateString) {
      if (!dateString) return '-';
      const date = new Date(dateString);
      return new Intl.DateTimeFormat('zh-CN', {
        year: 'numeric', month: '2-digit', day: '2-digit',
        hour: '2-digit', minute: '2-digit', second: '2-digit',
        hour12: false,
      }).format(date);
    },
  },
};
</script>

<style scoped>
/* ========== 页面整体 ========== */
.lock-page {
  padding: 4px 0;
}

/* ========== 统计卡片 ========== */
.stat-icon-total {
  background: var(--color-primary-bg);
  color: var(--color-primary);
}

.stat-icon-active {
  background: var(--danger-bg);
  color: var(--color-danger);
}

.stat-icon-attempts {
  background: var(--warning-bg);
  color: var(--color-warning);
}

.stat-icon-user {
  background: var(--success-bg);
  color: var(--color-success);
}

/* 表格卡片（全局已提供） */

/* ========== 节点分块 ========== */
.node-blocks {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.node-block {
  border: 1px solid var(--border-light, #ebeef5);
  border-radius: 8px;
  overflow: hidden;
}

.node-block-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  background: var(--bg-header, #fafafa);
  border-bottom: 1px solid var(--border-light, #ebeef5);
}

.node-block-title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.node-block-name {
  font-weight: 600;
  font-size: 14px;
  color: var(--text-primary);
}

.node-block-count {
  font-size: 12px;
  color: var(--text-secondary);
}

.node-block-alert {
  margin: 0;
  border-radius: 0;
}

/* 单节点：去掉分块外框，呈现为普通扁平表格 */
.node-block--flat {
  border: none;
  border-radius: 0;
  padding: 0;
  margin: 0;
  background: transparent;
}

.node-blocks--flat {
  gap: 0;
}

/* 表格内 */
.lock-desc {
  font-size: 13px;
  color: var(--text-primary);
}

.lock-user {
  font-weight: 600;
  color: var(--text-primary);
}

.status-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 5px;
  vertical-align: middle;
}

.status-dot-locked {
  background: var(--color-danger);
  box-shadow: 0 0 0 2px rgba(245, 108, 108, 0.2);
}

.status-dot-free {
  background: var(--color-success);
  box-shadow: 0 0 0 2px rgba(103, 194, 58, 0.2);
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

.attempts-count {
  font-weight: 600;
  font-size: 13px;
  color: var(--text-primary);
}

.attempts-high {
  color: var(--color-danger);
}

/* 响应式已由全局样式处理 */
</style>
