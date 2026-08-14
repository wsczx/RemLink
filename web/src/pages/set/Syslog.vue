<template>
  <div class="syslog-page">
    <div v-if="mode === 'live'" style="flex:1; display:flex; flex-direction:column; min-height:0; overflow:hidden">
      <div class="syslog-toolbar">
        <div class="toolbar-left">
          <el-switch v-model="syslogWsLive" active-text="实时" inactive-text="暂停" @change="onLiveToggle" size="small">
          </el-switch>
          <span class="conn-status" v-if="syslogWsLive" :class="{ connected: syslogWsConnected }">
            <span class="live-dot"></span>
            {{ syslogWsConnected ? '已连接' : '连接中...' }}
          </span>
          <span class="conn-status offline" v-else>已暂停</span>
          <el-select v-model="filterLevel" size="mini" placeholder="日志级别" clearable
            style="width: 100px; margin-left: 12px">
            <el-option label="Trace" value="Trace"></el-option>
            <el-option label="Debug" value="Debug"></el-option>
            <el-option label="Info" value="Info"></el-option>
            <el-option label="Warn" value="Warn"></el-option>
            <el-option label="Error" value="Error"></el-option>
            <el-option label="Fatal" value="Fatal"></el-option>
          </el-select>
          <el-input v-model="searchText" size="mini" placeholder="搜索关键字..." clearable
            style="width: 200px; margin-left: 8px">
            <i slot="prefix" class="el-icon-search"></i>
          </el-input>
          <el-button size="mini" :disabled="!historyEnabled" @click="switchToHistory" style="margin-left: 8px"
            :title="historyEnabled ? '查看历史日志' : '未配置日志文件路径，历史日志不可用'">
            历史日志
          </el-button>
        </div>
        <div class="toolbar-right">
          <span class="log-count">{{ filteredLogs.length }} / {{ logs.length }} 条</span>
          <el-button size="mini" type="text" @click="clearLogs" style="margin-left: 8px">清空</el-button>
          <el-button size="mini" type="text" @click="autoScroll = !autoScroll" style="margin-left: 4px">
            {{ autoScroll ? '锁定滚动' : '跟随滚动' }}
          </el-button>
        </div>
      </div>

      <div class="syslog-container" ref="logContainer" @scroll="onScroll">
        <div v-if="filteredLogs.length === 0" class="syslog-empty">
          <i class="el-icon-document"></i>
          <span>{{ logs.length === 0 ? '等待日志...' : '无匹配日志' }}</span>
          <p v-if="logs.length === 0 && !syslogWsConnected && syslogWsLive" class="empty-hint">正在建立连接...</p>
        </div>
        <div v-for="(entry, idx) in filteredLogs" :key="idx"
          :class="['syslog-line', 'level-' + entry.level.toLowerCase()]">
          <span class="log-time">{{ entry.time }}</span>
          <span class="log-level">{{ entry.level }}</span>
          <span class="log-msg" v-html="highlightLine(entry)"></span>
        </div>
      </div>
    </div>

    <div v-if="mode === 'history'" style="flex:1; display:flex; flex-direction:column; min-height:0; overflow:hidden">
      <div class="syslog-toolbar">
        <div class="toolbar-left">
          <el-date-picker v-model="historyDate" type="date" value-format="yyyy-MM-dd" size="mini" placeholder="选择日期"
            :clearable="false" @change="onHistoryFilterChange" style="margin-left: 4px">
          </el-date-picker>
          <el-select v-model="historyLevel" size="mini" placeholder="日志级别" clearable
            style="width: 100px; margin-left: 8px" @change="onHistoryFilterChange">
            <el-option label="Trace" value="Trace"></el-option>
            <el-option label="Debug" value="Debug"></el-option>
            <el-option label="Info" value="Info"></el-option>
            <el-option label="Warn" value="Warn"></el-option>
            <el-option label="Error" value="Error"></el-option>
            <el-option label="Fatal" value="Fatal"></el-option>
          </el-select>
          <el-input v-model="historyKeyword" size="mini" placeholder="搜索关键字..." clearable
            style="width: 200px; margin-left: 8px" @keyup.enter.native="onHistoryFilterChange"
            @clear="onHistoryFilterChange">
            <i slot="prefix" class="el-icon-search"></i>
          </el-input>
          <el-button size="mini" type="primary" icon="el-icon-search" @click="loadHistory(1)"
            style="margin-left: 8px">查询</el-button>
          <el-button size="mini" @click="switchToLive" style="margin-left: 8px">返回实时</el-button>
        </div>
        <div class="toolbar-right">
          <span class="log-count">{{ historyLogs.length }} 条（共 {{ historyTotal }} 条）</span>
        </div>
      </div>

      <div class="syslog-container" ref="historyContainer" @scroll="onHistoryScroll">
        <div v-if="historyLoading && historyLogs.length === 0" class="syslog-empty">
          <i class="el-icon-loading"></i>
          <span>加载中...</span>
        </div>
        <div v-else-if="historyLogs.length === 0" class="syslog-empty">
          <i class="el-icon-document"></i>
          <span>当日无日志或无匹配记录</span>
        </div>
        <div v-for="(entry, idx) in historyLogs" :key="idx"
          :class="['syslog-line', 'level-' + entry.level.toLowerCase()]">
          <span class="log-time">{{ entry.time }}</span>
          <span class="log-level">{{ entry.level }}</span>
          <span class="log-msg" v-html="highlightHistoryLine(entry)"></span>
        </div>
        <div v-if="historyLogs.length > 0 && historyLoading" class="syslog-loadmore">
          <i class="el-icon-loading"></i> 加载更多...
        </div>
        <div v-else-if="historyLogs.length > 0 && historyNoMore" class="syslog-loadmore">
          已经到底了
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import axios from "axios";
import syslogWsMixin from "../../mixins/syslogWs";

export default {
  name: "Syslog",
  mixins: [syslogWsMixin],
  mounted() {
    this.syslogWsConnect();
  },
  created() {
    this.$emit('update:route_path', this.$route.path)
    this.$emit('update:route_name', ['日志审计', '系统日志'])
    this.checkHistoryEnabled()
  },
  data() {
    return {
      logs: [],
      filterLevel: '',
      searchText: '',
      maxLogs: 2000,
      autoScroll: true,
      mode: 'live',
      historyEnabled: false,
      historyDate: '',
      historyLevel: '',
      historyKeyword: '',
      historyLogs: [],
      historyTotal: 0,
      historyPage: 1,
      historyPageSize: 200,
      historyLoading: false,
      historyNoMore: false,
    }
  },
  computed: {
    filteredLogs() {
      let list = this.logs
      if (this.filterLevel) {
        list = list.filter(l => l.level === this.filterLevel)
      }
      if (this.searchText) {
        const kw = this.searchText.toLowerCase()
        list = list.filter(l => l.msg.toLowerCase().includes(kw) || l.time.includes(kw))
      }
      return list
    }
  },
  watch: {
    filteredLogs() {
      if (this.autoScroll) {
        this.$nextTick(() => {
          this.scrollToBottom()
        })
      }
    }
  },
  methods: {
    onSyslogEntry(entry) {
      this.logs.push(entry)
      while (this.logs.length > this.maxLogs) {
        this.logs.shift()
      }
    },

    scrollToBottom() {
      const el = this.$refs.logContainer
      if (el) {
        el.scrollTop = el.scrollHeight
      }
    },

    clearLogs() {
      this.logs = []
    },

    onLiveToggle(val) {
      if (val) {
        this.syslogWsConnect()
      } else {
        this.syslogWsDisconnect()
      }
    },

    onScroll() {
      const el = this.$refs.logContainer
      if (!el) return
      // 滚到底部附近时恢复跟随
      const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 60
      if (nearBottom && !this.autoScroll) {
        this.autoScroll = true
      }
    },

    highlightLine(entry) {
      let text = this.escapeHtml(entry.msg)
      if (this.searchText) {
        const kw = this.escapeHtml(this.searchText)
        if (kw) {
          const re = new RegExp('(' + kw.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + ')', 'gi')
          text = text.replace(re, '<mark class="log-highlight">$1</mark>')
        }
      }
      return text
    },

    escapeHtml(str) {
      const div = document.createElement('div')
      div.textContent = str
      return div.innerHTML
    },

    highlightHistoryLine(entry) {
      let text = this.escapeHtml(entry.msg)
      if (this.historyKeyword) {
        const kw = this.escapeHtml(this.historyKeyword)
        if (kw) {
          const re = new RegExp('(' + kw.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + ')', 'gi')
          text = text.replace(re, '<mark class="log-highlight">$1</mark>')
        }
      }
      return text
    },

    async checkHistoryEnabled() {
      try {
        const resp = await axios.get('/set/syslog/history_enabled')
        if (resp.data && resp.data.code === 0) {
          this.historyEnabled = !!resp.data.data.enabled
          if (this.historyEnabled) {
            const today = new Date()
            this.historyDate = today.getFullYear() + '-' +
              String(today.getMonth() + 1).padStart(2, '0') + '-' +
              String(today.getDate()).padStart(2, '0')
          }
        }
      } catch (e) {
        this.historyEnabled = false
      }
    },

    switchToHistory() {
      if (!this.historyEnabled) return
      this.mode = 'history'
      this.loadHistory(1, true)
    },

    switchToLive() {
      this.mode = 'live'
    },

    onHistoryFilterChange() {
      this.loadHistory(1, true)
    },

    onHistoryScroll() {
      const el = this.$refs.historyContainer
      if (!el) return
      const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 60
      if (nearBottom && !this.historyLoading && !this.historyNoMore) {
        this.loadHistory(this.historyPage + 1)
      }
    },

    async loadHistory(page, reset) {
      if (!this.historyDate) return
      if (reset) {
        this.historyLogs = []
        this.historyNoMore = false
        this.historyPage = 1
      } else {
        this.historyPage = page
      }
      this.historyLoading = true
      try {
        const params = {
          date: this.historyDate,
          page: this.historyPage,
          page_size: this.historyPageSize,
        }
        if (this.historyLevel) params.level = this.historyLevel
        if (this.historyKeyword) params.keyword = this.historyKeyword
        const resp = await axios.get('/set/syslog/history_list', { params })
        if (resp.data && resp.data.code === 0) {
          const d = resp.data.data
          const datas = d.datas || []
          this.historyTotal = d.count || 0
          if (reset) {
            this.historyLogs = datas
            const el = this.$refs.historyContainer
            if (el) el.scrollTop = 0
          } else {
            this.historyLogs = this.historyLogs.concat(datas)
          }
          // 返回不足一页即视为已加载完所有匹配记录
          if (datas.length < this.historyPageSize) {
            this.historyNoMore = true
          }
        } else {
          this.$message.error((resp.data && resp.data.msg) || '加载历史日志失败')
          if (reset) {
            this.historyLogs = []
            this.historyTotal = 0
          }
        }
      } catch (e) {
        this.$message.error('加载历史日志失败：' + (e.message || e))
        if (reset) {
          this.historyLogs = []
          this.historyTotal = 0
        }
      } finally {
        this.historyLoading = false
      }
    },
  },
  beforeDestroy() {
    this.syslogWsDisconnect()
  }
}
</script>

<style scoped>
.syslog-page {
  height: calc(100vh - 140px);
  display: flex;
  flex-direction: column;
  background: #0d1117;
  border-radius: 6px;
  overflow: hidden;
  border: 1px solid #30363d;
}

/* 控制栏 */
.syslog-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 16px;
  background: #161b22;
  border-bottom: 1px solid #30363d;
  flex-shrink: 0;
}

.toolbar-left,
.toolbar-right {
  display: flex;
  align-items: center;
}

.conn-status {
  font-size: 12px;
  color: #8b949e;
  display: flex;
  align-items: center;
  gap: 4px;
  margin-left: 12px;
}

.conn-status.connected {
  color: #3fb950;
}

.conn-status.offline {
  color: #8b949e;
}

.live-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #8b949e;
}

.conn-status.connected .live-dot {
  background: #3fb950;
  animation: pulse 2s infinite;
}

@keyframes pulse {

  0%,
  100% {
    opacity: 1;
  }

  50% {
    opacity: 0.3;
  }
}

.log-count {
  font-size: 12px;
  color: #8b949e;
}

/* 日志容器 */
.syslog-container {
  flex: 1;
  overflow-y: auto;
  padding: 8px 0;
  font-family: 'SF Mono', 'Cascadia Code', 'Consolas', 'Courier New', 'Menlo', monospace;
  font-size: 13px;
  line-height: 1.7;
}

.syslog-container::-webkit-scrollbar {
  width: 6px;
}

.syslog-container::-webkit-scrollbar-track {
  background: transparent;
}

.syslog-container::-webkit-scrollbar-thumb {
  background: #30363d;
  border-radius: 3px;
}

.syslog-container::-webkit-scrollbar-thumb:hover {
  background: #484f58;
}

/* 空状态 */
.syslog-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: #484f58;
  font-size: 14px;
  gap: 8px;
}

.syslog-empty i {
  font-size: 40px;
  opacity: 0.5;
}

.empty-hint {
  font-size: 12px;
  color: #30363d;
  margin-top: 4px;
}

/* 日志行 */
.syslog-line {
  display: flex;
  padding: 2px 16px;
  white-space: pre-wrap;
  word-break: break-all;
  border-left: 3px solid transparent;
  transition: background 0.12s ease;
}

.syslog-line:hover {
  background: rgba(255, 255, 255, 0.03);
}

.log-time {
  color: #484f58;
  margin-right: 10px;
  flex-shrink: 0;
}

.log-level {
  width: 44px;
  flex-shrink: 0;
  margin-right: 10px;
  text-align: center;
  font-weight: 700;
  font-size: 11px;
  border-radius: 3px;
  padding: 0 4px;
  line-height: 1.5;
}

.log-msg {
  color: #c9d1d9;
}

/* 各级别颜色 — GitHub 风格 */
.level-trace {
  border-left-color: transparent;
}

.level-trace .log-level {
  color: #484f58;
}

.level-debug {
  border-left-color: #388bfd26;
}

.level-debug .log-level {
  color: #58a6ff;
}

.level-info {
  border-left-color: #3fb95026;
}

.level-info .log-level {
  color: #3fb950;
}

.level-warn {
  border-left-color: #d2992226;
}

.level-warn .log-level {
  color: #d29922;
}

.level-warn .log-msg {
  color: #e3b341;
}

.level-warn {
  background: rgba(210, 153, 34, 0.05);
}

.level-error {
  border-left-color: #f8514926;
}

.level-error .log-level {
  color: #f85149;
}

.level-error .log-msg {
  color: #f85149;
}

.level-error {
  background: rgba(248, 81, 73, 0.05);
}

.level-fatal {
  border-left-color: #f85149;
}

.level-fatal .log-level {
  color: var(--text-inverse);
  background: #da3633;
  border-radius: 3px;
  padding: 0 5px;
}

.level-fatal .log-msg {
  color: #ff7b72;
  font-weight: bold;
}

/* 历史日志加载更多提示 */
.syslog-loadmore {
  text-align: center;
  padding: 10px 0;
  color: #8b949e;
  font-size: 12px;
}
</style>

<style>
/* 日志搜索高亮 */
.log-highlight {
  background: #d2992226;
  color: #e3b341;
  border-radius: 2px;
  padding: 0 2px;
}
</style>
