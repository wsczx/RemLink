<template>
  <div class="cluster-page">
    <el-card class="box-card" shadow="never">
      <div slot="header" class="clearfix">
        <span class="card-title"><i class="el-icon-s-platform"></i> 节点管理</span>
        <el-button style="float: right" type="primary" size="small" :loading="loading" @click="load">刷新</el-button>
        <el-button style="float: right; margin-right: 8px" type="warning" size="small" :disabled="nodes.length === 0"
          @click="restartAll">重启全部</el-button>
        <el-button style="float: right; margin-right: 8px" type="success" size="small" :disabled="nodes.length === 0"
          @click="upgradeAll">升级全部</el-button>
      </div>
      <el-alert type="info" :closable="false" show-icon style="margin-bottom: 12px">
        多节点共享同一数据库，配置/用户/策略在各节点自动同步。仅监听地址、虚拟网络（CIDR/NAT/网卡）、防火墙、数据库类型等「需重启」类配置改动后需对各节点分别重启才生效；用户/策略及多数运行期配置即时生效，无需重启。节点特定配置（主网卡等）可用各机
        LINK_* 环境变量覆盖
      </el-alert>
      <el-alert type="success" :closable="false" show-icon style="margin-bottom: 12px">
        多节点共享同一数据库即互认为同一组节点，无需手动注册或配置：节点启动后通过心跳互相发现。各节点需能经「管理地址」互访后台
      </el-alert>
      <el-alert v-if="versionSkew" type="warning" :closable="false" show-icon style="margin-bottom: 12px">
        检测到节点版本不一致，建议先统一版本再操作
      </el-alert>
      <el-table :data="nodes" v-loading="loading" stripe border style="width: 100%">
        <el-table-column label="状态" width="120">
          <template slot-scope="scope">
            <el-tag :type="scope.row.reachable ? 'success' : 'danger'" size="mini">{{ scope.row.reachable ? '正常' : '故障'
              }}</el-tag>
            <el-tag v-if="scope.row.online === false" type="info" size="mini" effect="plain">离线</el-tag>
            <span v-if="scope.row.is_self" class="self-tag">本机</span>
            <el-tag v-if="scope.row.needs_restart" type="warning" size="mini">需重启</el-tag>
            <el-tooltip v-if="scope.row.reachable === false && scope.row.error" :content="scope.row.error"
              placement="top">
              <div class="node-err">{{ scope.row.error }}</div>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column prop="name" label="名称" min-width="120" />
        <el-table-column label="管理地址" min-width="220">
          <template slot-scope="scope">
            <span class="addr-text">{{ scope.row.admin_url }}</span>
            <el-button type="text" size="mini" icon="el-icon-edit" :disabled="!scope.row.is_self"
              @click="openEdit(scope.row)">编辑</el-button>
          </template>
        </el-table-column>
        <el-table-column prop="version" label="版本" width="100" />
        <el-table-column label="最后心跳" width="170">
          <template slot-scope="scope">{{ formatTime(scope.row.last_seen) }}</template>
        </el-table-column>
        <el-table-column label="资源占用" min-width="150">
          <template slot-scope="scope">
            <span v-if="scope.row.health">CPU {{ scope.row.health.cpu.toFixed(1) }}% · 内存 {{
              scope.row.health.mem.toFixed(1) }}%</span>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="实时吞吐" min-width="170">
          <template slot-scope="scope">
            <span v-if="scope.row.health">
              <span class="rate-up">↑{{ fmtRate(scope.row.health.up) }}</span>
              <span class="rate-sep"> / </span>
              <span class="rate-down">↓{{ fmtRate(scope.row.health.down) }}</span>
            </span>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="并发连接" width="112" align="center">
          <template slot-scope="scope">
            <span v-if="scope.row.concurrency">{{ scope.row.concurrency.current }}<span class="muted"> / {{
              scope.row.concurrency.max || '-' }}</span></span>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="运行时长" width="120">
          <template slot-scope="scope">{{ fmtUptime(scope.row.uptime) }}</template>
        </el-table-column>
        <el-table-column label="在线会话" width="90" align="center">
          <template slot-scope="scope">{{ scope.row.sessions_count || 0 }}</template>
        </el-table-column>
        <el-table-column label="锁定" width="76" align="center">
          <template slot-scope="scope">{{ (scope.row.locks || []).length }}</template>
        </el-table-column>
        <el-table-column label="操作" width="170" fixed="right">
          <template slot-scope="scope">
            <el-button type="warning" size="mini" :disabled="!scope.row.reachable"
              @click="restartOne(scope.row)">重启</el-button>
            <el-button type="success" size="mini" :disabled="!scope.row.reachable"
              @click="upgradeOne(scope.row)">升级</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog :title="'编辑节点 · ' + (editNode.name || '')" :visible.sync="editDialogVisible" width="520px" append-to-body
      @close="editUrl = ''; editName = ''">
      <el-alert type="info" :closable="false" show-icon style="margin-bottom: 12px">节点名称与管理地址用于节点间识别与调用：管理地址优先取
        LINK_CLUSTER_URL，其次此处手动填写（其它节点可访问的 https 地址），最后为默认兜底；名称默认取主机名或
        LINK_NODE_NAME，可在此修改并持久化到本机</el-alert>
      <el-form label-width="88px" style="margin-top: 8px">
        <el-form-item label="节点名称">
          <el-input v-model="editName" :disabled="!editNode.is_self" placeholder="节点名称" />
        </el-form-item>
        <el-form-item label="管理地址">
          <el-input v-model="editUrl" :disabled="!editNode.is_self" placeholder="https://节点IP或域名:8800" />
        </el-form-item>
      </el-form>
      <span slot="footer">
        <el-button size="small" @click="editDialogVisible = false">取消</el-button>
        <el-button size="small" type="primary" :loading="editSaving" @click="saveEdit">保存</el-button>
      </span>
    </el-dialog>
  </div>
</template>

<script>
import axios from "axios";
export default {
  name: "Cluster",
  data() {
    return { baseNodes: [], overviewRaw: [], nodes: [], loading: false, versionSkew: false, editDialogVisible: false, editNode: {}, editUrl: '', editName: '', editSaving: false, timer: null };
  },
  created() {
    this.$emit('update:route_name', ['节点管理']);
  },
  mounted() {
    this.load();
    this.loadOverview();
    this.timer = setInterval(() => { this.load(); this.loadOverview(); }, 20000);
  },
  beforeDestroy() {
    if (this.timer) clearInterval(this.timer);
  },
  methods: {
    load() {
      this.loading = true;
      axios.get('/cluster/nodes').then(resp => {
        this.baseNodes = (resp.data && resp.data.data) || [];
        this.applyMerge();
      }).catch(() => {
        this.$message.error('获取节点列表失败');
      }).finally(() => { this.loading = false; });
    },
    loadOverview() {
      return axios.get('/cluster/overview').then(resp => {
        this.overviewRaw = (resp.data && resp.data.data) || [];
        this.applyMerge();
        const vers = this.overviewRaw.map(n => n.version).filter(Boolean);
        this.versionSkew = new Set(vers).size > 1;
      }).catch(() => {
        this.$message.error('获取节点总览失败');
      });
    },
    applyMerge() {
      const byId = {};
      this.overviewRaw.forEach(n => { byId[n.node_id] = n; });
      this.nodes = this.baseNodes.map(n => {
        const o = byId[n.node_id] || byId[n.id];
        return o ? Object.assign({}, n, {
          reachable: o.reachable, error: o.error, health: o.health,
          concurrency: o.concurrency, sessions_count: o.sessions_count, locks: o.locks,
          uptime: o.uptime,
        }) : n;
      });
    },
    openEdit(node) {
      this.editNode = node;
      this.editUrl = node.admin_url || '';
      this.editName = node.name || '';
      this.editDialogVisible = true;
    },
    saveEdit() {
      if (!this.editUrl) { this.$message.warning('请填写管理地址'); return; }
      this.editSaving = true;
      axios.post('/cluster/node/update', { id: this.editNode.id, admin_url: this.editUrl, name: this.editName }).then(() => {
        this.$message.success('已更新节点管理地址');
        this.editDialogVisible = false;
        setTimeout(this.load, 800);
      }).catch(() => { this.$message.error('更新失败'); }).finally(() => { this.editSaving = false; });
    },
    formatTime(ts) {
      if (!ts) return '-';
      const t = typeof ts === 'number' && ts < 1e12 ? ts * 1000 : ts;
      return new Date(t).toLocaleString();
    },
    // 字节/秒 → 人类可读速率
    fmtRate(b) {
      b = Number(b) || 0;
      if (b < 1024) return b.toFixed(0) + ' B/s';
      if (b < 1024 * 1024) return (b / 1024).toFixed(1) + ' KB/s';
      if (b < 1024 * 1024 * 1024) return (b / 1024 / 1024).toFixed(2) + ' MB/s';
      return (b / 1024 / 1024 / 1024).toFixed(2) + ' GB/s';
    },
    // 秒 → 简洁时长
    fmtUptime(s) {
      s = Number(s) || 0;
      const d = Math.floor(s / 86400);
      const h = Math.floor((s % 86400) / 3600);
      const m = Math.floor((s % 3600) / 60);
      if (d > 0) return d + '天' + h + '时';
      if (h > 0) return h + '时' + m + '分';
      return m + '分';
    },
    restartOne(node) {
      this.$confirm(`确认重启节点「${node.name}」？该节点 VPN 连接会短暂中断。`, '重启节点', {
        type: 'warning', confirmButtonText: '重启', cancelButtonText: '取消'
      }).then(() => {
        axios.post('/cluster/restart', { node: node.id }).then(() => {
          this.$message.success('已发送重启指令');
          this.afterRestart();
        }).catch(() => { this.$message.error('重启指令发送失败'); });
      }).catch(() => { });
    },
    restartAll() {
      this.$confirm('确认重启全部节点？将先重启其它节点，最后重启本机。', '重启全部', {
        type: 'warning', confirmButtonText: '全部重启', cancelButtonText: '取消'
      }).then(() => {
        axios.post('/cluster/restart/all').then(resp => {
          const d = (resp.data && resp.data.data) || {};
          let msg = '已发送全部重启指令，本机即将重启';
          if ((d.failed && d.failed.length) || d.skipped) {
            msg += `（跳过离线 ${d.skipped || 0} 个，失败 ${d.failed ? d.failed.length : 0} 个）`;
          }
          this.$message.success(msg);
          this.afterRestart();
        }).catch(() => { this.$message.error('重启指令发送失败'); });
      }).catch(() => { });
    },
    refreshAll() {
      this.load();
      this.loadOverview();
    },
    upgradeOne(node) {
      this.$confirm(`确认升级节点「${node.name}」？该节点将下载新版本并自动重启。`, '节点升级', {
        type: 'warning', confirmButtonText: '升级', cancelButtonText: '取消'
      }).then(() => {
        axios.post('/cluster/upgrade', { node: node.id }).then(() => {
          this.$message.success(`已触发节点「${node.name}」升级，将自动下载新版本并重启`);
          this.afterRestart();
        }).catch(() => { this.$message.error('升级触发失败'); });
      }).catch(() => { });
    },
    upgradeAll() {
      this.$confirm('确认升级全部节点？将逐一触发各节点下载新版本并重启（先其它节点，最后本机）。', '升级全部', {
        type: 'warning', confirmButtonText: '全部升级', cancelButtonText: '取消'
      }).then(() => {
        axios.post('/cluster/upgrade/all').then(resp => {
          const d = (resp.data && resp.data.data) || {};
          let msg = '已发送全部升级指令，各节点将依次下载并重启';
          if ((d.failed && d.failed.length) || d.skipped) {
            msg += `（跳过离线 ${d.skipped || 0} 个，失败 ${d.failed ? d.failed.length : 0} 个）`;
          }
          this.$message.success(msg);
          this.afterRestart();
        }).catch(() => { this.$message.error('升级指令发送失败'); });
      }).catch(() => { });
    },
    afterRestart() {
      // 重启后轮询一段时间，捕捉节点下线→上线；整体 20s 自动刷新兜底
      [2000, 10000, 25000].forEach(d => setTimeout(this.refreshAll, d));
    },
  },
};
</script>

<style scoped>
.card-title {
  font-weight: 600;
}

.self-tag {
  margin-left: 6px;
  color: var(--color-primary);
  font-size: 12px;
}

.addr-text {
  word-break: break-all;
}

.rate-up {
  color: var(--color-success);
}

.rate-down {
  color: var(--text-secondary);
}

.rate-sep {
  color: var(--border-base);
  margin: 0 2px;
}

.muted {
  color: var(--text-placeholder);
}

.node-err {
  color: #f56c6c;
  font-size: 11px;
  line-height: 14px;
  margin-top: 4px;
  word-break: break-all;
  max-height: 42px;
  overflow: hidden;
}
</style>
