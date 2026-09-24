package admin

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/sessdata"
)

// 节点间调用客户端：封装专用 JWT 鉴权 + 超时；节点间为内网管控通道，TLS 固定跳过校验
type ClusterClient struct {
	http *http.Client
}

var clusterCli = &ClusterClient{
	http: &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	},
}

// 获取节点间调用客户端单例
func GetClusterClient() *ClusterClient { return clusterCli }

// 向目标节点后台发起管控请求，自动携带节点间 JWT；超时由 ctx 控制
func (c *ClusterClient) Call(ctx context.Context, method, adminUrl, path string, body io.Reader, contentType string) (int, []byte, error) {
	token, err := SetClusterJwtData()
	if err != nil {
		return 0, nil, err
	}
	u := strings.TrimRight(adminUrl, "/") + path
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Jwt", token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, b, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return resp.StatusCode, b, nil
}

// 用节点间专用令牌向目标节点后台发起重启
func (c *ClusterClient) Restart(adminUrl, remoteAddr string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	if _, _, err := c.Call(ctx, http.MethodPost, adminUrl, "/set/restart", nil, ""); err != nil {
		return err
	}
	dbdata.AdminLog("节点", "远程重启", "触发节点重启: "+adminUrl, remoteAddr)
	return nil
}

// 用节点间专用令牌向目标节点触发本地在线升级（非阻塞，立即返回；各节点按自身架构下载对应安装包）
func (c *ClusterClient) Upgrade(adminUrl, remoteAddr string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, _, err := c.Call(ctx, http.MethodPost, adminUrl, "/set/upgrade/trigger", nil, ""); err != nil {
		return err
	}
	dbdata.AdminLog("节点", "远程升级", "触发节点升级: "+adminUrl, remoteAddr)
	return nil
}

// 用节点间专用令牌向目标节点拉取 /cluster/status；ctx 控制超时
func (c *ClusterClient) Status(ctx context.Context, adminUrl string) (map[string]any, error) {
	_, b, err := c.Call(ctx, http.MethodGet, adminUrl, "/cluster/status", nil, "")
	if err != nil {
		return nil, err
	}
	var env struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, err
	}
	if env.Code != 0 {
		return nil, fmt.Errorf("节点返回错误码 %d", env.Code)
	}
	return env.Data, nil
}

// 经目标节点 /cluster/session/kick 踢下线（仅 /cluster/* 接受节点间令牌，命中本机分支即直踢）
func (c *ClusterClient) Kick(adminUrl, nodeId, token, remoteAddr string) error {
	body, _ := json.Marshal(map[string]any{"node": nodeId, "token": token})
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	if _, _, err := c.Call(ctx, http.MethodPost, adminUrl, "/cluster/session/kick", bytes.NewReader(body), "application/json"); err != nil {
		return err
	}
	dbdata.AdminLog("节点", "踢下线", "节点踢下线: "+adminUrl, remoteAddr)
	return nil
}

// 经目标节点 /cluster/unlock 解锁账号/IP（仅 /cluster/* 接受节点间令牌，命中本机分支即直解）
func (c *ClusterClient) Unlock(adminUrl string, body []byte, remoteAddr string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	if _, _, err := c.Call(ctx, http.MethodPost, adminUrl, "/cluster/unlock", bytes.NewReader(body), "application/json"); err != nil {
		return err
	}
	dbdata.AdminLog("节点", "解锁", "节点解锁: "+adminUrl, remoteAddr)
	return nil
}

// 把单个配置字段变更同步到对端
func (c *ClusterClient) SetConfigField(adminUrl, name string, data any) error {
	body, _ := json.Marshal(map[string]any{"name": name, "data": data})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_, b, err := c.Call(ctx, http.MethodPost, adminUrl, "/cluster/set/config", bytes.NewReader(body), "application/json")
	if err != nil {
		return err
	}
	var env map[string]any
	if err := json.Unmarshal(b, &env); err != nil {
		return err
	}
	if code, ok := env["code"].(float64); !ok || code != 0 {
		msg, _ := env["msg"].(string)
		return fmt.Errorf("对端返回错误: %s", msg)
	}
	return nil
}

// 节点清单：含在线状态与本机标记
func ClusterNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := dbdata.GetClusterNodeRepo().GetNodes()
	if err != nil {
		RespError(w, RespInternalErr, err.Error())
		return
	}
	now := time.Now().Unix()
	selfId := base.GetNodeId()
	out := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		row := map[string]any{
			"id":            n.Id,
			"name":          n.Name,
			"admin_url":     n.AdminUrl,
			"version":       n.Version,
			"last_seen":     n.LastSeen,
			"online":        now-n.LastSeen <= 90,
			"is_self":       n.Id == selfId,
			"needs_restart": n.NeedsRestart,
		}
		if n.Id == selfId {
			// 编辑后立即生效
			row["admin_url"] = base.GetNodeAdminUrl()
			row["name"] = base.GetNodeName()
		}
		out = append(out, row)
	}
	RespSucess(w, out)
}

// 重启指定节点（本机走本地重启，其它节点走共享 JwtSecret 节点间调用）
func ClusterRestart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Node string `json:"node"`
	}
	if b, err := io.ReadAll(r.Body); err == nil {
		_ = json.Unmarshal(b, &req)
	}
	if req.Node == "" {
		RespError(w, RespParamErr, "node 必填")
		return
	}
	if req.Node == base.GetNodeId() {
		if !selfRestart(r.RemoteAddr) {
			RespError(w, RespInternalErr, "重启进行中，请稍后再试")
			return
		}
		RespSucess(w, map[string]any{"message": "restart scheduled"})
		return
	}
	target, err := dbdata.GetClusterNodeRepo().Get(req.Node)
	if err != nil {
		RespError(w, RespInternalErr, err.Error())
		return
	}
	if target == nil {
		RespError(w, RespParamErr, "节点不存在")
		return
	}
	if err := clusterCli.Restart(target.AdminUrl, r.RemoteAddr); err != nil {
		RespError(w, RespInternalErr, "远程重启失败: "+err.Error())
		return
	}
	RespSucess(w, map[string]any{"message": "restart scheduled"})
}

// 重启全部节点（先其它节点，最后本机）；收集离线跳过与失败清单返回，便于前端提示
func ClusterRestartAll(w http.ResponseWriter, r *http.Request) {
	nodes, err := dbdata.GetClusterNodeRepo().GetNodes()
	if err != nil {
		RespError(w, RespInternalErr, err.Error())
		return
	}
	selfId := base.GetNodeId()
	now := time.Now().Unix()
	var failed []string
	skipped := 0
	for _, n := range nodes {
		if n.Id == selfId {
			continue
		}
		if now-n.LastSeen > 90 {
			skipped++
			continue
		}
		if err := clusterCli.Restart(n.AdminUrl, r.RemoteAddr); err != nil {
			failed = append(failed, n.Name+": "+err.Error())
		}
	}
	RespSucess(w, map[string]any{
		"message": "restart scheduled for all",
		"failed":  failed,
		"skipped": skipped,
	})
	selfRestart(r.RemoteAddr)
}

// 触发指定节点在线升级：本机直跑，其它节点经节点间调用触发其本地升级（各节点按自身架构下载对应安装包）
func ClusterUpgrade(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Node string `json:"node"`
	}
	if b, err := io.ReadAll(r.Body); err == nil {
		_ = json.Unmarshal(b, &req)
	}
	if req.Node == "" {
		RespError(w, RespParamErr, "node 必填")
		return
	}
	if req.Node == base.GetNodeId() {
		version, err := beginUpgrade()
		if err != nil {
			RespError(w, RespInternalErr, err.Error())
			return
		}
		dbdata.AdminLog("节点", "在线升级", fmt.Sprintf("本机从 %s 升级到 %s", base.APP_VER, version), r.RemoteAddr)
		RespSucess(w, map[string]any{"message": "upgrade scheduled", "version": version})
		return
	}
	target, err := dbdata.GetClusterNodeRepo().Get(req.Node)
	if err != nil {
		RespError(w, RespInternalErr, err.Error())
		return
	}
	if target == nil {
		RespError(w, RespParamErr, "节点不存在")
		return
	}
	if err := clusterCli.Upgrade(target.AdminUrl, r.RemoteAddr); err != nil {
		RespError(w, RespInternalErr, "远程升级触发失败: "+err.Error())
		return
	}
	RespSucess(w, map[string]any{"message": "upgrade scheduled"})
}

// 升级全部节点（先其它节点，最后本机）；收集离线跳过与失败清单返回
func ClusterUpgradeAll(w http.ResponseWriter, r *http.Request) {
	nodes, err := dbdata.GetClusterNodeRepo().GetNodes()
	if err != nil {
		RespError(w, RespInternalErr, err.Error())
		return
	}
	selfId := base.GetNodeId()
	now := time.Now().Unix()
	var failed []string
	skipped := 0
	for _, n := range nodes {
		if n.Id == selfId {
			continue
		}
		if now-n.LastSeen > 90 {
			skipped++
			continue
		}
		if err := clusterCli.Upgrade(n.AdminUrl, r.RemoteAddr); err != nil {
			failed = append(failed, n.Name+": "+err.Error())
		}
	}
	if v, err := beginUpgrade(); err != nil {
		failed = append(failed, "本机: "+err.Error())
	} else {
		dbdata.AdminLog("节点", "在线升级", fmt.Sprintf("本机从 %s 升级到 %s", base.APP_VER, v), r.RemoteAddr)
	}
	RespSucess(w, map[string]any{
		"message": "upgrade scheduled for all",
		"failed":  failed,
		"skipped": skipped,
	})
}

// 本节点对外可达地址：统一由 base.GetNodeAdminUrl() 按 LINK>本地手动>默认 解析，心跳仅镜像该值
func resolveAdminUrl() string {
	return base.GetNodeAdminUrl()
}

// 汇总本节点运行时状态（在线会话/锁/并发/健康）
func getSelfStatus() map[string]any {
	sessions := sessdata.GetOnlineSess("", "", true)
	cur, maxC, maxU, perUser := sessdata.GetConcurrency()
	up, down := latestNetwork()
	return map[string]any{
		"node_id":        base.GetNodeId(),
		"name":           base.GetNodeName(),
		"admin_url":      resolveAdminUrl(),
		"version":        base.APP_VER,
		"boot_time":      base.GetNodeBootTime(),
		"needs_restart":  base.GetNeedsRestart(),
		"uptime":         int64(time.Since(serverStartTime).Seconds()),
		"sessions":       sessions,
		"sessions_count": len(sessions),
		"locks":          auth.GetLockManager().LockInfo(),
		"concurrency": map[string]any{
			"current":  cur,
			"max":      maxC,
			"max_user": maxU,
			"per_user": perUser,
		},
		"health": map[string]any{
			"cpu":  sessdata.GetCpuPercent(),
			"mem":  sessdata.GetMemPercent(),
			"up":   up,
			"down": down,
		},
	}
}

// 返回本节点状态
func ClusterNodeStatus(w http.ResponseWriter, r *http.Request) {
	RespSucess(w, getSelfStatus())
}

// 聚合所有节点状态：本机本地汇总
// 整体 15s 超时，单节点受 ctx 约束，避免个别节点不可达时长阻塞总览接口
func ClusterOverview(w http.ResponseWriter, r *http.Request) {
	nodes, err := dbdata.GetClusterNodeRepo().GetNodes()
	if err != nil {
		RespError(w, RespInternalErr, err.Error())
		return
	}
	selfId := base.GetNodeId()
	now := time.Now().Unix()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	out := make([]map[string]any, len(nodes))
	var wg sync.WaitGroup
	for i, n := range nodes {
		online := now-n.LastSeen <= 90
		if n.Id == selfId {
			st := getSelfStatus()
			st["online"] = true
			st["reachable"] = true
			st["is_self"] = true
			out[i] = st
			continue
		}
		wg.Add(1)
		go func(idx int, node *dbdata.ClusterNode, isOnline bool) {
			defer wg.Done()
			st, ferr := clusterCli.Status(ctx, node.AdminUrl)
			if ferr != nil {
				out[idx] = map[string]any{
					"node_id":   node.Id,
					"name":      node.Name,
					"admin_url": node.AdminUrl,
					"version":   node.Version,
					"online":    isOnline,
					"reachable": false,
					"error":     ferr.Error(),
				}
				return
			}
			st["online"] = isOnline
			st["reachable"] = true
			out[idx] = st
		}(i, n, online)
	}
	wg.Wait()
	RespSucess(w, out)
}

// 取最近一次实时网络吞吐
func latestNetwork() (up, down uint64) {
	if dbdata.StatsInfoIns == nil {
		return 0, 0
	}
	rs := dbdata.StatsInfoIns.GetRealTime("network")
	if len(rs) == 0 {
		return 0, 0
	}
	if sn, ok := rs[len(rs)-1].(dbdata.StatsNetwork); ok {
		return sn.Up, sn.Down
	}
	return 0, 0
}

// 踢下线指定节点的某个会话
func ClusterSessionKick(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Node  string `json:"node"`
		Token string `json:"token"`
	}
	if b, err := io.ReadAll(r.Body); err == nil {
		_ = json.Unmarshal(b, &req)
	}
	if req.Node == "" || req.Token == "" {
		RespError(w, RespParamErr, "node 与 token 必填")
		return
	}
	if req.Node == base.GetNodeId() {
		dbdata.AdminLog("节点", "踢下线", "本机踢下线: "+req.Token, r.RemoteAddr)
		sessdata.CloseSess(req.Token, dbdata.UserLogoutAdmin)
		RespSucess(w, map[string]any{"message": "kicked"})
		return
	}
	target, err := dbdata.GetClusterNodeRepo().Get(req.Node)
	if err != nil {
		RespError(w, RespInternalErr, err.Error())
		return
	}
	if target == nil {
		RespError(w, RespParamErr, "节点不存在")
		return
	}
	if err := clusterCli.Kick(target.AdminUrl, target.Id, req.Token, r.RemoteAddr); err != nil {
		RespError(w, RespInternalErr, "远程踢下线失败: "+err.Error())
		return
	}
	RespSucess(w, map[string]any{"message": "kicked"})
}

// 解锁指定节点的账号/IP 锁定
func ClusterUnlock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Node        string          `json:"node"`
		Username    string          `json:"username"`
		IP          string          `json:"ip"`
		Description string          `json:"description"`
		State       *auth.LockState `json:"state"`
	}
	if b, err := io.ReadAll(r.Body); err == nil {
		_ = json.Unmarshal(b, &req)
	}
	if req.Node == "" {
		RespError(w, RespParamErr, "node 必填")
		return
	}
	if req.Username == "" && req.IP == "" {
		RespError(w, RespParamErr, "username 与 ip 至少填一个")
		return
	}
	if req.Node == base.GetNodeId() {
		lm := auth.GetLockManager()
		switch {
		case req.Username != "" && req.IP != "":
			lm.UnlockUserIP(req.Username, req.IP)
		case req.Username != "":
			lm.UnlockUser(req.Username)
		case req.IP != "":
			lm.UnlockIP(req.IP)
		}
		dbdata.AdminLog("节点", "解锁", "本机解锁: "+req.Description, r.RemoteAddr)
		RespSucess(w, map[string]any{"message": "unlocked"})
		return
	}
	target, err := dbdata.GetClusterNodeRepo().Get(req.Node)
	if err != nil {
		RespError(w, RespInternalErr, err.Error())
		return
	}
	if target == nil {
		RespError(w, RespParamErr, "节点不存在")
		return
	}
	body, err := json.Marshal(req)
	if err != nil {
		RespError(w, RespInternalErr, err.Error())
		return
	}
	if err := clusterCli.Unlock(target.AdminUrl, body, r.RemoteAddr); err != nil {
		RespError(w, RespInternalErr, "远程解锁失败: "+err.Error())
		return
	}
	RespSucess(w, map[string]any{"message": "unlocked"})
}

// 聚合全集群在线会话：本机始终以本地 getSelfStatus 兜底（单节点/Sqlite 的 cluster_node 表为空也能正常展示）
// 其余节点按 cluster_node 表逐个探活；不可达节点返回 reachable=false 与 error
func ClusterSessions(w http.ResponseWriter, r *http.Request) {
	selfId := base.GetNodeId()
	selfSt := getSelfStatus()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	out := make([]map[string]any, 0, 4)
	out = append(out, map[string]any{
		"node_id":        selfId,
		"node_name":      selfSt["name"],
		"is_self":        true,
		"reachable":      true,
		"error":          "",
		"sessions":       selfSt["sessions"],
		"sessions_count": selfSt["sessions_count"],
	})

	nodes, err := dbdata.GetClusterNodeRepo().GetNodes()
	if err != nil {
		RespError(w, RespInternalErr, err.Error())
		return
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, n := range nodes {
		if n.Id == selfId {
			continue
		}
		n := n
		wg.Go(func() {
			item := map[string]any{
				"node_id":   n.Id,
				"node_name": n.Name,
				"is_self":   false,
				"reachable": false,
				"error":     "",
				"sessions":  []any{},
			}
			st, err := clusterCli.Status(ctx, n.AdminUrl)
			if err != nil {
				item["error"] = err.Error()
			} else {
				item["reachable"] = true
				item["sessions"] = st["sessions"]
				item["sessions_count"] = st["sessions_count"]
			}
			mu.Lock()
			out = append(out, item)
			mu.Unlock()
		})
	}
	wg.Wait()
	RespSucess(w, out)
}

// 聚合全集群账号/IP 锁定：本机以本地 getSelfStatus 兜底，其余节点逐个探活
func ClusterLocks(w http.ResponseWriter, r *http.Request) {
	selfId := base.GetNodeId()
	selfSt := getSelfStatus()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	out := make([]map[string]any, 0, 4)
	out = append(out, map[string]any{
		"node_id":   selfId,
		"node_name": selfSt["name"],
		"is_self":   true,
		"reachable": true,
		"error":     "",
		"locks":     selfSt["locks"],
	})

	nodes, err := dbdata.GetClusterNodeRepo().GetNodes()
	if err != nil {
		RespError(w, RespInternalErr, err.Error())
		return
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, n := range nodes {
		if n.Id == selfId {
			continue
		}
		n := n
		wg.Go(func() {
			item := map[string]any{
				"node_id":   n.Id,
				"node_name": n.Name,
				"is_self":   false,
				"reachable": false,
				"error":     "",
				"locks":     []any{},
			}
			st, err := clusterCli.Status(ctx, n.AdminUrl)
			if err != nil {
				item["error"] = err.Error()
			} else {
				item["reachable"] = true
				item["locks"] = st["locks"]
			}
			mu.Lock()
			out = append(out, item)
			mu.Unlock()
		})
	}
	wg.Wait()
	RespSucess(w, out)
}

// 断连指定节点的某个会话（集群内按 node 路由，命中本机分支即直断）
func ClusterReline(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Node  string `json:"node"`
		Token string `json:"token"`
	}
	if b, err := io.ReadAll(r.Body); err == nil {
		_ = json.Unmarshal(b, &req)
	}
	if req.Node == "" || req.Token == "" {
		RespError(w, RespParamErr, "node 与 token 必填")
		return
	}
	if req.Node == base.GetNodeId() {
		dbdata.AdminLog("节点", "断连", "本机断连: "+req.Token, r.RemoteAddr)
		sessdata.CloseCSess(req.Token)
		RespSucess(w, map[string]any{"message": "reline"})
		return
	}
	target, err := dbdata.GetClusterNodeRepo().Get(req.Node)
	if err != nil {
		RespError(w, RespInternalErr, err.Error())
		return
	}
	if target == nil {
		RespError(w, RespParamErr, "节点不存在")
		return
	}
	if err := clusterCli.Reline(target.AdminUrl, req.Node, req.Token, r.RemoteAddr); err != nil {
		RespError(w, RespInternalErr, "远程断连失败: "+err.Error())
		return
	}
	RespSucess(w, map[string]any{"message": "reline"})
}

// 经目标节点 /cluster/session/reline 断连指定会话（命中本机分支即直断）
func (c *ClusterClient) Reline(adminUrl, nodeId, token, remoteAddr string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	body, _ := json.Marshal(map[string]any{"node": nodeId, "token": token})
	if _, _, err := c.Call(ctx, http.MethodPost, adminUrl, "/cluster/session/reline", bytes.NewReader(body), "application/json"); err != nil {
		return err
	}
	dbdata.AdminLog("节点", "远程断连", "触发会话断连: "+adminUrl, remoteAddr)
	return nil
}

// 更新本节点管理地址与名称（仅本机生效；管理地址持久化到本地文件，优先级低于 LINK_CLUSTER_URL）
func ClusterNodeUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Id       string `json:"id"`
		AdminUrl string `json:"admin_url"`
		Name     string `json:"name"`
	}
	if b, err := io.ReadAll(r.Body); err == nil {
		_ = json.Unmarshal(b, &req)
	}
	if req.Id == "" {
		RespError(w, RespParamErr, "id 必填")
		return
	}
	// 管理地址与名称仅本机可改，写本地文件；远程节点身份由其自身决定
	if req.Id != base.GetNodeId() {
		RespError(w, RespParamErr, "仅可修改本节点")
		return
	}
	if req.AdminUrl != "" {
		base.SetNodeAdminUrl(req.AdminUrl)
	}
	if req.Name != "" {
		base.SetNodeName(req.Name)
	}
	// 立即同步
	if err := dbdata.GetClusterNodeRepo().SaveSelf(); err != nil {
		RespError(w, RespInternalErr, "同步节点信息失败: "+err.Error())
		return
	}
	RespSucess(w, map[string]any{"message": "updated"})
}

// 集群配置设置
func ClusterSetConfig(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if b, err := io.ReadAll(r.Body); err == nil {
		_ = json.Unmarshal(b, &req)
	}
	name, _ := req["name"].(string)
	if name == "" {
		RespError(w, RespParamErr, "name 必填")
		return
	}
	data := req["data"]
	restart, err := base.SetConfigField(name, data)
	if err != nil {
		RespError(w, RespInternalErr, "应用配置失败: "+err.Error())
		return
	}
	switch name {
	case "ip_whitelist":
		auth.GetLockManager().LoadIPList(auth.IPWhiteList, base.GetCfg().IPWhiteList)
	case "ip_blacklist":
		auth.GetLockManager().LoadIPList(auth.IPBlackList, base.GetCfg().IPBlackList)
	case "show_sql":
		dbdata.GetXdb().ShowSQL(base.GetCfg().ShowSQL)
	}
	if err := dbdata.SettingSaveServerConfig(); err != nil {
		RespError(w, RespInternalErr, "保存配置失败: "+err.Error())
		return
	}
	dbdata.AdminLog("节点", "配置同步", "接收并应用集群配置: "+name, r.RemoteAddr)
	RespSucess(w, map[string]any{"restart": restart})
}

// 把单个配置字段变更同步推送到集群内其它在线节点
func syncConfigToPeers(name string, data any) []string {
	nodes, err := dbdata.GetClusterNodeRepo().GetNodes()
	if err != nil {
		base.Warn("集群配置同步：获取节点清单失败:", err)
		return nil
	}
	selfId := base.GetNodeId()
	now := time.Now().Unix()
	var (
		mu     sync.Mutex
		failed = []string{}
		wg     sync.WaitGroup
	)
	for _, n := range selectPeerNodes(nodes, selfId, now) {
		wg.Go(func() {
			if err := clusterCli.SetConfigField(n.AdminUrl, name, data); err != nil {
				base.Warn(fmt.Sprintf("集群配置同步失败 节点=%s 字段=%s: %v", n.Name, name, err))
				mu.Lock()
				failed = append(failed, n.Name)
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	return failed
}

// 筛出需要同步的对端：排除自身、排除超过心跳 TTL(90s)的离线节点
func selectPeerNodes(nodes []*dbdata.ClusterNode, selfID string, now int64) []*dbdata.ClusterNode {
	var peers []*dbdata.ClusterNode
	for _, n := range nodes {
		if n.Id == selfID || now-n.LastSeen > 90 {
			continue
		}
		peers = append(peers, n)
	}
	return peers
}
