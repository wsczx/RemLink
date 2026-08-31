package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

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

// 读取用户配置的更新源（gitee / github）
func getUpgradeSource() string {
	sc := &dbdata.SettingServerConfig{}
	if err := dbdata.SettingGet(sc); err != nil {
		return "gitee"
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

func StartUpgrade(w http.ResponseWriter, r *http.Request) {
	upgradeMux.Lock()
	if upgradeState != nil && upgradeState.Running {
		upgradeMux.Unlock()
		RespError(w, RespInternalErr, "已有升级任务在运行")
		return
	}

	info, needUpgrade, err := base.CheckUpdate(getUpgradeSource())
	if err != nil {
		upgradeMux.Unlock()
		RespError(w, RespInternalErr, "获取更新信息失败: ", err)
		return
	}
	if !needUpgrade {
		upgradeMux.Unlock()
		RespError(w, RespParamErr, "当前已是最新版本")
		return
	}

	upgradeState = &UpgradeState{
		Running: true,
		Stage:   "downloading",
		Info:    info,
	}
	upgradeMux.Unlock()

	defer func() {
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

	dbdata.AdminLog("系统设置", "在线升级", fmt.Sprintf("从 %s 升级到 %s", base.APP_VER, info.Version), r.RemoteAddr)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		base.Error("在线升级: http.Flusher 接口不支持")
		return
	}

	progressCh := make(chan base.UpgradeProgress, 10)

	go base.DoUpgrade(info, progressCh)

	for p := range progressCh {
		upgradeMux.Lock()
		if upgradeState != nil {
			upgradeState.Stage = p.Stage
			upgradeState.Progress = p.Progress
			upgradeState.Error = p.Error
		}
		upgradeMux.Unlock()

		data, _ := json.Marshal(p)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		if p.Stage == "done" || p.Stage == "error" {
			break
		}
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
