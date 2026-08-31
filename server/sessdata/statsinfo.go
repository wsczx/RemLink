package sessdata

import (
	"encoding/json"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
	"github.com/wsczx/remlink/dbdata"
)

const (
	StatsCycleSec = 10 // 统计周期（秒）
	AddCycleSec   = 60 // 记录到数据表周期（秒）
)

func saveStatsInfo() {
	go func() {
		tick := time.NewTicker(time.Second * StatsCycleSec)
		count := 0
		for range tick.C {
			up := uint64(0)
			down := uint64(0)
			upGroups := make(map[int]uint64)
			downGroups := make(map[int]uint64)
			numGroups := make(map[int]int)
			onlineNum := 0
			sessMux.Lock()
			for _, v := range sessions {
				v.mux.Lock()
				if v.IsActive && v.CSess.Group != nil {
					// 在线人数
					onlineNum += 1
					numGroups[v.CSess.Group.Id] += 1
					// 网络吞吐
					userUp := v.CSess.BandwidthUpPeriod.Load()
					userDown := v.CSess.BandwidthDownPeriod.Load()
					if userUp > 0 {
						upGroups[v.CSess.Group.Id] += userUp
					}
					if userDown > 0 {
						downGroups[v.CSess.Group.Id] += userDown
					}
					up += userUp
					down += userDown
				}
				v.mux.Unlock()
			}
			sessMux.Unlock()

			tNow := time.Now()
			// online
			numData, err := json.Marshal(numGroups)
			if err != nil {
				numData = []byte("{}")
			}
			so := dbdata.StatsOnline{Num: onlineNum, NumGroups: string(numData), CreatedAt: tNow}
			// network
			upData, err := json.Marshal(upGroups)
			if err != nil {
				upData = []byte("{}")
			}
			downData, err := json.Marshal(downGroups)
			if err != nil {
				downData = []byte("{}")
			}
			sn := dbdata.StatsNetwork{Up: up, Down: down, UpGroups: string(upData), DownGroups: string(downData), CreatedAt: tNow}
			// cpu
			sc := dbdata.StatsCpu{Percent: getCpuPercent(), CreatedAt: tNow}
			// mem
			sm := dbdata.StatsMem{Percent: getMemPercent(), CreatedAt: tNow}
			count++
			// 是否保存至数据库
			save := count*StatsCycleSec >= AddCycleSec
			// 历史数据
			if save {
				count = 0
			}
			// 设置统计数据
			setStatsData(save, so, sn, sc, sm)
		}
	}()
}

func setStatsData(save bool, so dbdata.StatsOnline, sn dbdata.StatsNetwork, sc dbdata.StatsCpu, sm dbdata.StatsMem) {
	// 实时数据
	dbdata.StatsInfoIns.SetRealTime("online", so)
	dbdata.StatsInfoIns.SetRealTime("network", sn)
	dbdata.StatsInfoIns.SetRealTime("cpu", sc)
	dbdata.StatsInfoIns.SetRealTime("mem", sm)
	if !save {
		return
	}
	dbdata.StatsInfoIns.SaveStatsInfo(so, sn, sc, sm)
}

func getCpuPercent() float64 {
	cpuUsedPercent, err := cpu.Percent(0, false)
	if err != nil || len(cpuUsedPercent) == 0 {
		return 0
	}
	percent := cpuUsedPercent[0]
	if percent == 0 {
		percent = 1
	}
	return decimal(percent)
}

func getMemPercent() float64 {
	m, err := mem.VirtualMemory()
	if err != nil {
		return 0
	}
	return decimal(m.UsedPercent)
}

func decimal(f float64) float64 {
	i := int(f * 100)
	return float64(i) / 100
}
