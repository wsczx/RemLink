package dbdata

import (
	"net/url"
	"time"

	"github.com/wsczx/remlink/base"
)

// 节点注册表
// 本机身份（node_id）落本地文件，不进共享库，避免多节点互相覆盖
type ClusterNode struct {
	Id           string `xorm:"PK VARCHAR(64)"`
	Name         string `xorm:"VARCHAR(128)"`
	AdminUrl     string `xorm:"VARCHAR(512)"`
	Version      string `xorm:"VARCHAR(64)"`
	LastSeen     int64  `xorm:"BIGINT"`
	CreatedAt    int64  `xorm:"BIGINT"`
	BootTime     int64  `xorm:"BIGINT"`
	NeedsRestart bool   `xorm:"BOOL"`
}

func (ClusterNode) TableName() string { return "cluster_node" }

// 节点注册表：心跳续期 + 查询，落共享库
type ClusterNodeRepo struct{}

var clusterNodeRepo = &ClusterNodeRepo{}

func GetClusterNodeRepo() *ClusterNodeRepo { return clusterNodeRepo }

// 节点超过此时长未上报心跳即视为废弃并清理（默认 5 分钟，留 90s 在线窗口余量）
const clusterNodeStaleTTL = 5 * 60

// 启动心跳：注册本机并周期续期，同时清理过期节点
func (r *ClusterNodeRepo) Start() {
	base.LoadClusterNode()
	// 单机 SQLite 不跨节点共享，无需心跳注册与可达性告警
	if base.GetCfg().DbType == "sqlite3" {
		r.PruneStale() // 清理历史遗留的节点行，避免残留行被当作幽灵节点
		return
	}
	r.warnAdminUrl()
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		r.beat()
		r.PruneStale()
		for range ticker.C {
			r.beat()
			r.PruneStale()
		}
	}()
}

// 本机节点行：管理地址取 GetNodeAdminUrl 优先级结果（LINK>本地手动>默认）
func (r *ClusterNodeRepo) addSelfNode() *ClusterNode {
	return &ClusterNode{
		Id:           base.GetNodeId(),
		Name:         base.GetNodeName(),
		AdminUrl:     base.GetNodeAdminUrl(),
		Version:      base.APP_VER,
		LastSeen:     time.Now().Unix(),
		CreatedAt:    time.Now().Unix(),
		BootTime:     base.GetNodeBootTime(),
		NeedsRestart: base.GetNeedsRestart(),
	}
}

// 周期把本节点注册/续期到共享库，供其它节点发现与远程重启
func (r *ClusterNodeRepo) beat() {
	if err := r.Save(r.addSelfNode()); err != nil {
		base.Warn("cluster heartbeat:", err)
	}
}

// 编辑本机名称/管理地址后立即刷新
func (r *ClusterNodeRepo) SaveSelf() error {
	return r.Save(r.addSelfNode())
}

// 清理长期未心跳的节点行
func (r *ClusterNodeRepo) PruneStale() {
	cutoff := time.Now().Unix() - clusterNodeStaleTTL
	if _, err := GetXdb().Where("last_seen < ?", cutoff).Delete(&ClusterNode{}); err != nil {
		base.Warn("prune stale cluster nodes:", err)
	}
}

// 启动告警：本机 admin_url 为不可达地址时其它节点无法访问，多节点部署必须显式设置 LINK_CLUSTER_URL
func (r *ClusterNodeRepo) warnAdminUrl() {
	u := base.GetNodeAdminUrl()
	hu, err := url.Parse(u)
	if err != nil {
		return
	}
	switch hu.Hostname() {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0", "::", "":
		base.Warn("节点: 本节点 admin_url 为不可达地址(", hu.Hostname(), ")，其它节点无法访问；多节点部署请通过 LINK_CLUSTER_URL 设置可被其它节点访问的地址")
	}
}

// 按 id 取节点
func (r *ClusterNodeRepo) Get(id string) (*ClusterNode, error) {
	n := &ClusterNode{}
	has, err := GetXdb().ID(id).Get(n)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, nil
	}
	return n, nil
}

// 取全部节点（按注册时间与 id 排序）
func (r *ClusterNodeRepo) GetNodes() ([]*ClusterNode, error) {
	var list []ClusterNode
	if err := GetXdb().OrderBy("created_at ASC, id ASC").Find(&list); err != nil {
		return nil, err
	}
	out := make([]*ClusterNode, len(list))
	for i := range list {
		out[i] = &list[i]
	}
	return out, nil
}

// 集群单例任务选主：心跳表内存活（now-last_seen<=90）节点 Id 最小者为 leader
func isLeader(nodes []*ClusterNode, selfId string, now int64) bool {
	minId := selfId
	hasPeer := false
	for _, n := range nodes {
		if now-n.LastSeen > 90 {
			continue
		}
		if n.Id == selfId {
			continue
		}
		hasPeer = true
		if n.Id < minId {
			minId = n.Id
		}
	}
	if !hasPeer {
		return true
	}
	return selfId == minId
}

// 本机是否应执行单例定时任务
func IsClusterLeader() bool {
	nodes, err := GetClusterNodeRepo().GetNodes()
	if err != nil {
		base.Warn("集群 leader 判定失败，本机按 leader 执行:", err)
		return true
	}
	return isLeader(nodes, base.GetNodeId(), time.Now().Unix())
}

// 首次插入、之后仅刷新可变字段（保留 CreatedAt 用于排序）
func (r *ClusterNodeRepo) Save(n *ClusterNode) error {
	exist, err := GetXdb().ID(n.Id).Get(&ClusterNode{})
	if err != nil {
		return err
	}
	if exist {
		_, err = GetXdb().ID(n.Id).Update(&ClusterNode{
			Name:         n.Name,
			AdminUrl:     n.AdminUrl,
			Version:      n.Version,
			LastSeen:     n.LastSeen,
			BootTime:     n.BootTime,
			NeedsRestart: n.NeedsRestart,
		})
		return err
	}
	_, err = GetXdb().Insert(n)
	return err
}
