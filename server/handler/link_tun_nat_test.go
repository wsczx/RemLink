package handler

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/sessdata"
)

type groupNATTestFirewall struct {
	mu      sync.Mutex
	v4Calls []groupNATCall
	v6Calls []groupNATCall
	v4Dels  []groupNATCall
	v6Dels  []groupNATCall
}

type groupNATCall struct {
	cidr   string
	egress string
}

func (f *groupNATTestFirewall) CreateChains(string, string) error { return nil }
func (f *groupNATTestFirewall) AddNatRule(string, string) error   { return nil }
func (f *groupNATTestFirewall) DelNatRule(string, string) error   { return nil }
func (f *groupNATTestFirewall) SetupGlobalNAT(string, string, bool) error {
	return nil
}
func (f *groupNATTestFirewall) SetupGlobalNAT6(string, string, bool) error {
	return nil
}
func (f *groupNATTestFirewall) AddGroupNAT(cidr, egress string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.v4Calls = append(f.v4Calls, groupNATCall{cidr: cidr, egress: egress})
	return nil
}
func (f *groupNATTestFirewall) AddGroupNAT6(cidr, egress string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.v6Calls = append(f.v6Calls, groupNATCall{cidr: cidr, egress: egress})
	return nil
}
func (f *groupNATTestFirewall) DelGroupNAT(cidr, egress string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.v4Dels = append(f.v4Dels, groupNATCall{cidr: cidr, egress: egress})
	return nil
}
func (f *groupNATTestFirewall) DelGroupNAT6(cidr, egress string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.v6Dels = append(f.v6Dels, groupNATCall{cidr: cidr, egress: egress})
	return nil
}
func (f *groupNATTestFirewall) CleanupFakeIP() error { return nil }
func (f *groupNATTestFirewall) CleanupGlobal() error { return nil }

func TestSetGroupNATFiltersByProtocol(t *testing.T) {
	originalCfg := *base.GetCfg()
	t.Cleanup(func() { base.UpdateCfg(func(cfg *base.ServerConfig) { *cfg = originalCfg }) })

	for i, tc := range []struct {
		name       string
		globalNat  bool
		globalNat6 bool
		wantV4     int
		wantV6     int
	}{
		{name: "both enabled", globalNat: true, globalNat6: true, wantV4: 1, wantV6: 1},
		{name: "only v4 enabled", globalNat: true, globalNat6: false, wantV4: 1},
		{name: "only v6 enabled", globalNat: false, globalNat6: true, wantV6: 1},
		{name: "both disabled", globalNat: false, globalNat6: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base.ReinitLog()
			fw := &groupNATTestFirewall{}
			installGroupNATTestFirewall(t, fw)
			base.UpdateCfg(func(cfg *base.ServerConfig) {
				cfg.GlobalNat = tc.globalNat
				cfg.GlobalNat6 = tc.globalNat6
				cfg.Ipv6CIDR = "2001:db8::/32"
			})
			group := &dbdata.Group{
				Name:          "engineering",
				ClientCidr:    "10.3." + string(rune('5'+i)) + ".0/24",
				ClientStart:   "10.3." + string(rune('5'+i)) + ".10",
				ClientEnd:     "10.3." + string(rune('5'+i)) + ".200",
				ClientGateway: "10.3." + string(rune('5'+i)) + ".1",
				ClientCidr6:   "2001:db8:" + string(rune('5'+i)) + "::/64",
				OutDev:        "lo",
			}
			setGroupNAT(&sessdata.ConnSession{
				IpPool: sessdata.GetGroupIpPool(group),
				Group:  group,
			})
			require.Len(t, fw.v4Calls, tc.wantV4)
			require.Len(t, fw.v6Calls, tc.wantV6)
			if tc.wantV4 == 1 {
				require.Equal(t, group.ClientCidr, fw.v4Calls[0].cidr)
				require.Equal(t, "lo", fw.v4Calls[0].egress)
			}
			if tc.wantV6 == 1 {
				require.Equal(t, group.ClientCidr6, fw.v6Calls[0].cidr)
				require.Equal(t, "lo", fw.v6Calls[0].egress)
			}
		})
	}
}

func TestRemoveGroupNATUsesTrackedEgress(t *testing.T) {
	originalCfg := *base.GetCfg()
	t.Cleanup(func() { base.UpdateCfg(func(cfg *base.ServerConfig) { *cfg = originalCfg }) })
	base.ReinitLog()
	base.UpdateCfg(func(cfg *base.ServerConfig) {
		cfg.GlobalNat = true
		cfg.GlobalNat6 = false
	})
	fw := &groupNATTestFirewall{}
	installGroupNATTestFirewall(t, fw)
	group := &dbdata.Group{Name: "engineering-remove", ClientCidr: "10.3.10.0/24", ClientStart: "10.3.10.10", ClientEnd: "10.3.10.200", ClientGateway: "10.3.10.1", OutDev: "lo"}
	sessdata.RemoveGroupNAT(group.ClientCidr, "", "")
	fw.v4Dels = nil
	sessdata.SyncGroupNAT(group.ClientCidr, "", "eth-old")
	sessdata.RemoveGroupNAT(group.ClientCidr, "", "eth-new")
	require.Equal(t, []groupNATCall{{cidr: group.ClientCidr, egress: "eth-old"}}, fw.v4Dels)
}

func TestSetGroupNATUsesMasterDevWhenGroupOutDevEmpty(t *testing.T) {
	originalCfg := *base.GetCfg()
	t.Cleanup(func() { base.UpdateCfg(func(cfg *base.ServerConfig) { *cfg = originalCfg }) })
	base.UpdateCfg(func(cfg *base.ServerConfig) {
		cfg.GlobalNat = true
		cfg.GlobalNat6 = false
		cfg.MasterDev = "lo"
	})

	group := &dbdata.Group{
		Name:          "engineering-master-dev",
		ClientCidr:    "10.3.9.0/24",
		ClientStart:   "10.3.9.10",
		ClientEnd:     "10.3.9.200",
		ClientGateway: "10.3.9.1",
	}
	fw := &groupNATTestFirewall{}
	installGroupNATTestFirewall(t, fw)
	setGroupNAT(&sessdata.ConnSession{
		IpPool: sessdata.GetGroupIpPool(group),
		Group:  group,
	})
	require.Equal(t, []groupNATCall{{cidr: group.ClientCidr, egress: "lo"}}, fw.v4Calls)
}

func installGroupNATTestFirewall(t *testing.T, fw *groupNATTestFirewall) {
	sessdata.GlobalFirewallMu.Lock()
	oldFirewall := sessdata.GlobalFirewall
	oldDone := sessdata.GlobalFirewallDone
	sessdata.GlobalFirewall = fw
	sessdata.GlobalFirewallDone = true
	sessdata.GlobalFirewallMu.Unlock()
	t.Cleanup(func() {
		sessdata.GlobalFirewallMu.Lock()
		sessdata.GlobalFirewall = oldFirewall
		sessdata.GlobalFirewallDone = oldDone
		sessdata.GlobalFirewallMu.Unlock()
	})
}
