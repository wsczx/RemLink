package dbdata

import (
	"errors"
	"net"
	"time"
)

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

func SetIpMap(v *IpMap) error {
	var err error

	if len(v.IpAddr) < 4 || len(v.MacAddr) < 6 {
		return errors.New("IP或MAC错误")
	}

	macHw, err := net.ParseMAC(v.MacAddr)
	if err != nil {
		return errors.New("MAC错误")
	}
	// 统一macAddr的格式
	v.MacAddr = macHw.String()

	v.UpdatedAt = time.Now()
	if v.Id > 0 {
		err = Set(v)
	} else {
		err = Add(v)
	}
	return err
}
