package admin

import (
	"crypto/tls"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

// WebSocket 系统日志实时推送处理函数，由 handler 包在启动时注入
var SyslogWSHandler func(w http.ResponseWriter, r *http.Request)

// WebSocket 系统日志实时推送入口（本机）
func SyslogWS(w http.ResponseWriter, r *http.Request) {
	if SyslogWSHandler != nil {
		SyslogWSHandler(w, r)
	}
}

// 实时日志 WebSocket 升级器（浏览器与本机、及对端与本机代理复用）
var syslogProxyUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true // 管理后台，允许所有来源
	},
}

// 将管理地址（http/https）转换为 WebSocket 地址（ws/wss）
func syslogWSScheme(adminUrl string) string {
	if after, ok := strings.CutPrefix(adminUrl, "https://"); ok {
		return "wss://" + after
	}
	if after, ok := strings.CutPrefix(adminUrl, "http://"); ok {
		return "ws://" + after
	}
	return adminUrl
}

// 集群实时日志：默认（无 node 或本机）
// 选定其它节点时由本节点作为代理，用节点间令牌建立到目标节点 /cluster/syslog/ws 的
// WebSocket 并双向转发
func ClusterSyslogWS(w http.ResponseWriter, r *http.Request) {
	nodeId := strings.TrimSpace(r.FormValue("node"))
	if nodeId == "" || nodeId == base.GetNodeId() {
		// 本机
		if SyslogWSHandler != nil {
			SyslogWSHandler(w, r)
		}
		return
	}

	nodes, err := dbdata.GetClusterNodeRepo().GetNodes()
	if err != nil {
		http.Error(w, "节点查询失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var target *dbdata.ClusterNode
	for _, n := range nodes {
		if n.Id == nodeId {
			target = n
			break
		}
	}
	if target == nil {
		http.Error(w, "节点不存在或已下线", http.StatusBadRequest)
		return
	}

	// 升级与本机浏览器的连接
	clientConn, err := syslogProxyUpgrader.Upgrade(w, r, nil)
	if err != nil {
		base.Error("ClusterSyslogWS: 浏览器连接升级失败:", err)
		return
	}

	// 生成节点间令牌，作为代理身份访问目标节点 /cluster/syslog/ws
	token, err := SetClusterJwtData()
	if err != nil {
		clientConn.Close()
		return
	}
	remoteURL := syslogWSScheme(strings.TrimRight(target.AdminUrl, "/")) + "/cluster/syslog/ws"
	header := http.Header{}
	header.Set("Jwt", token)
	dialer := &websocket.Dialer{
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true}, // 内网管控通道，与 clusterCli 一致
		HandshakeTimeout: 10 * time.Second,
	}
	remoteConn, _, err := dialer.Dial(remoteURL, header)
	if err != nil {
		base.Error("ClusterSyslogWS: 连接目标节点失败:", err)
		clientConn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "连接目标节点失败: "+err.Error()))
		clientConn.Close()
		return
	}

	done := make(chan struct{})
	// 对端 → 浏览器
	go func() {
		defer close(done)
		for {
			mt, data, rerr := remoteConn.ReadMessage()
			if rerr != nil {
				clientConn.Close() // 让主循环退出
				return
			}
			if werr := clientConn.WriteMessage(mt, data); werr != nil {
				return
			}
		}
	}()

	// 浏览器 → 对端（主要为心跳/控制帧）
	for {
		mt, data, cerr := clientConn.ReadMessage()
		if cerr != nil {
			break
		}
		if werr := remoteConn.WriteMessage(mt, data); werr != nil {
			break
		}
	}
	remoteConn.Close()
	<-done
}
