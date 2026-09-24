package base

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const clusterNodeFile = "conf/cluster_node.json"

// 本机节点身份；node_id/名称/管理地址在共享库后端持久化到本地文件（SQLite 单机不落盘，仅内存态）。
// 管理地址优先级：LINK_CLUSTER_URL（环境变量）> 本地手动配置 > 默认兜底
type localClusterNode struct {
	NodeId       string `json:"node_id"`
	Name         string `json:"name"`
	AdminUrl     string `json:"admin_url"`
	NeedsRestart bool   `json:"needs_restart"`
}

var (
	clusterMu     sync.RWMutex
	localCluster  localClusterNode
	clusterLoaded bool
)

// 加载/生成本节点身份
func LoadClusterNode() {
	clusterMu.Lock()
	defer clusterMu.Unlock()
	if clusterLoaded {
		return
	}
	lc := localClusterNode{}
	if data, err := os.ReadFile(clusterNodeFile); err == nil {
		_ = json.Unmarshal(data, &lc)
	}
	// 编排环境下用稳定环境变量锁定身份，避免容器重建随机生成 node_id 产生幽灵节点
	if id := os.Getenv("LINK_NODE_ID"); id != "" {
		lc.NodeId = id
	}
	if lc.NodeId == "" {
		lc.NodeId = genNodeId()
	}
	if name := os.Getenv("LINK_NODE_NAME"); name != "" {
		lc.Name = name
	} else if lc.Name == "" {
		if h, e := os.Hostname(); e == nil {
			lc.Name = h
		} else {
			lc.Name = lc.NodeId[:8]
		}
	}
	// 管理地址优先级在 GetNodeAdminUrl 中统一计算（LINK > 本地手动 > 默认），此处仅填默认兜底
	if lc.AdminUrl == "" {
		lc.AdminUrl = defaultClusterAdminUrl()
	}
	// 进程刚启动，清掉上次运行遗留的待重启标记；node_id 与名称写回本地文件
	lc.NeedsRestart = false
	localCluster = lc
	if err := saveLocalClusterNodeId(lc.NodeId); err != nil {
		Warn("save cluster node id:", err)
	}
	clusterLoaded = true
}

func saveLocalClusterNodeId(id string) error {
	// 单机 SQLite 禁跨节点共享
	if GetCfg().DbType == "sqlite3" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(clusterNodeFile), 0o755); err != nil {
		return err
	}
	// 持久化 node_id、名称、手动管理地址与待重启标记；管理地址以本地为准，共享库仅作镜像
	data, _ := json.Marshal(localClusterNode{
		NodeId:       id,
		Name:         localCluster.Name,
		AdminUrl:     localCluster.AdminUrl,
		NeedsRestart: localCluster.NeedsRestart,
	})
	return os.WriteFile(clusterNodeFile, data, 0o600)
}

func genNodeId() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(b)
}

// 默认兜底地址
func defaultClusterAdminUrl() string {
	return "https://" + GetCfg().AdminAddr
}

func GetNodeId() string {
	clusterMu.RLock()
	defer clusterMu.RUnlock()
	return localCluster.NodeId
}

func GetNodeName() string {
	clusterMu.RLock()
	defer clusterMu.RUnlock()
	return localCluster.Name
}

// 节点名称：后台可编辑并持久化；LINK_NODE_NAME 环境变量优先级更高
func SetNodeName(name string) {
	clusterMu.Lock()
	localCluster.Name = name
	clusterMu.Unlock()
	if err := saveLocalClusterNodeId(localCluster.NodeId); err != nil {
		Warn("save cluster node name:", err)
	}
}

// 管理地址解析优先级：LINK_CLUSTER_URL（环境变量）> 本地手动配置（UI，落本地文件）> 默认兜底
func GetNodeAdminUrl() string {
	clusterMu.RLock()
	defer clusterMu.RUnlock()
	if link := os.Getenv("LINK_CLUSTER_URL"); link != "" {
		return link
	}
	if localCluster.AdminUrl != "" {
		return localCluster.AdminUrl
	}
	return defaultClusterAdminUrl()
}

// 设置本节点手动管理地址（UI 校正），持久化到本地文件，优先级低于 LINK_CLUSTER_URL
func SetNodeAdminUrl(adminUrl string) {
	clusterMu.Lock()
	localCluster.AdminUrl = adminUrl
	clusterMu.Unlock()
	if err := saveLocalClusterNodeId(localCluster.NodeId); err != nil {
		Warn("save cluster admin url:", err)
	}
}

// 节点待重启标记：保存了需重启的配置项后置位，进程启动（LoadClusterNode）即清零，供总览显示「需重启」
func GetNeedsRestart() bool {
	clusterMu.RLock()
	defer clusterMu.RUnlock()
	return localCluster.NeedsRestart
}

// 标记本节点需要重启（保存了 restart:true 的配置项时由配置层调用），持久化到本地文件
func SetNeedsRestart(v bool) {
	clusterMu.Lock()
	localCluster.NeedsRestart = v
	clusterMu.Unlock()
	if err := saveLocalClusterNodeId(localCluster.NodeId); err != nil {
		Warn("save cluster needs restart:", err)
	}
}

var clusterBootTime int64

// 进程启动时间，供心跳上报 boot_time，详情面板展示「启动时间」
func SetNodeBootTime(t int64) { clusterBootTime = t }

func GetNodeBootTime() int64 { return clusterBootTime }
