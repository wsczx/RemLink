/**
 * 系统日志 WebSocket 实时推送混入
 * 用法: 在 Syslog.vue 中混入
 */
export default {
  data() {
    return {
      syslogWs: null,
      syslogWsLive: true,
      syslogWsNode: '',              // 当前查看实时日志的节点（空=本机），与历史日志共用 historyNode
      syslogWsConnected: false,     // 响应式连接状态
      syslogWsReconnectTimer: null,
      syslogWsReconnectDelay: 3000,
    }
  },

  methods: {
    /** 建立 WebSocket 连接（node 为空表示本机，非空表示集群中指定节点） */
    syslogWsConnect(node) {
      if (this.syslogWs && this.syslogWs.readyState === WebSocket.OPEN) return
      this.syslogWsNode = node || ''
      const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
      const query = this.syslogWsNode ? '?node=' + encodeURIComponent(this.syslogWsNode) : ''
      const wsUrl = `${protocol}//${location.host}/cluster/syslog/ws${query}`
      try {
        this.syslogWs = new WebSocket(wsUrl)
        this.syslogWs.onopen = this.onSyslogWsOpen
        this.syslogWs.onmessage = this.onSyslogWsMessage
        this.syslogWs.onclose = this.onSyslogWsClose
        this.syslogWs.onerror = this.onSyslogWsError
      } catch (e) {
        this.syslogWsScheduleReconnect()
      }
    },

    /** 断开 WebSocket */
    syslogWsDisconnect() {
      if (this.syslogWsReconnectTimer) {
        clearTimeout(this.syslogWsReconnectTimer)
        this.syslogWsReconnectTimer = null
      }
      if (this.syslogWs) {
        this.syslogWs.onclose = null
        this.syslogWs.close()
        this.syslogWs = null
      }
    },

    /** 切换实时/暂停 */
    syslogWsToggleLive() {
      this.syslogWsLive = !this.syslogWsLive
    },

    /** WebSocket 连接建立 */
    onSyslogWsOpen() {
      this.syslogWsConnected = true
      this.syslogWsReconnectDelay = 3000
    },

    /** 收到消息 */
    onSyslogWsMessage(event) {
      if (!this.syslogWsLive) return
      try {
        const msg = JSON.parse(event.data)
        if (msg.type === 'syslog' && msg.data) {
          this.onSyslogEntry(msg.data)
        }
      } catch (e) {
        // 忽略解析失败的消息
      }
    },

    /** 实际处理日志条目（由页面组件覆盖实现） */
    onSyslogEntry(/* entry */) {
      // 页面组件应覆盖此方法
    },

    /** WebSocket 断开，自动重连 */
    onSyslogWsClose() {
      this.syslogWsConnected = false
      this.syslogWs = null
      this.syslogWsScheduleReconnect()
    },

    /** WebSocket 错误 */
    onSyslogWsError() {
      // onclose 会随后触发，由 onSyslogWsClose 处理重连
    },

    /** 调度重连 */
    syslogWsScheduleReconnect() {
      if (this.syslogWsReconnectTimer) return
      this.syslogWsReconnectTimer = setTimeout(() => {
        this.syslogWsReconnectTimer = null
        if (this.syslogWsLive) {
          this.syslogWsConnect(this.syslogWsNode)
        }
      }, this.syslogWsReconnectDelay)
      this.syslogWsReconnectDelay = Math.min(this.syslogWsReconnectDelay * 1.5, 30000)
    }
  },

  beforeDestroy() {
    this.syslogWsDisconnect()
  }
}
