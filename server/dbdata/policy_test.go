package dbdata

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPolicySetAndLoad(t *testing.T) {
	ast := assert.New(t)

	preIpData(t)
	defer closeIpdata()

	p1 := Policy{Name: "test-p1", Status: 1, ClientDns: []ValData{{Val: "114.114.114.114"}}, DsExcludeDomains: "baidu.com,163.com"}
	err := SetPolicy(&p1)
	ast.Nil(err)

	p2 := Policy{Name: "test-p2", Status: 1, ClientDns: []ValData{{Val: "114.114.114.114"}}, DsExcludeDomains: "com.cn,qq.com"}
	err = SetPolicy(&p2)
	ast.Nil(err)

	route := []ValData{{Val: "192.168.1.0/24"}}
	p3 := Policy{Name: "test-p3", Status: 1, ClientDns: []ValData{{Val: "114.114.114.114"}}, RouteInclude: route, DsExcludeDomains: "com.cn,qq.com"}
	err = SetPolicy(&p3)
	ast.Nil(err)
	ast.Equal(p3.RouteInclude[0].IpMask, "192.168.1.0/255.255.255.0")

	route2 := []ValData{{Val: "192.168.2.0/24"}}
	p4 := Policy{Name: "test-p4", Status: 1, ClientDns: []ValData{{Val: "114.114.114.114"}}, RouteInclude: route2, RouteExclude: route2, DsIncludeDomains: "com.cn,qq.com"}
	err = SetPolicy(&p4)
	ast.Nil(err)
	ast.Equal(p4.RouteExclude[0].IpMask, "192.168.2.0/255.255.255.0")

	pAll := []string{"test-p1", "test-p2", "test-p3", "test-p4"}
	for _, name := range pAll {
		pt := &Policy{}
		err := One("Name", name, pt)
		ast.Nil(err)
		ast.NotEqual(pt.Id, 0)
		ast.Equal(name, pt.Name)
	}
}

func TestPolicyTrafficQuotaValidate(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()

	// 配额为负数
	err := SetPolicy(&Policy{Name: "t-q1", Status: 1, TrafficQuota: -1, ClientDns: []ValData{{Val: "8.8.8.8"}}})
	ast.NotNil(err)
	ast.Contains(err.Error(), "流量配额不能小于0")

	// 上行带宽为负数
	err = SetPolicy(&Policy{Name: "t-q2", Status: 1, BandwidthUp: -1, ClientDns: []ValData{{Val: "8.8.8.8"}}})
	ast.NotNil(err)
	ast.Contains(err.Error(), "上行带宽")

	// 重置周期非法
	err = SetPolicy(&Policy{Name: "t-q3", Status: 1, TrafficQuota: 1024, TrafficReset: "yearly", ClientDns: []ValData{{Val: "8.8.8.8"}}})
	ast.NotNil(err)
	ast.Contains(err.Error(), "重置周期")

	// 配额 > 0 且未指定重置周期，默认 monthly
	p := &Policy{Name: "t-q4", Status: 1, TrafficQuota: 1024, ClientDns: []ValData{{Val: "8.8.8.8"}}}
	err = SetPolicy(p)
	ast.Nil(err)
	ast.Equal("monthly", p.TrafficReset)

	// 配额 = 0，重置周期清空
	p0 := &Policy{Name: "t-q5", Status: 1, TrafficQuota: 0, TrafficReset: "daily", ClientDns: []ValData{{Val: "8.8.8.8"}}}
	err = SetPolicy(p0)
	ast.Nil(err)
	ast.Equal("", p0.TrafficReset)
}

func TestNextTrafficReset(t *testing.T) {
	ast := assert.New(t)

	// daily: 2026-07-05 13:00 → 2026-07-06 00:00
	t1 := time.Date(2026, 7, 5, 13, 0, 0, 0, time.Local)
	next := nextTrafficReset("daily", t1)
	ast.Equal(time.Date(2026, 7, 6, 0, 0, 0, 0, time.Local), next)

	// weekly: 2026-07-05(周日) → 2026-07-06(周一) 00:00
	t2 := time.Date(2026, 7, 5, 13, 0, 0, 0, time.Local) // 周日
	next = nextTrafficReset("weekly", t2)
	ast.Equal(time.Date(2026, 7, 6, 0, 0, 0, 0, time.Local), next)

	// weekly: 2026-07-06(周一) → 2026-07-13(下周一) 00:00
	t3 := time.Date(2026, 7, 6, 13, 0, 0, 0, time.Local) // 周一
	next = nextTrafficReset("weekly", t3)
	ast.Equal(time.Date(2026, 7, 13, 0, 0, 0, 0, time.Local), next)

	// monthly: 2026-07-05 → 2026-08-01 00:00
	t4 := time.Date(2026, 7, 5, 13, 0, 0, 0, time.Local)
	next = nextTrafficReset("monthly", t4)
	ast.Equal(time.Date(2026, 8, 1, 0, 0, 0, 0, time.Local), next)

	// 未知周期
	next = nextTrafficReset("unknown", t4)
	ast.True(next.IsZero())
}

// 流量配额检查与累加
func TestQuotaExceededAndAdd(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()

	pGroup := &Policy{Name: "q-group-policy", Status: 1, ClientDns: []ValData{{Val: "8.8.8.8"}}}
	_ = SetPolicy(pGroup)
	_ = SetGroup(&Group{Name: "qg1", PolicyId: pGroup.Id, Status: 1})

	u := &User{Username: "quota-user", Groups: []string{"qg1"}, Status: 1, PinCode: "123456"}
	err := SetUser(u)
	ast.Nil(err)

	p := &Policy{Name: "quota-policy", Status: 1, TrafficQuota: 1000, TrafficReset: "monthly",
		ClientDns: []ValData{{Val: "8.8.8.8"}}}
	err = SetPolicy(p)
	ast.Nil(err)

	// 首次检查：未超
	exceeded, used := QuotaExceeded("quota-user", p)
	ast.False(exceeded)
	ast.Equal(int64(0), used)

	AddTrafficUsed("quota-user", 500)
	exceeded, used = QuotaExceeded("quota-user", p)
	ast.False(exceeded)
	ast.Equal(int64(500), used)

	// 再累加 500 字节，达到配额
	AddTrafficUsed("quota-user", 500)
	exceeded, used = QuotaExceeded("quota-user", p)
	ast.True(exceeded)
	ast.Equal(int64(1000), used)

	u2 := &User{}
	_ = One("Username", "quota-user", u2)
	ast.NotNil(u2.TrafficResetAt)
	ast.True(u2.TrafficResetAt.After(time.Now()))

	// 配额 = 0，永远不超
	pNoQuota := &Policy{Name: "no-quota", Status: 1, ClientDns: []ValData{{Val: "8.8.8.8"}}}
	_ = SetPolicy(pNoQuota)
	exceeded, _ = QuotaExceeded("quota-user", pNoQuota)
	ast.False(exceeded)

	// nil 策略
	exceeded, _ = QuotaExceeded("quota-user", nil)
	ast.False(exceeded)

	// 不存在的用户
	exceeded, _ = QuotaExceeded("non-existent-user", p)
	ast.False(exceeded)
}

// 并发累加安全性
func TestAddTrafficUsedConcurrent(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()

	pGroup := &Policy{Name: "conc-group-policy", Status: 1, ClientDns: []ValData{{Val: "8.8.8.8"}}}
	_ = SetPolicy(pGroup)
	_ = SetGroup(&Group{Name: "cg1", PolicyId: pGroup.Id, Status: 1})

	u := &User{Username: "concurrent-user", Groups: []string{"cg1"}, Status: 1, PinCode: "123456"}
	err := SetUser(u)
	ast.Nil(err)

	p := &Policy{Name: "conc-policy", Status: 1, TrafficQuota: 100000, TrafficReset: "monthly",
		ClientDns: []ValData{{Val: "8.8.8.8"}}}
	_ = SetPolicy(p)

	// 并发 100 次，每次累加 100 字节
	done := make(chan struct{}, 100)
	for range 100 {
		go func() {
			AddTrafficUsed("concurrent-user", 100)
			done <- struct{}{}
		}()
	}
	for range 100 {
		<-done
	}

	u2 := &User{}
	_ = One("Username", "concurrent-user", u2)
	ast.Equal(int64(10000), u2.TrafficUsed)
}

func TestResetTrafficUsed(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()

	pGroup := &Policy{Name: "reset-group-policy", Status: 1, ClientDns: []ValData{{Val: "8.8.8.8"}}}
	_ = SetPolicy(pGroup)
	_ = SetGroup(&Group{Name: "rg1", PolicyId: pGroup.Id, Status: 1})

	u := &User{Username: "reset-user", Groups: []string{"rg1"}, Status: 1, PinCode: "123456"}
	_ = SetUser(u)

	p := &Policy{Name: "reset-policy", Status: 1, TrafficQuota: 1000, TrafficReset: "daily",
		ClientDns: []ValData{{Val: "8.8.8.8"}}}
	_ = SetPolicy(p)

	AddTrafficUsed("reset-user", 500)

	err := ResetTrafficUsed("reset-user", p)
	ast.Nil(err)

	u2 := &User{}
	_ = One("Username", "reset-user", u2)
	ast.Equal(int64(0), u2.TrafficUsed)
	ast.NotNil(u2.TrafficResetAt)

	// 重置周期到达后 QuotaExceeded 应触发重置
	pastTime := time.Now().Add(-1 * time.Hour)
	_, err = xdb.Where("username=?", "reset-user").
		Cols("traffic_reset_at").Update(&User{TrafficResetAt: &pastTime})
	ast.Nil(err)

	// 再累加一些
	AddTrafficUsed("reset-user", 200)

	// 此时 QuotaExceeded 应触发重置并返回 false
	exceeded, _ := QuotaExceeded("reset-user", p)
	ast.False(exceeded)

	u3 := &User{}
	_ = One("Username", "reset-user", u3)
	ast.NotNil(u3.TrafficResetAt)
	ast.True(u3.TrafficResetAt.After(time.Now()))
}
