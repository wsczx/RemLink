package service

import (
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

// 保存组并联动：证书缓存失效、旧 NAT 清理、外部用户同步
// 旧值读取失败按"视为新建"处理
func SaveGroup(v *dbdata.Group) error {
	oldName := v.Name
	var old dbdata.Group
	if v.Id > 0 {
		if err := dbdata.Groups.Get(v.Id, &old); err == nil && old.Id > 0 {
			oldName = old.Name
		}
	}
	if err := dbdata.Groups.SetGroup(v); err != nil {
		return err
	}

	// 组配置变化后成员 Token 中固化的组信息可能过期，统一在 service 触发撤回
	actions.RevokeGroupWebVpn([]string{oldName})
	actions.InvalidateCertCache()
	if (old.ClientCidr != "" || old.ClientCidr6 != "") &&
		(old.ClientCidr != v.ClientCidr || old.ClientCidr6 != v.ClientCidr6 || old.OutDev != v.OutDev) {
		actions.RemoveGroupNAT(old.ClientCidr, old.ClientCidr6, old.OutDev)
	}
	actions.SyncExternalUsers(v)
	return nil
}

// 删除组后的联动清理；g 为删除前查出的组
func DeleteGroup(g *dbdata.Group) {
	actions.InvalidateCertCache()
	actions.RemoveGroupNAT(g.ClientCidr, g.ClientCidr6, g.OutDev)
	actions.RevokeGroupWebVpn([]string{g.Name})
	if err := dbdata.Users.RemoveGroupFromUsers(g.Name); err != nil {
		base.Error(err)
	}
}
