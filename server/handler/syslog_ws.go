package handler

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wsczx/remlink/base"
)

// 系统日志条目（通过 WebSocket 推送）
type SyslogEntry struct {
	Level string `json:"level"` // Trace/Debug/Info/Warn/Error/Fatal
	Time  string `json:"time"`  // 2006-01-02 15:04:05
	Msg   string `json:"msg"`   // 日志内容（已去除末尾换行）
}

// SyslogMessage 广播消息结构
type SyslogMessage struct {
	Type string      `json:"type"`
	Data SyslogEntry `json:"data"`
}

var (
	syslogUpgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 4096,
		CheckOrigin: func(r *http.Request) bool {
			return true // 管理后台，允许所有来源
		},
	}

	syslogHub *SyslogHub

	// 系统日志广播通道（base → handler）
	syslogBroadcast = make(chan SyslogEntry, 2048)
)

// 单个 WebSocket 客户端连接
type SyslogClient struct {
	conn *websocket.Conn
	send chan []byte
	hub  *SyslogHub
}

// WebSocket 连接管理中心
type SyslogHub struct {
	clients    map[*SyslogClient]bool
	broadcast  chan []byte
	register   chan *SyslogClient
	unregister chan *SyslogClient
	mu         sync.RWMutex
}

// 创建系统日志 WebSocket Hub
func newSyslogHub() *SyslogHub {
	return &SyslogHub{
		clients:    make(map[*SyslogClient]bool),
		broadcast:  make(chan []byte, 512),
		register:   make(chan *SyslogClient),
		unregister: make(chan *SyslogClient),
	}
}

// 启动 Hub 主循环
func (h *SyslogHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			base.Debug("SyslogWS: 新客户端连接，当前连接数:", len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			base.Debug("SyslogWS: 客户端断开，当前连接数:", len(h.clients))

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// 客户端缓冲区满，断开
					go func(c *SyslogClient) {
						h.unregister <- c
					}(client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// 读取客户端消息
func (c *SyslogClient) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// 写入消息到客户端
func (c *SyslogClient) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// 启动系统日志实时推送 Hub
func StartSyslogHub() {
	syslogHub = newSyslogHub()
	go syslogHub.Run()
	go broadcastSyslogLoop()
}

// 监听系统日志广播通道
func broadcastSyslogLoop() {
	for entry := range syslogBroadcast {
		msg := SyslogMessage{
			Type: "syslog",
			Data: entry,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		select {
		case syslogHub.broadcast <- data:
		default:
			// 广播通道满则丢弃（非关键）
		}
	}
}

// 系统日志广播回调
func BroadcastSyslog(level int, msg string) {
	// 去除末尾换行
	l := len(msg)
	if l > 0 && msg[l-1] == '\n' {
		msg = msg[:l-1]
	}
	entry := SyslogEntry{
		Level: base.GetLogLevelName(level),
		Time:  time.Now().Format("2006-01-02 15:04:05"),
		Msg:   msg,
	}
	select {
	case syslogBroadcast <- entry:
	default:
	}
}

// WebSocket 升级处理函数
func HandleSyslogWS(w http.ResponseWriter, r *http.Request) {
	conn, err := syslogUpgrader.Upgrade(w, r, nil)
	if err != nil {
		base.Error("SyslogWS: WebSocket升级失败:", err)
		return
	}
	if syslogHub == nil {
		base.Error("SyslogWS: Hub 未初始化")
		conn.Close()
		return
	}
	client := &SyslogClient{
		conn: conn,
		send: make(chan []byte, 256),
		hub:  syslogHub,
	}
	syslogHub.register <- client

	go client.writePump()
	go client.readPump()
}
