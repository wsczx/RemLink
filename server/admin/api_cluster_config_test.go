package admin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/dbdata"
)

func TestSelectPeerNodes(t *testing.T) {
	ast := assert.New(t)
	now := int64(1000)
	self := "node-self"
	mk := func(id string, lastSeen int64) *dbdata.ClusterNode {
		return &dbdata.ClusterNode{Id: id, Name: id, AdminUrl: "https://" + id, LastSeen: lastSeen}
	}

	// 空清单
	ast.Empty(selectPeerNodes(nil, self, now))
	ast.Empty(selectPeerNodes([]*dbdata.ClusterNode{}, self, now))

	// 仅自身 → 无对端（单节点/SQLite 场景，不应发起集群调用）
	ast.Empty(selectPeerNodes([]*dbdata.ClusterNode{mk(self, now)}, self, now))

	// 自身 + 在线对端 → 仅对端
	peers := selectPeerNodes([]*dbdata.ClusterNode{mk(self, now), mk("b", now)}, self, now)
	ast.Len(peers, 1)
	ast.Equal("b", peers[0].Id)

	// 心跳 TTL 边界：now-lastSeen==90 仍算在线
	peers = selectPeerNodes([]*dbdata.ClusterNode{mk("b", now-90)}, self, now)
	ast.Len(peers, 1, "lastSeen 恰好 90s 前应仍在线")

	// 超过 TTL → 跳过（离线）
	ast.Empty(selectPeerNodes([]*dbdata.ClusterNode{mk("b", now-91)}, self, now))

	// 混合：自身 + 在线对端 + 离线对端
	peers = selectPeerNodes([]*dbdata.ClusterNode{
		mk(self, now),
		mk("online", now-10),
		mk("offline", now-200),
	}, self, now)
	ast.Len(peers, 1)
	ast.Equal("online", peers[0].Id)
}
