package dbdata

import (
	"errors"
	"net"
	"time"
)

// IP 与 MAC 映射的存储入口
type IpMapStore struct{ *table[IpMap] }

// IP 与 MAC 映射的存储入口实例，由 SetDB 注入持久化实现
var IpMaps = &IpMapStore{}

type IpMap struct {
	Id        int       `json:"id" xorm:"pk autoincr not null"`
	IpAddr    string    `json:"ip_addr" xorm:"varchar(64) not null unique"`
	IpAddr6   string    `json:"ip_addr6" xorm:"varchar(64)"`                                              // IPv6 双栈地址；与 v4 同属一行（同 mac_addr+ip_group），不单独建行以避免 mac_addr 唯一约束冲突
	MacAddr   string    `json:"mac_addr" xorm:"varchar(32) not null unique(MacGroup)"`                    // mac_addr 与 ip_group 组成复合唯一（同 MAC 跨组可有多行绑定）
	Group     string    `json:"group" xorm:"varchar(60) not null default '' 'ip_group' unique(MacGroup)"` // 所属组/出口；同 MAC 跨组允许多行
	UniqueMac bool      `json:"unique_mac" xorm:"Bool index"`
	Username  string    `json:"username" xorm:"varchar(60)"`
	Nickname  string    `json:"nickname" xorm:"-"`
	Keep      bool      `json:"keep" xorm:"Bool"` // 保留 ip-mac 绑定
	KeepTime  time.Time `json:"keep_time" xorm:"DateTime"`
	Note      string    `json:"note" xorm:"varchar(255)"` // 备注
	LastLogin time.Time `json:"last_login" xorm:"DateTime"`
	UpdatedAt time.Time `json:"updated_at" xorm:"DateTime updated"`
}

func (st *IpMapStore) SetIpMap(v *IpMap) error {
	var err error

	if len(v.IpAddr) < 4 || len(v.MacAddr) < 6 {
		return errors.New("IP或MAC错误")
	}

	if ip := net.ParseIP(v.IpAddr); ip == nil || ip.To4() == nil {
		return errors.New("IP错误：仅支持 IPv4 地址")
	}
	if v.IpAddr6 != "" {
		if ip6 := net.ParseIP(v.IpAddr6); ip6 == nil || ip6.To4() != nil {
			return errors.New("IPv6 地址错误")
		}
	}

	macHw, err := net.ParseMAC(v.MacAddr)
	if err != nil {
		return errors.New("MAC错误")
	}
	// 统一macAddr的格式
	v.MacAddr = macHw.String()

	v.UpdatedAt = time.Now()
	if v.Id > 0 {
		err = GetDB().Save(v)
	} else {
		err = GetDB().Insert(v)
	}
	return err
}

// 按 ID 加载 IP 映射
func (st *IpMapStore) GetByID(id int, dest *IpMap) error {
	return GetDB().Get("Id", id, dest)
}

// 按 IPv4 地址加载 IP 映射
func (st *IpMapStore) GetByIP(ip string, dest *IpMap) error {
	return GetDB().Get("ip_addr", ip, dest)
}

// 按 IPv6 地址加载 IP 映射
func (st *IpMapStore) GetByIP6(ip string, dest *IpMap) error {
	return GetDB().Get("ip_addr6", ip, dest)
}

// 按 ID 删除 IP 映射
func (st *IpMapStore) Delete(id int) error {
	return GetDB().Delete(&IpMap{Id: id})
}

// 返回 IP 映射总行数
func (st *IpMapStore) Count() int {
	return GetDB().Count(&IpMap{})
}
