package admin

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/skip2/go-qrcode"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/notify"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/wsczx/remlink/sessdata"
	mail "github.com/xhit/go-simple-mail/v2"
)

func UserList(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	prefix := r.FormValue("prefix")
	prefix = strings.TrimSpace(prefix)
	userType := strings.TrimSpace(r.FormValue("type"))
	group := strings.TrimSpace(r.FormValue("group"))
	status := strings.TrimSpace(r.FormValue("status"))
	pageS := r.FormValue("page")
	page, _ := strconv.Atoi(pageS)
	if page < 1 {
		page = 1
	}

	pageSizeS := r.FormValue("page_size")
	pageSize, _ := strconv.Atoi(pageSizeS)
	if pageSize <= 0 {
		pageSize = dbdata.PageSize // 使用默认值
	}

	var (
		count int
		datas []dbdata.User
		err   error
	)

	// 查询前缀匹配 + 类型 / 用户组 / 状态 筛选
	var wheres []string
	var args []any
	if len(prefix) > 0 {
		fuzzy := "%" + prefix + "%"
		wheres = append(wheres, "(username LIKE ? OR nickname LIKE ? OR email LIKE ? OR type LIKE ?)")
		args = append(args, fuzzy, fuzzy, fuzzy, fuzzy)
	}
	if userType != "" {
		wheres = append(wheres, "type = ?")
		args = append(args, userType)
	}
	if status != "" {
		wheres = append(wheres, "status = ?")
		args = append(args, status)
	}

	var allDatas []dbdata.User
	if len(wheres) > 0 {
		where := strings.Join(wheres, " AND ")
		err = dbdata.FindWhere(&allDatas, 0, 0, where, args...)
	} else {
		err = dbdata.Find(&allDatas, 0, 0)
	}
	if err != nil && !dbdata.CheckErrNotFound(err) {
		RespError(w, RespInternalErr, err)
		return
	}
	if allDatas == nil {
		allDatas = []dbdata.User{}
	}

	if group != "" {
		groupMembers, gerr := dbdata.UsersInGroups([]string{group})
		if gerr != nil {
			RespError(w, RespInternalErr, gerr)
			return
		}
		memberSet := make(map[string]bool, len(groupMembers))
		for _, u := range groupMembers {
			memberSet[u.Username] = true
		}
		filtered := allDatas[:0]
		for _, u := range allDatas {
			if memberSet[u.Username] {
				filtered = append(filtered, u)
			}
		}
		allDatas = filtered
	}

	count = len(allDatas)
	// 内存分页
	start := min((page-1)*pageSize, count)
	end := min(start+pageSize, count)
	datas = allDatas[start:end]

	// 确保空结果返回 [] 而非 null
	if datas == nil {
		datas = []dbdata.User{}
	}

	// 统计卡片按"当前筛选条件下的全量数据"聚合，不受分页影响
	stats, err := userListStats(wheres, args)
	if err != nil && !dbdata.CheckErrNotFound(err) {
		RespError(w, RespInternalErr, err)
		return
	}

	data := map[string]any{
		"count":          count,
		"page_size":      pageSize,
		"datas":          datas,
		"stats_total":    stats.Total,
		"stats_local":    stats.Local,
		"stats_external": stats.External,
		"stats_active":   stats.Active,
		"stats_disable":  stats.Disable,
	}

	RespSucess(w, data)
}

// 在数据库侧做聚合统计（COUNT + CASE WHEN）
// 不全量加载用户对象；external 覆盖所有非本地用户类型（ldap/radius/wxwork/dingtalk/feishu/external）
func userListStats(wheres []string, args []any) (dbdata.UserStats, error) {
	where := ""
	if len(wheres) > 0 {
		where = strings.Join(wheres, " AND ")
	}
	return dbdata.UserStatsWhere(where, args...)
}

func UserDetail(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	idS := r.FormValue("id")
	id, _ := strconv.Atoi(idS)
	if id < 1 {
		RespError(w, RespParamErr, "用户名错误")
		return
	}

	var user dbdata.User
	err := dbdata.One("Id", id, &user)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	RespSucess(w, user)
}

func UserSet(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()

	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()
	data := &dbdata.User{}
	err = json.Unmarshal(body, data)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	// 密码为空时由系统随机生成并通过邮件下发，非空时校验复杂度
	if data.PinCode == "" {
		data.PinCode = utils.RandomRunes(8)
	} else if err := utils.CheckPasswordPolicy(data.PinCode); err != nil {
		RespError(w, RespParamErr, err.Error())
		return
	}
	plainpwd := data.PinCode
	err = dbdata.SetUser(data)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	// 审计日志
	dbdata.AdminLog("用户管理", data.Username, "创建/修改了用户", r.RemoteAddr)

	// 发送邮件（邮件中包含明文密码）
	if data.SendEmail {
		data.PinCode = plainpwd
		err = userAccountMail(data)
		if err != nil {
			RespError(w, RespInternalErr, err)
			return
		}
	}
	// 修改用户资料刷新在线会话缓存的过期时间使其即时生效
	sessdata.UpdateUserLimitTime(data.Username, data.LimitTime)
	RespSucess(w, nil)
}

func UserDel(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	idS := r.FormValue("id")
	id, _ := strconv.Atoi(idS)

	if id < 1 {
		RespError(w, RespParamErr, "用户id错误")
		return
	}

	// 先查出用户名用于审计日志
	user := &dbdata.User{}
	if err := dbdata.One("Id", id, user); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	err := dbdata.Del(user)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	// 删除用户后，令其已签发的 WebVPN 会话立即失效，避免旧会话在有效期内仍可访问
	if err := dbdata.WebVpnRevokeUser(user.Username); err != nil {
		base.Error("用户删除成功但 WebVPN 会话吊销持久化失败:", user.Username, err)
	}
	dbdata.AdminLog("用户管理", user.Username, "删除了用户", r.RemoteAddr)
	RespSucess(w, nil)
}

func UserOtpQr(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	b64S := r.FormValue("b64")
	idS := r.FormValue("id")
	id, _ := strconv.Atoi(idS)

	var b64 bool
	if b64S == "1" {
		b64 = true
	}
	data, err := userOtpQr(id, b64)
	if err != nil {
		base.Error(err)
	}
	io.WriteString(w, data)
}

func userOtpQr(uid int, b64 bool) (string, error) {
	var user dbdata.User
	err := dbdata.One("Id", uid, &user)
	if err != nil {
		return "", err
	}

	issuer := url.QueryEscape(base.GetCfg().Issuer)
	qrstr := fmt.Sprintf("otpauth://totp/%s:%s?issuer=%s&secret=%s", issuer, user.Email, issuer, user.OtpSecret)
	qr, _ := qrcode.New(qrstr, qrcode.High)

	if b64 {
		data, err := qr.PNG(300)
		if err != nil {
			return "", err
		}
		s := base64.StdEncoding.EncodeToString(data)
		return s, nil
	}

	buf := bytes.NewBuffer(nil)
	err = qr.Write(300, buf)
	return buf.String(), err
}

// 在线用户
func UserOnline(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	search_cate := r.FormValue("search_cate")
	search_text := r.FormValue("search_text")
	show_sleeper := r.FormValue("show_sleeper")
	showSleeper, _ := strconv.ParseBool(show_sleeper)
	// one_offline := r.FormValue("one_offline")

	// datas := sessdata.OnlineSess()
	datas := sessdata.GetOnlineSess(search_cate, search_text, showSleeper)

	data := map[string]any{
		"count":     len(datas),
		"page_size": dbdata.PageSize,
		"datas":     datas,
	}

	RespSucess(w, data)
}

func UserOffline(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	token := r.FormValue("token")
	dbdata.AdminLog("用户管理", token, "踢用户下线", r.RemoteAddr)
	sessdata.CloseSess(token, dbdata.UserLogoutAdmin)
	RespSucess(w, nil)
}

func UserReline(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	token := r.FormValue("token")
	dbdata.AdminLog("用户管理", token, "断开用户连接", r.RemoteAddr)
	sessdata.CloseCSess(token)
	RespSucess(w, nil)
}

func UserResetTraffic(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	username := strings.TrimSpace(r.FormValue("username"))
	if username == "" {
		RespError(w, RespParamErr, "用户名不能为空")
		return
	}

	u := &dbdata.User{}
	if err := dbdata.One("Username", username, u); err != nil {
		RespError(w, RespParamErr, "用户不存在")
		return
	}

	// 重置流量计数和重置时间
	_, err := dbdata.GetXdb().Where("username=?", username).
		Cols("traffic_used", "traffic_reset_at").
		Update(&dbdata.User{})
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	dbdata.AdminLog("用户管理", username, "重置流量配额", r.RemoteAddr)
	RespSucess(w, nil)
}

// 批量发送邮件
func UserBatchSendEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserIds []int `json:"user_ids"`
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	if len(req.UserIds) == 0 {
		RespError(w, RespInternalErr, errors.New("用户ID列表不能为空"))
		return
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	successCount := 0
	failCount := 0

	// 限制最大并发数
	maxConcurrency := min(len(req.UserIds), 10)
	concurrencyLimiter := make(chan struct{}, maxConcurrency)

	for _, userId := range req.UserIds {
		wg.Add(1)
		go func(uid int) {
			defer wg.Done()
			concurrencyLimiter <- struct{}{}
			defer func() { <-concurrencyLimiter }()

			user := &dbdata.User{}
			err := dbdata.One("Id", uid, user)
			if err != nil {
				base.Error("批量发送邮件失败，获取用户信息错误:", uid, err)
				mu.Lock()
				failCount++
				mu.Unlock()
				return
			}

			err = userAccountMail(user)
			mu.Lock()
			if err != nil {
				base.Error("批量发送邮件失败:", user.Username, err)
				failCount++
			} else {
				successCount++
			}
			mu.Unlock()
		}(userId)
	}

	wg.Wait()

	msg := fmt.Sprintf("批量发送邮件完成，成功：%d，失败：%d", successCount, failCount)
	dbdata.AdminLog("用户管理", "批量操作", "批量发送邮件(成功:"+strconv.Itoa(successCount)+",失败:"+strconv.Itoa(failCount)+")", r.RemoteAddr)

	if successCount > 0 {
		RespSucess(w, msg)
	} else {
		RespError(w, RespInternalErr, errors.New(msg))
	}
}

// 批量删除用户
func UserBatchDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserIds []int `json:"user_ids"`
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	if len(req.UserIds) == 0 {
		RespError(w, RespInternalErr, errors.New("用户ID列表不能为空"))
		return
	}

	successCount := 0
	failCount := 0
	skipCount := 0

	// 一次性查出待删除用户（1 次查询，避免逐个 One 的 N+1），同时收集 username 供后续吊销 WebVPN 会话
	var users []dbdata.User
	if err := dbdata.GetXdb().In("id", req.UserIds).Find(&users); err != nil {
		base.Error("批量删除用户-查询失败:", err)
		RespError(w, RespInternalErr, err)
		return
	}
	usernames := make([]string, 0, len(users))
	for _, u := range users {
		usernames = append(usernames, u.Username)
	}

	// 单条 DELETE ... WHERE id IN (?) 一次删完（1 次写，避免逐个 Del 在 SQLite 单写锁下反复竞争导致部分删除失败）
	affected, err := dbdata.GetXdb().In("id", req.UserIds).Delete(&dbdata.User{})
	if err != nil {
		base.Error("批量删除用户失败:", err)
		RespError(w, RespInternalErr, err)
		return
	}
	successCount = int(affected)
	// 列表里不在库中的（已删除/重复提交）计入跳过
	skipCount = len(req.UserIds) - successCount

	// 删除后令其已签发的 WebVPN 会话失效（纯收尾安全动作，异步执行避免阻塞响应）
	if len(usernames) > 0 {
		go dbdata.WebVpnRevokeUsers(usernames)
	}

	dbdata.AdminLog("用户管理", "批量操作", "批量删除了"+strconv.Itoa(successCount)+"个用户", r.RemoteAddr)

	// 已删除（跳过）的用户不计入失败，避免整批被误报为“删除失败”
	if successCount == 0 && failCount == 0 {
		RespSucess(w, "所选用户均不存在，无需删除")
		return
	}
	if failCount == 0 {
		RespSucess(w, fmt.Sprintf("批量删除完成，成功：%d，跳过：%d（已不存在）", successCount, skipCount))
		return
	}
	RespError(w, RespInternalErr, fmt.Errorf("批量删除完成，成功：%d，失败：%d，跳过：%d（已不存在）", successCount, failCount, skipCount))
}

type userAccountMailData struct {
	Issuer       string
	LinkAddr     string
	Group        string
	Username     string
	Nickname     string
	PinCode      string
	LimitTime    string
	OtpImg       string
	OtpImgBase64 string
	DisableOtp   bool
}

func userAccountMail(user *dbdata.User) error {
	// 平台通知
	htmlBody := `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta http-equiv="Content-Type" content="text/html; charset=utf-8"/>
    <title>Hello RemLink!</title>
</head>
<body>
%s
</body>
</html>
`
	dataOther := &dbdata.SettingOther{}
	err := dbdata.SettingGet(dataOther)
	if err != nil {
		base.Error(err)
		return err
	}
	htmlBody = fmt.Sprintf(htmlBody, dataOther.AccountMail)

	// token有效期3天
	expiresAt := time.Now().Unix() + 3600*24*3
	jwtData := map[string]any{"id": user.Id}
	tokenString, err := SetJwtData(jwtData, expiresAt)
	if err != nil {
		return err
	}

	setting := &dbdata.SettingOther{}
	err = dbdata.SettingGet(setting)
	if err != nil {
		base.Error(err)
		return err
	}

	otpData, _ := userOtpQr(user.Id, true)

	data := userAccountMailData{
		Issuer:       base.GetCfg().Issuer,
		LinkAddr:     setting.LinkAddr,
		Group:        strings.Join(user.Groups, ","),
		Username:     user.Username,
		Nickname:     user.Nickname,
		PinCode:      user.PinCode,
		OtpImg:       fmt.Sprintf("https://%s/otp_qr?id=%d&jwt=%s", setting.LinkAddr, user.Id, tokenString),
		OtpImgBase64: "data:image/png;base64," + otpData,
		DisableOtp:   user.DisableOtp,
	}

	if user.Type == "ldap" {
		data.PinCode = "同ldap密码"
	}

	if user.LimitTime == nil {
		data.LimitTime = "无限制"
	} else {
		data.LimitTime = user.LimitTime.Local().Format("2006-01-02")
	}

	w := bytes.NewBufferString("")
	t, _ := template.New("auth_complete").Parse(htmlBody)
	err = t.Execute(w, data)
	if err != nil {
		return err
	}

	var attach *mail.File
	if user.DisableOtp {
		attach = nil
	} else {
		imgData, _ := userOtpQr(user.Id, false)
		attach = &mail.File{
			MimeType: "image/png",
			Name:     "userOtpQr.png",
			Data:     []byte(imgData),
			Inline:   true,
		}
	}

	return notify.GetNotify().SendEmail(notify.Message{
		Subject:    base.GetCfg().Issuer,
		To:         user.Email,
		Body:       w.String(),
		Attachment: attach,
	})
}
