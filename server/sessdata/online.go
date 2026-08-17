package sessdata

import (
	"bytes"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
)

type Online struct {
	Token             string    `json:"token"`
	IsActive          bool      `json:"is_active"`
	Username          string    `json:"username"`
	Nickname          string    `json:"nickname"`
	Group             string    `json:"group"`
	MacAddr           string    `json:"mac_addr"`
	UniqueMac         bool      `json:"unique_mac"`
	Ip                net.IP    `json:"ip"`
	RemoteAddr        string    `json:"remote_addr"`
	TransportProtocol string    `json:"transport_protocol"`
	TunName           string    `json:"tun_name"`
	Mtu               int       `json:"mtu"`
	Client            string    `json:"client"`
	DeviceType        string    `json:"device_type"`
	PlatformVersion   string    `json:"platform_version"`
	BandwidthUp       string    `json:"bandwidth_up"`
	BandwidthDown     string    `json:"bandwidth_down"`
	BandwidthUpAll    string    `json:"bandwidth_up_all"`
	BandwidthDownAll  string    `json:"bandwidth_down_all"`
	TrafficQuota      string    `json:"traffic_quota"`
	TrafficUsed       string    `json:"traffic_used"`
	TrafficReset      string    `json:"traffic_reset"`
	LastLogin         time.Time `json:"last_login"`
}

type Onlines []Online

func (o Onlines) Len() int {
	return len(o)
}

func (o Onlines) Less(i, j int) bool {
	return bytes.Compare(o[i].Ip, o[j].Ip) < 0
}

func (o Onlines) Swap(i, j int) {
	o[i], o[j] = o[j], o[i]
}

// 返回在线会话列表，支持按用户名/组/MAC/IP/远端地址模糊搜索
func GetOnlineSess(search_cate string, search_text string, show_sleeper bool) []Online {
	var datas Onlines
	if strings.TrimSpace(search_text) == "" {
		search_cate = ""
	}
	sessMux.Lock()
	defer sessMux.Unlock()

	for _, v := range sessions {
		func() {
			v.mux.Lock()
			defer v.mux.Unlock()

			cSess := v.CSess
			if cSess == nil {
				cSess = &ConnSession{}
			}
			// 选择需要比较的字符串
			var compareText string
			switch search_cate {
			case "username":
				compareText = v.Username
			case "group":
				compareText = v.Group
			case "mac_addr":
				compareText = v.MacAddr
			case "ip":
				if cSess != nil {
					compareText = cSess.IpAddr.String()
				}
			case "remote_addr":
				if cSess != nil {
					compareText = cSess.RemoteAddr
				}
			}
			if search_cate != "" && !strings.Contains(compareText, search_text) {
				return
			}

			// 真实在线：活跃且仍有活动连接；show_sleeper 额外展示已掉线但仍在等待清理/重连的休眠态
			if (show_sleeper && !v.IsActive) || (v.IsActive && cSess != nil && cSess.IpAddr != nil) {
				transportProtocol := "TCP"
				dSess := cSess.GetDtlsSession()
				if dSess != nil {
					transportProtocol = "UDP"
				}
				quotaStr, usedStr, resetStr := "", "", ""
				u := &dbdata.User{}
				dbdata.One("Username", v.Username, u)
				if cSess.Policy != nil && cSess.Policy.TrafficQuota > 0 {
					quotaStr = utils.HumanByte(uint64(cSess.Policy.TrafficQuota))
					usedStr = utils.HumanByte(uint64(u.TrafficUsed))
					resetStr = cSess.Policy.TrafficReset
				}
				val := Online{
					Token:             v.Token,
					IsActive:          v.IsActive,
					Ip:                cSess.IpAddr,
					Username:          v.Username,
					Nickname:          u.Nickname,
					Group:             v.Group,
					MacAddr:           v.MacAddr,
					UniqueMac:         v.UniqueMac,
					RemoteAddr:        cSess.RemoteAddr,
					TransportProtocol: transportProtocol,
					TunName:           cSess.IfName,
					Mtu:               cSess.Mtu,
					Client:            cSess.Client,
					DeviceType:        v.DeviceType,
					PlatformVersion:   v.PlatformVersion,
					BandwidthUp:       utils.HumanByte(cSess.BandwidthUpPeriod.Load()) + "/s",
					BandwidthDown:     utils.HumanByte(cSess.BandwidthDownPeriod.Load()) + "/s",
					BandwidthUpAll:    utils.HumanByte(cSess.BandwidthUpAll.Load()),
					BandwidthDownAll:  utils.HumanByte(cSess.BandwidthDownAll.Load()),
					TrafficQuota:      quotaStr,
					TrafficUsed:       usedStr,
					TrafficReset:      resetStr,
					LastLogin:         v.LastLogin,
				}
				datas = append(datas, val)
			}
		}()
	}
	sort.Sort(&datas)
	return datas
}
