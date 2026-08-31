package admin

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/dbdata"
)

// 查询锁定信息
func GetLocksInfo(w http.ResponseWriter, r *http.Request) {
	infos := auth.GetLockManager().LockInfo()
	names := make([]string, 0, len(infos))
	for _, info := range infos {
		names = append(names, info.Username)
	}
	nickMap := dbdata.NicknameMap(names)
	for i := range infos {
		infos[i].Nickname = nickMap[infos[i].Username]
	}
	RespSucess(w, infos)
}

// 解锁用户/IP
func UnlockUser(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	var req struct {
		Description string          `json:"description"`
		Username    string          `json:"username"`
		IP          string          `json:"ip"`
		State       *auth.LockState `json:"state"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if req.State == nil {
		RespError(w, RespInternalErr, "未找到锁定用户！")
		return
	}

	lm := auth.GetLockManager()
	switch {
	case req.Username != "" && req.IP != "":
		// 单用户IP锁定
		lm.UnlockUserIP(req.Username, req.IP)
	case req.Username != "":
		// 全局用户锁定：解该用户的所有锁定
		lm.UnlockUser(req.Username)
	case req.IP != "":
		// 全局IP锁定：解该 IP 的所有锁定
		lm.UnlockIP(req.IP)
	}

	dbdata.AdminLog("用户管理", req.Username, "解锁了用户/IP:"+req.Description, r.RemoteAddr)
	RespSucess(w, "解锁成功！")
}
