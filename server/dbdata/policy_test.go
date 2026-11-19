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
	err := Policies.SetPolicy(&p1)
	ast.Nil(err)

	p2 := Policy{Name: "test-p2", Status: 1, ClientDns: []ValData{{Val: "114.114.114.114"}}, DsExcludeDomains: "com.cn,qq.com"}
	err = Policies.SetPolicy(&p2)
	ast.Nil(err)

	route := []ValData{{Val: "192.168.1.0/24"}}
	p3 := Policy{Name: "test-p3", Status: 1, ClientDns: []ValData{{Val: "114.114.114.114"}}, RouteInclude: route, DsExcludeDomains: "com.cn,qq.com"}
	err = Policies.SetPolicy(&p3)
	ast.Nil(err)
	ast.Equal(p3.RouteInclude[0].IpMask, "192.168.1.0/255.255.255.0")

	route2 := []ValData{{Val: "192.168.2.0/24"}}
	p4 := Policy{Name: "test-p4", Status: 1, ClientDns: []ValData{{Val: "114.114.114.114"}}, RouteInclude: route2, RouteExclude: route2, DsIncludeDomains: "com.cn,qq.com"}
	err = Policies.SetPolicy(&p4)
	ast.Nil(err)
	ast.Equal(p4.RouteExclude[0].IpMask, "192.168.2.0/255.255.255.0")

	pAll := []string{"test-p1", "test-p2", "test-p3", "test-p4"}
	for _, name := range pAll {
		pt := &Policy{}
		err := Policies.ByName(name, pt)
		ast.Nil(err)
		ast.NotEqual(pt.Id, 0)
		ast.Equal(name, pt.Name)
	}
}

func TestPolicyTrafficQuotaValidate(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()

	err := Policies.SetPolicy(&Policy{Name: "t-q1", Status: 1, TrafficQuota: -1, ClientDns: []ValData{{Val: "8.8.8.8"}}})
	ast.NotNil(err)
	ast.Contains(err.Error(), "流量配额不能小于0")

	err = Policies.SetPolicy(&Policy{Name: "t-q2", Status: 1, BandwidthUp: -1, ClientDns: []ValData{{Val: "8.8.8.8"}}})
	ast.NotNil(err)
	ast.Contains(err.Error(), "上行带宽")

	err = Policies.SetPolicy(&Policy{Name: "t-q3", Status: 1, TrafficQuota: 1024, TrafficReset: "yearly", ClientDns: []ValData{{Val: "8.8.8.8"}}})
	ast.NotNil(err)
	ast.Contains(err.Error(), "重置周期")

	p := &Policy{Name: "t-q4", Status: 1, TrafficQuota: 1024, ClientDns: []ValData{{Val: "8.8.8.8"}}}
	err = Policies.SetPolicy(p)
	ast.Nil(err)
	ast.Equal("monthly", p.TrafficReset)

	p0 := &Policy{Name: "t-q5", Status: 1, TrafficQuota: 0, TrafficReset: "daily", ClientDns: []ValData{{Val: "8.8.8.8"}}}
	err = Policies.SetPolicy(p0)
	ast.Nil(err)
	ast.Equal("", p0.TrafficReset)
}

func TestNextTrafficReset(t *testing.T) {
	ast := assert.New(t)

	t1 := time.Date(2026, 7, 5, 13, 0, 0, 0, time.Local)
	next := nextTrafficReset("daily", t1)
	ast.Equal(time.Date(2026, 7, 6, 0, 0, 0, 0, time.Local), next)

	t2 := time.Date(2026, 7, 5, 13, 0, 0, 0, time.Local)
	next = nextTrafficReset("weekly", t2)
	ast.Equal(time.Date(2026, 7, 6, 0, 0, 0, 0, time.Local), next)

	t3 := time.Date(2026, 7, 6, 13, 0, 0, 0, time.Local)
	next = nextTrafficReset("weekly", t3)
	ast.Equal(time.Date(2026, 7, 13, 0, 0, 0, 0, time.Local), next)

	t4 := time.Date(2026, 7, 5, 13, 0, 0, 0, time.Local)
	next = nextTrafficReset("monthly", t4)
	ast.Equal(time.Date(2026, 8, 1, 0, 0, 0, 0, time.Local), next)

	next = nextTrafficReset("unknown", t4)
	ast.True(next.IsZero())
}

// 流量配额检查与累加
func TestQuotaExceededAndAdd(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()

	pGroup := &Policy{Name: "q-group-policy", Status: 1, ClientDns: []ValData{{Val: "8.8.8.8"}}}
	_ = Policies.SetPolicy(pGroup)
	_ = Groups.SetGroup(&Group{Name: "qg1", PolicyId: pGroup.Id, Status: 1})

	u := &User{Username: "quota-user", Groups: []string{"qg1"}, Status: 1, PinCode: "123456"}
	err := Users.SetUser(u)
	ast.Nil(err)

	p := &Policy{Name: "quota-policy", Status: 1, TrafficQuota: 1000, TrafficReset: "monthly",
		ClientDns: []ValData{{Val: "8.8.8.8"}}}
	err = Policies.SetPolicy(p)
	ast.Nil(err)

	exceeded, used := Policies.QuotaExceeded("quota-user", p)
	ast.False(exceeded)
	ast.Equal(int64(0), used)
	Policies.AddTrafficUsed("quota-user", 500)
	exceeded, used = Policies.QuotaExceeded("quota-user", p)
	ast.False(exceeded)
	ast.Equal(int64(500), used)
	Policies.AddTrafficUsed("quota-user", 500)
	exceeded, used = Policies.QuotaExceeded("quota-user", p)
	ast.True(exceeded)
	ast.Equal(int64(1000), used)

	u2 := &User{}
	_ = Users.GetByUsername("quota-user", u2)
	ast.NotNil(u2.TrafficResetAt)
	ast.True(u2.TrafficResetAt.After(time.Now()))

	pNoQuota := &Policy{Name: "no-quota", Status: 1, ClientDns: []ValData{{Val: "8.8.8.8"}}}
	_ = Policies.SetPolicy(pNoQuota)
	exceeded, _ = Policies.QuotaExceeded("quota-user", pNoQuota)
	ast.False(exceeded)

	exceeded, _ = Policies.QuotaExceeded("quota-user", nil)
	ast.False(exceeded)

	exceeded, _ = Policies.QuotaExceeded("non-existent-user", p)
	ast.False(exceeded)
}

// 并发累加安全性
func TestAddTrafficUsedConcurrent(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()

	pGroup := &Policy{Name: "conc-group-policy", Status: 1, ClientDns: []ValData{{Val: "8.8.8.8"}}}
	_ = Policies.SetPolicy(pGroup)
	_ = Groups.SetGroup(&Group{Name: "cg1", PolicyId: pGroup.Id, Status: 1})

	u := &User{Username: "concurrent-user", Groups: []string{"cg1"}, Status: 1, PinCode: "123456"}
	err := Users.SetUser(u)
	ast.Nil(err)

	p := &Policy{Name: "conc-policy", Status: 1, TrafficQuota: 100000, TrafficReset: "monthly",
		ClientDns: []ValData{{Val: "8.8.8.8"}}}
	_ = Policies.SetPolicy(p)

	done := make(chan struct{}, 100)
	for range 100 {
		go func() {
			Policies.AddTrafficUsed("concurrent-user", 100)
			done <- struct{}{}
		}()
	}
	for range 100 {
		<-done
	}

	u2 := &User{}
	_ = Users.GetByUsername("concurrent-user", u2)
	ast.Equal(int64(10000), u2.TrafficUsed)
}

func TestResetTrafficUsed(t *testing.T) {
	ast := assert.New(t)
	preIpData(t)
	defer closeIpdata()

	pGroup := &Policy{Name: "reset-group-policy", Status: 1, ClientDns: []ValData{{Val: "8.8.8.8"}}}
	_ = Policies.SetPolicy(pGroup)
	_ = Groups.SetGroup(&Group{Name: "rg1", PolicyId: pGroup.Id, Status: 1})

	u := &User{Username: "reset-user", Groups: []string{"rg1"}, Status: 1, PinCode: "123456"}
	_ = Users.SetUser(u)

	p := &Policy{Name: "reset-policy", Status: 1, TrafficQuota: 1000, TrafficReset: "daily",
		ClientDns: []ValData{{Val: "8.8.8.8"}}}
	_ = Policies.SetPolicy(p)
	Policies.AddTrafficUsed("reset-user", 500)

	err := Policies.ResetTrafficUsed("reset-user", p)
	ast.Nil(err)

	u2 := &User{}
	_ = Users.GetByUsername("reset-user", u2)
	ast.Equal(int64(0), u2.TrafficUsed)
	ast.NotNil(u2.TrafficResetAt)

	pastTime := time.Now().Add(-1 * time.Hour)
	_, err = GetDB().Engine().Where("username=?", "reset-user").
		Cols("traffic_reset_at").Update(&User{TrafficResetAt: &pastTime})
	ast.Nil(err)
	Policies.AddTrafficUsed("reset-user", 200)

	exceeded, _ := Policies.QuotaExceeded("reset-user", p)
	ast.False(exceeded)

	u3 := &User{}
	_ = Users.GetByUsername("reset-user", u3)
	ast.NotNil(u3.TrafficResetAt)
	ast.True(u3.TrafficResetAt.After(time.Now()))
}
