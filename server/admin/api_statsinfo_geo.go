package admin

import (
	"net/http"
	"sort"

	"github.com/wsczx/remlink/pkg/geoip"
	"github.com/wsczx/remlink/sessdata"
)

// 仪表盘地理分布：当前在线会话按来源 IP 国家计数，返回 {datas:[{code,value}]}
// 查不到国家（私网/未加载数据库/库中无记录）的会话不计入
func StatsGeo(w http.ResponseWriter, r *http.Request) {
	counts := map[string]int{}
	for _, addr := range sessdata.OnlineRemoteAddrs() {
		if code := geoip.Lookup(addr); code != "" {
			counts[code]++
		}
	}
	type item struct {
		Code  string `json:"code"`
		Value int    `json:"value"`
	}
	datas := make([]item, 0, len(counts))
	for c, v := range counts {
		datas = append(datas, item{Code: c, Value: v})
	}
	sort.Slice(datas, func(i, j int) bool {
		if datas[i].Value != datas[j].Value {
			return datas[i].Value > datas[j].Value
		}
		return datas[i].Code < datas[j].Code
	})
	RespSucess(w, map[string]any{
		"datas":         datas,
		"geoip_enabled": geoip.GetStatus().Enabled,
	})
}
