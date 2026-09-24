package dbdata

import (
	"testing"
	"time"
)

func TestIsLeader(t *testing.T) {
	now := time.Now().Unix()
	alive := func(id string, ago int64) *ClusterNode {
		return &ClusterNode{Id: id, LastSeen: now - ago}
	}
	cases := []struct {
		name   string
		nodes  []*ClusterNode
		selfId string
		want   bool
	}{
		{"空清单(单节点/SQLite)=leader", nil, "n1", true},
		{"仅自己=leader", []*ClusterNode{alive("n1", 0)}, "n1", true},
		{"自身 Id 最小=leader", []*ClusterNode{alive("n1", 0), alive("n2", 0), alive("n3", 0)}, "n1", true},
		{"对端 Id 更小=follower", []*ClusterNode{alive("n1", 0), alive("n2", 0)}, "n2", false},
		{"对端离线不计入→自身即唯一存活=leader", []*ClusterNode{alive("n1", 0), alive("n2", 200)}, "n1", true},
		{"自身心跳陈旧仍按运行态参选(对端更大 Id)=leader", []*ClusterNode{alive("n1", 200), alive("n2", 0)}, "n1", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isLeader(c.nodes, c.selfId, now); got != c.want {
				t.Errorf("isLeader()=%v want %v", got, c.want)
			}
		})
	}
}
