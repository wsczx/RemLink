<template>
  <div class="system-page">
    <!-- 统计卡片 -->
    <div class="stats-row">
      <div class="stat-card">
        <div class="stat-icon stat-icon-cpu">
          <i class="el-icon-cpu"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value" v-if="system.cpu">{{ system.cpu.percent }}%</div>
          <div class="stat-value" v-else>-</div>
          <div class="stat-label">CPU 使用率</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-icon-mem">
          <i class="el-icon-monitor"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value" v-if="system.mem">{{ system.mem.percent }}%</div>
          <div class="stat-value" v-else>-</div>
          <div class="stat-label">内存使用率</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-icon-disk">
          <i class="el-icon-files"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value" v-if="system.disk">{{ system.disk.percent }}%</div>
          <div class="stat-value" v-else>-</div>
          <div class="stat-label">磁盘使用率</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-icon-uptime">
          <i class="el-icon-time"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ formattedUptime }}</div>
          <div class="stat-label">运行时长</div>
        </div>
      </div>
    </div>

    <!-- 资源详情 -->
    <div class="detail-row">
      <el-card class="info-card" shadow="never" v-if="system.cpu">
        <div class="card-title"><i class="el-icon-cpu"></i> CPU</div>
        <div class="progress-wrap">
          <el-progress type="circle" :percentage="system.cpu.percent" :width="130" :stroke-width="10" :show-text="false"
            :color="progressColor(system.cpu.percent)" />
          <span class="progress-num" :style="{ color: progressColor(system.cpu.percent) }">{{ system.cpu.percent
          }}%</span>
        </div>
        <div class="info-list">
          <div class="info-item">
            <span class="info-label"><i class="el-icon-s-marketing"></i> 主频</span>
            <span class="info-value">{{ system.cpu.ghz }}</span>
          </div>
          <div class="info-item">
            <span class="info-label"><i class="el-icon-loading"></i> 系统负载</span>
            <span class="info-value">{{ system.sys.load }}</span>
          </div>
          <div class="info-item">
            <span class="info-label"><i class="el-icon-s-grid"></i> 核心数</span>
            <span class="info-value">{{ system.cpu.core }}</span>
          </div>
          <div class="info-item">
            <span class="info-label"><i class="el-icon-s-data"></i> 型号</span>
            <span class="info-value model-name">{{ system.cpu.modelName }}</span>
          </div>
        </div>
      </el-card>

      <el-card class="info-card" shadow="never" v-if="system.mem">
        <div class="card-title"><i class="el-icon-monitor"></i> 内存</div>
        <div class="progress-wrap">
          <el-progress type="circle" :percentage="system.mem.percent" :width="130" :stroke-width="10" :show-text="false"
            :color="progressColor(system.mem.percent)" />
          <span class="progress-num" :style="{ color: progressColor(system.mem.percent) }">{{ system.mem.percent
          }}%</span>
        </div>
        <div class="info-list">
          <div class="info-item">
            <span class="info-label"><i class="el-icon-s-marketing"></i> 总内存</span>
            <span class="info-value">{{ system.mem.total }}</span>
          </div>
          <div class="info-item">
            <span class="info-label"><i class="el-icon-s-opportunity"></i> 剩余内存</span>
            <span class="info-value">{{ system.mem.free }}</span>
          </div>
        </div>
      </el-card>

      <el-card class="info-card" shadow="never" v-if="system.disk">
        <div class="card-title"><i class="el-icon-files"></i> 磁盘</div>
        <div class="progress-wrap">
          <el-progress type="circle" :percentage="system.disk.percent" :width="130" :stroke-width="10"
            :show-text="false" :color="progressColor(system.disk.percent)" />
          <span class="progress-num" :style="{ color: progressColor(system.disk.percent) }">{{ system.disk.percent
          }}%</span>
        </div>
        <div class="info-list">
          <div class="info-item">
            <span class="info-label"><i class="el-icon-s-marketing"></i> 总存储</span>
            <span class="info-value">{{ system.disk.total }}</span>
          </div>
          <div class="info-item">
            <span class="info-label"><i class="el-icon-s-opportunity"></i> 剩余存储</span>
            <span class="info-value">{{ system.disk.free }}</span>
          </div>
        </div>
      </el-card>
    </div>

    <!-- 运行环境 + 服务器信息 双列 -->
    <div class="bottom-row" v-if="system.sys">
      <el-card class="info-card info-card--flat" shadow="never">
        <div class="card-title"><i class="el-icon-setting"></i> 运行环境</div>
        <div class="info-grid info-grid--two">
          <div class="info-item">
            <span class="info-label"><i class="el-icon-s-flag"></i> 软件版本</span>
            <span class="info-value code">
              {{ system.sys.appVersion }}
              <el-button type="text" size="mini" icon="el-icon-refresh" :loading="checkingUpdate" @click="checkUpdate"
                class="upgrade-check-btn">检查更新</el-button>
              <el-tag v-if="upgradeInfo" type="success" size="mini" class="upgrade-tag">
                新版本 {{ upgradeInfo.latest.version }}
              </el-tag>
            </span>
          </div>
          <div class="info-item">
            <span class="info-label"><i class="el-icon-s-marketing"></i> CommitId</span>
            <span class="info-value code">{{ system.sys.appCommitId }}</span>
          </div>
          <div class="info-item">
            <span class="info-label"><i class="el-icon-date"></i> BuildDate</span>
            <span class="info-value">{{ system.sys.appBuildDate }}</span>
          </div>
          <div class="info-item">
            <span class="info-label"><i class="el-icon-s-platform"></i> Go 版本</span>
            <span class="info-value">{{ system.sys.goVersion }}</span>
          </div>
          <div class="info-item">
            <span class="info-label"><i class="el-icon-monitor"></i> GO系统</span>
            <span class="info-value">{{ system.sys.goOs }}</span>
          </div>
          <div class="info-item">
            <span class="info-label"><i class="el-icon-cpu"></i> GoArch</span>
            <span class="info-value">{{ system.sys.goArch }}</span>
          </div>
          <div class="info-item">
            <span class="info-label"><i class="el-icon-connection"></i> Goroutine</span>
            <span class="info-value">{{ system.sys.goroutine }}</span>
          </div>
        </div>
      </el-card>
      <el-card class="info-card info-card--flat" shadow="never">
        <div class="card-title"><i class="el-icon-s-home"></i> 服务器信息</div>
        <div class="info-grid info-grid--one">
          <div class="info-item">
            <span class="info-label"><i class="el-icon-s-custom"></i> 机器名称</span>
            <span class="info-value">{{ system.sys.hostname }}</span>
          </div>
          <div class="info-item">
            <span class="info-label"><i class="el-icon-s-platform"></i> 操作系统</span>
            <span class="info-value">{{ system.sys.platform }}</span>
          </div>
          <div class="info-item">
            <span class="info-label"><i class="el-icon-cpu"></i> 内核版本</span>
            <span class="info-value">{{ system.sys.kernel }}</span>
          </div>
        </div>
      </el-card>
    </div>

    <!-- 升级对话框 -->
    <el-dialog title="软件升级" :visible.sync="upgradeDialog.visible" :close-on-click-modal="false"
      :close-on-press-escape="!upgradeDialog.running" :show-close="!upgradeDialog.running" width="520px"
      class="upgrade-dialog">
      <div v-if="upgradeDialog.step === 'info'" class="upgrade-body">
        <div class="upgrade-version-row">
          <div class="upgrade-version-item">
            <div class="upgrade-version-label">当前版本</div>
            <div class="upgrade-version-value">{{ system.sys ? system.sys.appVersion : '-' }}</div>
          </div>
          <div class="upgrade-arrow">
            <i class="el-icon-right"></i>
          </div>
          <div class="upgrade-version-item upgrade-version-new">
            <div class="upgrade-version-label">最新版本</div>
            <div class="upgrade-version-value">{{ upgradeDialog.info ? upgradeDialog.info.latest.version : '-' }}</div>
          </div>
        </div>
        <div class="upgrade-changelog" v-if="upgradeDialog.info && upgradeDialog.info.latest.body">
          <div class="upgrade-changelog-title">更新日志</div>
          <div class="upgrade-changelog-body">{{ upgradeDialog.info.latest.body }}</div>
        </div>
      </div>

      <div v-else-if="upgradeDialog.step === 'progress'" class="upgrade-body">
        <div class="upgrade-progress-wrap">
          <el-progress type="circle" :percentage="upgradeDialog.progress" :width="140" :stroke-width="10"
            :color="upgradeDialog.error ? 'var(--color-danger)' : 'var(--color-primary)'" />
          <span class="upgrade-stage-text">{{ stageText }}</span>
        </div>
        <div v-if="upgradeDialog.error" class="upgrade-error">
          <i class="el-icon-error"></i> {{ upgradeDialog.error }}
        </div>
      </div>

      <div v-else-if="upgradeDialog.step === 'done'" class="upgrade-body">
        <div class="upgrade-done">
          <i class="el-icon-success"></i>
          <div class="upgrade-done-text">升级成功！服务正在重启...</div>
          <div class="upgrade-done-sub">重启完成后将自动刷新页面</div>
        </div>
      </div>

      <span slot="footer" class="dialog-footer">
        <template v-if="upgradeDialog.step === 'info'">
          <el-button @click="upgradeDialog.visible = false">取消</el-button>
          <el-button type="primary" @click="startUpgrade">立即升级</el-button>
        </template>
        <template v-else-if="upgradeDialog.step === 'progress' && upgradeDialog.error">
          <el-button @click="upgradeDialog.visible = false">关闭</el-button>
        </template>
      </span>
    </el-dialog>
  </div>
</template>

<script>
import axios from "axios";

export default {
  name: 'Monitor',
  created() {
    this.$emit('update:route_path', this.$route.path)
    this.$emit('update:route_name', ['系统设置', '系统信息'])
  },
  mounted() {
    this.getData();
  },
  data() {
    return {
      system: {},
      uptimeSeconds: 0,
      checkingUpdate: false,
      upgradeInfo: null,
      upgradeDialog: {
        visible: false,
        step: 'info',    // info / progress / done
        info: null,
        running: false,
        progress: 0,
        error: '',
        stage: '',
      },
      reconnectTimer: null,
    }
  },
  computed: {
    formattedUptime() {
      const s = Math.max(0, Math.floor(this.uptimeSeconds || 0));
      const d = Math.floor(s / 86400);
      const h = Math.floor((s % 86400) / 3600);
      const m = Math.floor((s % 3600) / 60);
      const parts = [];
      if (d > 0) parts.push(d + ' 天');
      if (h > 0) parts.push(h + ' 小时');
      parts.push(m + ' 分钟');
      return parts.join(' ');
    },
    stageText() {
      const map = {
        downloading: '正在下载...',
        replacing: '正在替换文件...',
        restarting: '正在重启服务...',
        done: '升级完成',
      }
      return map[this.upgradeDialog.stage] || this.upgradeDialog.stage
    }
  },
  beforeDestroy() {
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
  },
  methods: {
    getData() {
      axios.get('/set/system', {}).then(resp => {
        this.system = resp.data.data;
        this.uptimeSeconds = (this.system.sys && this.system.sys.uptime) || 0;
      }).catch(() => {
        this.$message.error('请求出错');
      });
    },
    progressColor(percent) {
      if (percent >= 90) return 'var(--color-danger)';
      if (percent >= 70) return 'var(--color-warning)';
      return 'var(--color-success)';
    },
    checkUpdate() {
      this.checkingUpdate = true;
      axios.get('/set/upgrade/check').then(resp => {
        const data = resp.data.data;
        if (data.need_upgrade) {
          this.upgradeInfo = data;
          // 有更新，弹出升级对话框
          this.upgradeDialog.visible = true;
          this.upgradeDialog.step = 'info';
          this.upgradeDialog.info = data;
          this.upgradeDialog.running = false;
          this.upgradeDialog.progress = 0;
          this.upgradeDialog.error = '';
        } else {
          this.upgradeInfo = null;
          this.$message.success('当前已是最新版本');
        }
      }).catch((error) => {
        const msg = error.response && error.response.data ? error.response.data.msg : '检查更新失败';
        this.$message.error(msg);
      }).finally(() => {
        this.checkingUpdate = false;
      });
    },
    startUpgrade() {
      this.upgradeDialog.step = 'progress';
      this.upgradeDialog.running = true;
      this.upgradeDialog.progress = 0;
      this.upgradeDialog.error = '';

      // 使用 fetch + 手动解析 SSE（HttpOnly Cookie 自动携带 JWT）
      const baseUrl = process.env.NODE_ENV === 'production' ? '' : 'https://192.168.8.24:8800';
      const url = baseUrl + '/set/upgrade/start';

      const self = this;
      fetch(url, {
        method: 'POST',
        credentials: 'include',
      }).then(response => {
        if (!response.ok) {
          // 401 等错误时响应体可能为空，安全解析
          return response.text().then(text => {
            let msg = '升级请求失败';
            if (text) {
              try { msg = JSON.parse(text).msg || msg; } catch (e) { /* ignore */ }
            }
            throw new Error(msg);
          });
        }

        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';

        function processChunk() {
          reader.read().then(({ done, value }) => {
            if (done) return;

            buffer += decoder.decode(value, { stream: true });
            const lines = buffer.split('\n');
            buffer = lines.pop() || '';

            for (const line of lines) {
              if (line.startsWith('data: ')) {
                try {
                  const data = JSON.parse(line.slice(6));
                  self.upgradeDialog.progress = data.progress;
                  self.upgradeDialog.stage = data.stage;

                  if (data.stage === 'error') {
                    self.upgradeDialog.error = data.error;
                    self.upgradeDialog.running = false;
                  }
                  if (data.stage === 'done') {
                    self.upgradeDialog.step = 'done';
                    self.upgradeDialog.running = false;
                    // 重启后尝试重连
                    self.startReconnect();
                  }
                } catch (e) {
                  // 忽略单行解析错误，继续处理后续行
                }
              }
            }
            processChunk();
          }).catch(() => {
          });
        }
        processChunk();
      }).catch((error) => {
        self.upgradeDialog.error = error.message || '升级连接失败';
        self.upgradeDialog.running = false;
      });
    },
    startReconnect() {
      const self = this;
      let attempts = 0;
      const maxAttempts = 20;

      function tryReconnect() {
        attempts++;
        setTimeout(() => {
          axios.get('/status.html').then(() => {
            // 服务已恢复，关闭对话框刷新页面
            self.upgradeDialog.visible = false;
            self.$message.success('升级完成，服务已重启');
            setTimeout(() => location.reload(), 1000);
          }).catch(() => {
            if (attempts < maxAttempts) {
              tryReconnect();
            } else {
              self.upgradeDialog.visible = false;
              self.$message.warning('服务恢复超时，请手动刷新页面');
            }
          });
        }, 2000);
      }
      tryReconnect();
    },
  }
}
</script>

<style scoped>
.system-page {
  padding: 4px 0;
}

/* 统计卡片 — System 页面特有（卡片更高以容纳双行信息） */
.stat-card {
  min-height: 84px;
}

.stat-icon-cpu {
  background: var(--color-primary-bg);
  color: var(--color-primary);
}

.stat-icon-mem {
  background: var(--success-bg);
  color: var(--color-success);
}

.stat-icon-disk {
  background: var(--warning-bg);
  color: var(--color-warning);
}

.stat-icon-uptime {
  background: var(--info-bg);
  color: var(--color-info);
}

/* 资源详情卡片 */
.detail-row {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr;
  gap: 16px;
  margin-bottom: 16px;
}

.detail-row .info-card {
  display: flex;
  flex-direction: column;
}

.info-card {
  border-radius: var(--card-radius);
  overflow: hidden;
  border: 1px solid var(--border-color-light);
  text-align: center;
}

.info-card ::v-deep .el-card__body {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 20px;
}

.info-card--flat {
  text-align: left;
}

.info-card--flat ::v-deep .el-card__body {
  padding: 16px 20px;
}

.card-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 16px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.card-title i {
  color: var(--color-primary);
  font-size: 16px;
}

.info-list {
  margin-top: 16px;
  text-align: left;
}

.info-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 0;
  border-bottom: 1px solid #f5f5f5;
}

.info-item:last-child {
  border-bottom: none;
  padding-bottom: 0;
}

.info-item:first-child {
  padding-top: 0;
}

.info-label {
  font-size: 13px;
  color: var(--text-secondary);
  display: flex;
  align-items: center;
  gap: 6px;
}

.info-label i {
  font-size: 13px;
  color: var(--text-placeholder);
}

.info-value {
  font-size: 13px;
  color: var(--text-primary);
  font-weight: 500;
}

.info-value.code {
  font-family: Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  color: var(--text-regular);
}

.model-name {
  font-size: 11px;
  color: var(--text-secondary);
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Grid for flat cards */
.info-grid {
  display: grid;
  gap: 8px 24px;
}

.info-grid--two {
  grid-template-columns: repeat(2, 1fr);
}

.info-grid--one {
  grid-template-columns: 1fr;
}

.info-grid .info-item {
  padding: 8px 0;
  border-bottom: 1px solid #f5f5f5;
}

.info-grid .info-item:last-child {
  border-bottom: none;
}

.bottom-row {
  display: grid;
  grid-template-columns: 14fr 10fr;
  gap: 16px;
  margin-top: 16px;
}

.bottom-row .info-card {
  display: flex;
  flex-direction: column;
}

.bottom-row .info-card ::v-deep .el-card__body {
  flex: 1;
  display: flex;
  flex-direction: column;
}

/* 环形图：隐藏原生文字，用自定义 overlay 居中 */
.progress-wrap {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 130px;
  height: 130px;
  margin: 0 auto;
}

.progress-wrap ::v-deep .el-progress-circle__track,
.progress-wrap ::v-deep .el-progress-circle__path {
  stroke-linecap: round;
}

.progress-num {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  font-size: 24px;
  font-weight: 700;
  pointer-events: none;
  line-height: 1;
}

@media (max-width: 1200px) {
  .stats-row {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 1024px) {
  .detail-row {
    grid-template-columns: 1fr;
  }

  .bottom-row {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 768px) {
  .stats-row {
    grid-template-columns: 1fr;
  }

  .info-grid--two {
    grid-template-columns: 1fr;
  }
}

/* 升级按钮和标签 */
.upgrade-check-btn {
  margin-left: 12px;
  padding: 0;
  font-size: 12px;
}

.upgrade-tag {
  margin-left: 8px;
}

/* 升级对话框 */
.upgrade-body {
  padding: 0 4px;
}

.upgrade-version-row {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 24px;
  margin-bottom: 24px;
  padding: 20px 0;
  background: var(--bg-header);
  border-radius: 8px;
}

.upgrade-version-item {
  text-align: center;
}

.upgrade-version-label {
  font-size: 13px;
  color: var(--text-secondary);
  margin-bottom: 8px;
}

.upgrade-version-value {
  font-size: 20px;
  font-weight: 700;
  color: var(--text-primary);
  font-family: Menlo, Monaco, Consolas, monospace;
}

.upgrade-version-new .upgrade-version-value {
  color: var(--color-primary);
}

.upgrade-arrow {
  font-size: 20px;
  color: var(--text-placeholder);
}

.upgrade-changelog {
  border-top: 1px solid var(--border-color-light);
  padding-top: 16px;
}

.upgrade-changelog-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 8px;
}

.upgrade-changelog-body {
  font-size: 13px;
  color: var(--text-regular);
  line-height: 1.8;
  white-space: pre-wrap;
  max-height: 200px;
  overflow-y: auto;
}

/* 升级进度 */
.upgrade-progress-wrap {
  text-align: center;
  padding: 20px 0;
}

.upgrade-stage-text {
  display: block;
  margin-top: 16px;
  font-size: 14px;
  color: var(--text-regular);
}

.upgrade-error {
  margin-top: 16px;
  padding: 12px 16px;
  background: var(--danger-bg);
  border: 1px solid var(--danger-bg);
  border-radius: 4px;
  color: var(--color-danger);
  font-size: 13px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.upgrade-error i {
  font-size: 16px;
}

/* 升级完成 */
.upgrade-done {
  text-align: center;
  padding: 32px 0;
}

.upgrade-done i {
  font-size: 56px;
  color: var(--color-success);
  margin-bottom: 16px;
}

.upgrade-done-text {
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary);
  margin-top: 16px;
}

.upgrade-done-sub {
  font-size: 13px;
  color: var(--text-secondary);
  margin-top: 8px;
}
</style>