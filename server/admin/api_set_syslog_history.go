package admin

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

// 日志文件名与日期格式，与 base 包内日志配置保持一致
const (
	syslogLogName    = "remlink.log"
	syslogDateFormat = "2006-01-02"
)

// 历史日志条目，字段与实时推送的 SyslogEntry 保持一致
type SyslogHistoryEntry struct {
	Level string `json:"level"`
	Time  string `json:"time"`
	Msg   string `json:"msg"`
}

// 日志行格式：时间 文件:行 [级别] 内容
var syslogLineRe = regexp.MustCompile(`^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2})\s+([^:\s]+:\d+):\s+\[(\w+)\]\s+(.*)$`)

// 返回历史日志是否可用（后台已配置 log_path）
func SyslogHistoryEnabled(w http.ResponseWriter, r *http.Request) {
	RespSucess(w, map[string]any{
		"enabled": base.GetCfg().LogPath != "",
	})
}

// 列出 log_path 下按天切割的日志日期
func SyslogHistoryDates(w http.ResponseWriter, r *http.Request) {
	cfg := base.GetCfg()
	if cfg.LogPath == "" {
		RespError(w, RespInternalErr, "未配置文件日志路径，无法查看历史日志")
		return
	}
	entries, err := os.ReadDir(cfg.LogPath)
	if err != nil {
		RespError(w, RespInternalErr, "读取日志目录失败: "+err.Error())
		return
	}
	var dates []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == syslogLogName {
			dates = append(dates, time.Now().Format(syslogDateFormat))
			continue
		}
		if after, ok := strings.CutPrefix(name, syslogLogName+"."); ok {
			d := after
			if _, err := time.Parse(syslogDateFormat, d); err == nil {
				dates = append(dates, d)
			}
		}
	}
	// 日期倒序
	for i, j := 0, len(dates)-1; i < j; i, j = i+1, j-1 {
		dates[i], dates[j] = dates[j], dates[i]
	}
	RespSucess(w, map[string]any{"dates": dates})
}

// 按天查询历史日志，支持级别过滤、关键字匹配与分页
func SyslogHistoryList(w http.ResponseWriter, r *http.Request) {
	cfg := base.GetCfg()
	if cfg.LogPath == "" {
		RespError(w, RespInternalErr, "未配置文件日志路径，无法查看历史日志")
		return
	}

	// 日期默认今天
	date := r.FormValue("date")
	if date == "" {
		date = time.Now().Format(syslogDateFormat)
	}
	if _, err := time.Parse(syslogDateFormat, date); err != nil {
		RespError(w, RespParamErr, "日期格式应为 YYYY-MM-DD")
		return
	}

	level := strings.TrimSpace(r.FormValue("level"))
	keyword := strings.TrimSpace(r.FormValue("keyword"))

	page, _ := strconv.Atoi(r.FormValue("page"))
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.FormValue("page_size"))
	if pageSize <= 0 {
		pageSize = 100
	}
	if pageSize > 1000 {
		pageSize = 1000
	}

	candidates := []string{filepath.Join(cfg.LogPath, syslogLogName+"."+date)}
	if date == time.Now().Format(syslogDateFormat) {
		candidates = append(candidates, filepath.Join(cfg.LogPath, syslogLogName))
	}
	var f *os.File
	var openErr error
	for _, p := range candidates {
		f, openErr = os.Open(p)
		if openErr == nil {
			break
		}
	}
	if f == nil {
		if os.IsNotExist(openErr) {
			RespSucess(w, map[string]any{
				"count":     0,
				"page":      page,
				"page_size": pageSize,
				"datas":     []SyslogHistoryEntry{},
			})
			return
		}
		RespError(w, RespInternalErr, "打开日志文件失败: "+openErr.Error())
		return
	}
	defer f.Close()

	// 流式扫描
	start := (page - 1) * pageSize
	end := start + pageSize
	total := 0
	datas := make([]SyslogHistoryEntry, 0, min(pageSize, end-start))

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 4*1024*1024) // 支持长行
	for scanner.Scan() {
		line := scanner.Text()
		m := syslogLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		lvl := m[3]
		if level != "" && !strings.EqualFold(lvl, level) {
			continue
		}
		msg := strings.TrimRight(m[4], "\n")
		if keyword != "" && !strings.Contains(msg, keyword) {
			continue
		}

		timeStr := m[1]
		if len(timeStr) >= 19 {
			timeStr = timeStr[:4] + "-" + timeStr[5:7] + "-" + timeStr[8:]
		}
		if total >= start && total < end {
			datas = append(datas, SyslogHistoryEntry{
				Level: lvl,
				Time:  timeStr,
				Msg:   msg,
			})
		}
		total++
	}
	if err := scanner.Err(); err != nil {
		RespError(w, RespInternalErr, "读取日志文件失败: "+err.Error())
		return
	}

	// 仅在首次打开某日日志时记一条审计，避免瀑布流滚动逐页刷审计
	if page == 1 {
		detail := "级别=" + level + ", 关键字=" + keyword + ", 命中=" + strconv.Itoa(total)
		dbdata.AdminLog("系统设置", "查看历史系统日志:"+date, detail, r.RemoteAddr)
	}

	RespSucess(w, map[string]any{
		"count":     total,
		"page":      page,
		"page_size": pageSize,
		"datas":     datas,
	})
}

// 集群历史日志：默认（无 node 或本机）直接复用单节点 SyslogHistoryList，不另写读取逻辑；
// 选定其它节点时仅更换目标节点地址、访问该节点的同一接口，由对端按本机逻辑返回。
func ClusterSyslogHistory(w http.ResponseWriter, r *http.Request) {
	nodeId := strings.TrimSpace(r.FormValue("node"))
	if nodeId == "" || nodeId == base.GetNodeId() {
		SyslogHistoryList(w, r)
		return
	}

	nodes, err := dbdata.GetClusterNodeRepo().GetNodes()
	if err != nil {
		RespError(w, RespInternalErr, err.Error())
		return
	}
	var target *dbdata.ClusterNode
	for _, n := range nodes {
		if n.Id == nodeId {
			target = n
			break
		}
	}
	if target == nil {
		RespError(w, RespParamErr, "节点不存在或已下线")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	// 透传原始查询参数（含 node=<对端自身 id>），对端收到自身 node 即走本机分支
	_, body, cerr := clusterCli.Call(ctx, http.MethodGet, target.AdminUrl, "/cluster/syslog/history?"+r.URL.RawQuery, nil, "")
	if cerr != nil {
		RespError(w, RespInternalErr, "拉取节点日志失败: "+cerr.Error())
		return
	}
	var hr struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Count    int                  `json:"count"`
			Page     int                  `json:"page"`
			PageSize int                  `json:"page_size"`
			Datas    []SyslogHistoryEntry `json:"datas"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &hr) != nil {
		RespError(w, RespInternalErr, "解析节点日志响应失败")
		return
	}
	if hr.Code != 0 {
		RespError(w, RespInternalErr, hr.Msg)
		return
	}
	RespSucess(w, map[string]any{
		"count":     hr.Data.Count,
		"page":      hr.Data.Page,
		"page_size": hr.Data.PageSize,
		"datas":     hr.Data.Datas,
	})
}
