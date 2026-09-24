package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

var (
	upgradeMux   sync.Mutex
	upgradeState *UpgradeState
)

type UpgradeState struct {
	Running  bool              `json:"running"`
	Stage    string            `json:"stage"`
	Progress int               `json:"progress"`
	Error    string            `json:"error,omitempty"`
	Info     *base.ReleaseInfo `json:"info,omitempty"`
}

// 读取用户配置的更新源（github / gitee）
func getUpgradeSource() string {
	sc := &dbdata.SettingServerConfig{}
	if err := dbdata.SettingGet(sc); err != nil {
		return "github"
	}
	return sc.Config.UpgradeSource
}

// 检查是否有新版本
func CheckUpgrade(w http.ResponseWriter, r *http.Request) {
	info, needUpgrade, err := base.CheckUpdate(getUpgradeSource())
	if err != nil {
		RespError(w, RespInternalErr, "检查更新失败: ", err)
		return
	}

	data := map[string]any{
		"current_version": "v" + base.APP_VER,
		"need_upgrade":    needUpgrade,
		"upgrade_source":  info.UpgradeSource,
		"latest":          info,
	}
	RespSucess(w, data)
}

// 后台执行升级：下载/替换/重启，并同步进度到全局 upgradeState（非阻塞）
func runUpgrade(info *base.ReleaseInfo) {
	progressCh := make(chan base.UpgradeProgress, 10)
	go func() {
		defer close(progressCh)
		base.DoUpgrade(info, progressCh)
	}()
	go func() {
		for p := range progressCh {
			upgradeMux.Lock()
			if upgradeState != nil {
				upgradeState.Stage = p.Stage
				upgradeState.Progress = p.Progress
				upgradeState.Error = p.Error
			}
			upgradeMux.Unlock()
		}
		upgradeMux.Lock()
		if upgradeState != nil {
			upgradeState.Running = false
			if upgradeState.Stage != "done" && upgradeState.Stage != "error" {
				upgradeState.Stage = "error"
				upgradeState.Error = "升级异常中断"
			}
		}
		upgradeMux.Unlock()
	}()
}

// 触发本机在线升级（非阻塞）：检查更新后立即后台执行，进度见 UpgradeStatusHandler
func beginUpgrade() (string, error) {
	upgradeMux.Lock()
	if upgradeState != nil && upgradeState.Running {
		upgradeMux.Unlock()
		return "", errors.New("已有升级任务在运行")
	}
	info, needUpgrade, err := base.CheckUpdate(getUpgradeSource())
	if err != nil {
		upgradeMux.Unlock()
		return "", fmt.Errorf("获取更新信息失败: %w", err)
	}
	if !needUpgrade {
		upgradeMux.Unlock()
		return "", errors.New("当前已是最新版本")
	}
	upgradeState = &UpgradeState{Running: true, Stage: "downloading", Info: info}
	upgradeMux.Unlock()

	runUpgrade(info)
	return info.Version, nil
}

// 本机升级触发端点：供系统设置页以外的场景调用（节点管理经 /cluster/upgrade 转发，
// 对端节点经节点间令牌调用），非阻塞、立即返回，进度见 UpgradeStatusHandler
func handleUpgradeStart(w http.ResponseWriter, r *http.Request) {
	version, err := beginUpgrade()
	if err != nil {
		RespError(w, RespInternalErr, err.Error())
		return
	}
	RespSucess(w, map[string]any{"message": "upgrade scheduled", "version": version})
}

// 本机升级（SSE 流式）：复用 triggerUpgrade 触发后台升级，再轮询全局状态推流
func StartUpgrade(w http.ResponseWriter, r *http.Request) {
	version, err := beginUpgrade()
	if err != nil {
		RespError(w, RespInternalErr, err.Error())
		return
	}
	dbdata.AdminLog("系统设置", "在线升级", fmt.Sprintf("从 %s 升级到 %s", base.APP_VER, version), r.RemoteAddr)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		base.Error("在线升级: http.Flusher 接口不支持")
		return
	}

	for {
		upgradeMux.Lock()
		st := upgradeState
		upgradeMux.Unlock()
		if st == nil {
			break
		}
		data, _ := json.Marshal(base.UpgradeProgress{Stage: st.Stage, Progress: st.Progress, Error: st.Error})
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
		if st.Stage == "done" || st.Stage == "error" {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// 升级状态轮询
func UpgradeStatusHandler(w http.ResponseWriter, r *http.Request) {
	upgradeMux.Lock()
	state := upgradeState
	upgradeMux.Unlock()

	if state == nil {
		RespSucess(w, map[string]any{
			"running": false,
			"stage":   "idle",
		})
		return
	}

	RespSucess(w, map[string]any{
		"running":  state.Running,
		"stage":    state.Stage,
		"progress": state.Progress,
		"error":    state.Error,
	})
}
