<template>
  <div class="syslog-page">
    <div v-if="mode === 'live'" style="flex:1; display:flex; flex-direction:column; min-height:0; overflow:hidden">
      <div class="syslog-toolbar">
        <div class="toolbar-left">
          <span class="mode-badge mode-live"><span class="mode-icon"></span>实时日志</span>
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
          <span class="mode-badge mode-history"><span class="mode-icon"></span>历史日志</span>
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
      const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 60
      if (nearBottom && !this.autoScroll) {
        this.autoScroll = true
      }
    },

    highlightLine(entry) {
      return this.highlightLogMessage(entry, this.searchText)
    },

    highlightHistoryLine(entry) {
      return this.highlightLogMessage(entry, this.historyKeyword)
    },

    escapeHtml(str) {
      if (!str) return '';
      return String(str)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;');
    },

    highlightLogMessage(entry, keyword) {
      const text = this.escapeHtml(entry.msg || '');
      const searchStr = keyword ? this.escapeHtml(keyword).replace(/[.*+?^${}()|[\]\\]/g, '\\$&') : '';
      const patterns = {
        search: searchStr ? `(${searchStr})` : null,
        statusError: `((?:DTLS.*?handshake error|DTLS.*?握手失败|\\bhandshake error\\b|握手失败|握手错误|MasterSecret is nil|tun Read err.*?file already closed))`,
        statusWarn: `((?:read hdata:.*?|read tun:.*?|i/o timeout|context deadline exceeded))`,
        statusSuccess: `((?:WebAuth认证已完成|用户通过证书认证|证书自动认证|\\blogin\\b|\\bAcquireIp\\b|\\bconnect\\b))`,
        group: `((?:组|group)\\s*[:=]?\\s*[\\w.-]+)`,
        identity: `((?:username|user_name|nickname|realname|name|user|用户|姓名|login)\\s*(?:[:=]\\s*)?[\\w.-]+(?:\\([\\w.-]+\\))?|\\b[\\w.-]+\\([\\w.-]+\\))`,
        url: `(https?:\\/\\/[\\w\\-]+(?:\\.[\\w\\-]+)+(?:[\\w\\-\\.,@?^=%&amp;:/~\\+#]*[\\w\\-@?^=%&amp;/~\\+#])?)`,
        address: `((?:(?:\\b(?:\\d{1,3}\\.){3}\\d{1,3}(?::\\d{1,5})?\\b)|(?:\\b[0-9a-f]{1,4}(?::[0-9a-f]{1,4}){2,7}\\b))(?:\\/\\d{1,3})?)`,
        domain: `(\\b(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\\.)+(?:[a-z]{2,63}|xn--[a-z0-9-]+)\\b)`,
        network: `((?:NAT6?|IPv6|DTLS|TUN|TAP|macvtap|nftables|WebVPN)\\b)`,
        keyword: `((?:\\b(?:Error|Exception|Failed|Denied|Timeout|Panic|Fatal)\\b|失败|错误|异常|拒绝|超时|中断|不可用|中止|致命|\\b(?:Warning|Warn)\\b|警告|注意))`
      };
      const regexParts = [];
      const types = [];
      Object.entries(patterns).forEach(([type, pattern]) => {
        if (pattern) {
          regexParts.push(pattern);
          types.push(type);
        }
      });
      const combinedRegex = new RegExp(regexParts.join('|'), 'gi');
      return text.replace(combinedRegex, (match, ...groups) => {
        const index = groups.findIndex(group => group !== undefined);
        const type = types[index];
        if (type === 'search') return `<mark class="log-highlight">${match}</mark>`;
        if (type === 'url') return `<a href="${match}" target="_blank" rel="noopener noreferrer" class="log-token log-url">${match}</a>`;
        if (type === 'identity') return `<span class="log-token log-identity">${match}</span>`;
        if (type === 'statusError') return `<span class="log-token log-status-error">${match}</span>`;
        if (type === 'statusWarn') return `<span class="log-token log-status-warn">${match}</span>`;
        if (type === 'statusSuccess') return `<span class="log-token log-status-success">${match}</span>`;
        if (type === 'group') return `<span class="log-token log-group">${match}</span>`;
        if (type === 'address') return `<span class="log-token log-address">${match}</span>`;
        if (type === 'domain') return `<span class="log-token log-domain">${match}</span>`;
        if (type === 'network') return `<span class="log-token log-network">${match}</span>`;
        if (type === 'keyword') {
          const lowerMatch = match.toLowerCase();
          const errorWords = ['error', 'exception', 'failed', 'denied', 'timeout', 'panic', 'fatal', '失败', '错误', '异常', '拒绝', '超时', '中断', '不可用', '中止', '致命'];
          return `<span class="log-token ${errorWords.includes(lowerMatch) ? 'log-kw-error' : 'log-kw-warn'}">${match}</span>`;
        }
        return match;
      });
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
      this.loadHistoryLastPage()
    },

    switchToLive() {
      this.mode = 'live'
    },

    onHistoryFilterChange() {
      this.loadHistoryLastPage()
    },

    onHistoryScroll() {
      const el = this.$refs.historyContainer
      if (!el) return
      const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 60
      if (nearBottom && !this.historyLoading && !this.historyNoMore) {
        this.loadHistory(this.historyPage + 1)
      }
    },

    async loadHistory(page, reset, scroll) {
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
            if (scroll) this.scrollHistoryToBottom()
          } else {
            this.historyLogs = this.historyLogs.concat(datas)
          }
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

    scrollHistoryToBottom() {
      const el = this.$refs.historyContainer
      if (!el) return
      const maxRetry = 8
      const tick = (n) => {
        el.scrollTop = el.scrollHeight
        if (n > 0) requestAnimationFrame(() => tick(n - 1))
      }
      requestAnimationFrame(() => tick(maxRetry))
    },

    async loadHistoryLastPage() {
      this.historyLoading = true
      try {
        this.historyLogs = []
        this.historyNoMore = false
        this.historyPage = 1
        for (; ;) {
          await this.loadHistory(this.historyPage, false, false)
          if (this.historyNoMore) break
          this.historyPage++
          if (this.historyPage > 5000) break
        }
        this.scrollHistoryToBottom()
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
/* 原有的组件框架样式保持不变... */
.syslog-page {
  height: calc(100vh - 140px);
  display: flex;
  flex-direction: column;
  background: #0d1117;
  border-radius: 6px;
  overflow: hidden;
  border: 1px solid #30363d;
}

.syslog-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 48px;
  padding: 8px 16px;
  background: linear-gradient(180deg, #1b222c 0%, #161b22 100%);
  border-bottom: 1px solid #30363d;
  box-shadow: 0 2px 12px rgba(0, 0, 0, .18);
  flex-shrink: 0;
}

.toolbar-left,
.toolbar-right {
  display: flex;
  align-items: center;
}

.mode-badge {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  min-width: 86px;
  margin-right: 14px;
  color: #c9d1d9;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: .2px;
}

.mode-icon {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #58a6ff;
  box-shadow: 0 0 0 3px #58a6ff1c;
}

.mode-history .mode-icon {
  background: #a371f7;
  box-shadow: 0 0 0 3px #a371f71c;
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

.syslog-line {
  display: flex;
  min-height: 27px;
  padding: 3px 16px 3px 13px;
  white-space: pre-wrap;
  word-break: break-word;
  border-left: 3px solid transparent;
  transition: background 0.12s ease, border-color 0.12s ease;
}

.syslog-line:hover {
  background: rgba(88, 166, 255, 0.07);
  border-left-color: #58a6ff66;
}

.log-time {
  width: 156px;
  color: #6e7681;
  margin-right: 12px;
  flex-shrink: 0;
  font-size: 12px;
}

.log-level {
  width: 48px;
  flex-shrink: 0;
  margin-right: 12px;
  text-align: center;
  font-weight: 700;
  font-size: 10px;
  letter-spacing: .35px;
  border: 1px solid currentColor;
  border-radius: 10px;
  padding: 1px 4px;
  line-height: 1.35;
  align-self: flex-start;
  margin-top: 1px;
}

.log-msg {
  color: #c9d1d9;
}

/* GitHub 风格的日志级别 */
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
  background: rgba(210, 153, 34, 0.05);
}

.level-warn .log-level {
  color: #d29922;
}

.level-warn .log-msg {
  color: #e3b341;
}

.level-error {
  border-left-color: #f8514926;
  background: rgba(248, 81, 73, 0.05);
}

.level-error .log-level {
  color: #f85149;
}

.level-error .log-msg {
  color: #f85149;
}

.level-fatal {
  border-left-color: #f85149;
}

.level-fatal .log-level {
  color: #fff;
  background: #da3633;
  border-color: #f85149;
  padding: 1px 5px;
}

.level-fatal .log-msg {
  color: #ff7b72;
  font-weight: bold;
}

.syslog-loadmore {
  text-align: center;
  padding: 10px 0;
  color: #8b949e;
  font-size: 12px;
}
</style>

<style>
.log-token {
  font-weight: 600;
}

.log-address {
  color: #f2cc60;
}

.log-url,
.log-domain {
  color: #79c0ff;
  text-decoration: underline;
  text-decoration-style: dotted;
  text-underline-offset: 2px;
}

.log-url {
  text-decoration-style: solid;
}

.log-url:hover {
  text-decoration-color: #58a6ff;
}

.log-identity {
  color: #ffa657;
}

.log-status-error {
  color: #ff7b72;
  font-weight: 700;
  background: rgba(248, 81, 73, 0.1);
  border-radius: 3px;
  padding: 0 3px;
}

.log-status-warn {
  color: #e3b341;
  font-weight: 650;
}

.log-status-success {
  color: #7ee787;
  font-weight: 650;
}

.log-group {
  color: #7ee787;
}

.log-network {
  color: #d2a8ff;
}

.log-kw-error {
  color: #ff7b72;
  font-weight: 700;
}

.log-kw-warn {
  color: #e3b341;
  font-weight: 700;
}

.log-highlight {
  padding: 1px 3px;
  color: #0d1117;
  background: #e3b341;
  border-radius: 3px;
  font-weight: 700;
  box-shadow: 0 0 0 1px rgba(210, 153, 34, 0.65);
}
</style>