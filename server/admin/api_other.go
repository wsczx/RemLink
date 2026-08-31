package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/notify"
)

func setOtherGet(data any, w http.ResponseWriter) {
	err := dbdata.SettingGet(data)
	if err != nil && !dbdata.CheckErrNotFound(err) {
		RespError(w, RespInternalErr, err)
		return
	}
	switch v := data.(type) {
	case *dbdata.SettingSmtp:
		v.Password = v.Password.Masked()
	case *dbdata.SettingSms:
		v.AliAccessKeySecret = v.AliAccessKeySecret.Masked()
		v.TencentSecretKey = v.TencentSecretKey.Masked()
	}
	RespSucess(w, data)
}

func setOtherEdit(data any, w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<21) // 2MB：放宽品牌 logo/favicon（base64）等配置的保存上限
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()

	err = json.Unmarshal(body, data)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	switch v := data.(type) {
	case *dbdata.SettingSmtp:
		if v.Password.IsPlaceholder() {
			old := &dbdata.SettingSmtp{}
			if err := dbdata.SettingGet(old); err == nil {
				v.Password = old.Password
			}
		}
	case *dbdata.SettingSms:
		old := &dbdata.SettingSms{}
		if err := dbdata.SettingGet(old); err == nil {
			if v.Provider == "" {
				// 关闭短信：仅置空 provider，保留其余已配置字段，避免清空已填配置
				v.AliAccessKeyId = old.AliAccessKeyId
				v.AliAccessKeySecret = old.AliAccessKeySecret
				v.AliSignName = old.AliSignName
				v.AliTemplateCode = old.AliTemplateCode
				v.TencentSecretId = old.TencentSecretId
				v.TencentSecretKey = old.TencentSecretKey
				v.TencentSdkAppId = old.TencentSdkAppId
				v.TencentSignName = old.TencentSignName
				v.TencentTemplateId = old.TencentTemplateId
				v.TencentRegion = old.TencentRegion
			} else {
				// 启用/编辑：仅当密钥为占位符时沿用旧值，空串视为用户主动清空
				if v.AliAccessKeySecret.IsPlaceholder() {
					v.AliAccessKeySecret = old.AliAccessKeySecret
				}
				if v.TencentSecretKey.IsPlaceholder() {
					v.TencentSecretKey = old.TencentSecretKey
				}
			}
		}
	}
	err = dbdata.SettingSave(data)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	switch v := data.(type) {
	case *dbdata.SettingSmtp:
		v.Password = v.Password.Masked()
	case *dbdata.SettingSms:
		v.AliAccessKeySecret = v.AliAccessKeySecret.Masked()
		v.TencentSecretKey = v.TencentSecretKey.Masked()
	}
	RespSucess(w, data)
}

func SetOtherSmtp(w http.ResponseWriter, r *http.Request) {
	data := &dbdata.SettingSmtp{}
	setOtherGet(data, w)
}

func SetOtherSmtpEdit(w http.ResponseWriter, r *http.Request) {
	data := &dbdata.SettingSmtp{}
	setOtherEdit(data, w, r)
	dbdata.AdminLog("系统设置", "SMTP配置", "修改了SMTP配置", r.RemoteAddr)
}

func SetOther(w http.ResponseWriter, r *http.Request) {
	data := &dbdata.SettingOther{}
	setOtherGet(data, w)
}

func SetOtherEdit(w http.ResponseWriter, r *http.Request) {
	data := &dbdata.SettingOther{}
	setOtherEdit(data, w, r)
	dbdata.AdminLog("系统设置", "通用设置", "修改了通用设置", r.RemoteAddr)
}

func SetOtherAuditLog(w http.ResponseWriter, r *http.Request) {
	data, err := dbdata.SettingGetAuditLog()
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	auditInterval := base.GetCfg().AuditInterval

	resp := map[string]any{
		"life_day":       data.LifeDay,
		"clear_time":     data.ClearTime,
		"audit_interval": auditInterval,
	}
	RespSucess(w, resp)
}

func SetOtherAuditLogEdit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<21) // 2MB：放宽品牌 logo/favicon（base64）等配置的保存上限
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()
	data := &dbdata.SettingAuditLog{}
	err = json.Unmarshal(body, data)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if data.LifeDay < 0 || data.LifeDay > 365 {
		RespError(w, RespParamErr, errors.New("日志存储时长范围在 0 ~ 365"))
		return
	}
	ok, _ := regexp.Match("^([0-9]|0[0-9]|1[0-9]|2[0-3]):([0][0])$", []byte(data.ClearTime))
	if !ok {
		RespError(w, RespParamErr, errors.New("每天清理时间填写有误"))
		return
	}
	err = dbdata.SettingSave(data)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	dbdata.AdminLog("系统设置", "审计日志设置", "修改了审计日志设置", r.RemoteAddr)
	RespSucess(w, data)
}

func SetOtherSms(w http.ResponseWriter, r *http.Request) {
	data := &dbdata.SettingSms{}
	setOtherGet(data, w)
}

func SetOtherSmsEdit(w http.ResponseWriter, r *http.Request) {
	data := &dbdata.SettingSms{}
	setOtherEdit(data, w, r)
	dbdata.AdminLog("系统设置", "短信配置", "修改了短信配置", r.RemoteAddr)
}

func SetPortalBrand(w http.ResponseWriter, r *http.Request) {
	data := &dbdata.SettingPortalBrand{}
	setOtherGet(data, w)
}

func SetPortalBrandEdit(w http.ResponseWriter, r *http.Request) {
	data := &dbdata.SettingPortalBrand{}
	setOtherEdit(data, w, r)
	dbdata.AdminLog("系统设置", "品牌展示", "修改了品牌展示配置", r.RemoteAddr)
}
func SetPortalDashboard(w http.ResponseWriter, r *http.Request) {
	data := &dbdata.SettingPortalDashboard{}
	err := dbdata.SettingGet(data)
	if err != nil {
		if !dbdata.CheckErrNotFound(err) {
			RespError(w, RespInternalErr, err)
			return
		}
		// 初始化默认值
		data.ClientDownloadHtml = base.DefaultDownloadHtml
	}
	RespSucess(w, data)
}

func SetPortalDashboardEdit(w http.ResponseWriter, r *http.Request) {
	data := &dbdata.SettingPortalDashboard{}
	setOtherEdit(data, w, r)
	dbdata.AdminLog("系统设置", "门户首页", "修改了门户首页配置", r.RemoteAddr)
}

func SetOtherSmsTest(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<21) // 2MB：放宽品牌 logo/favicon（base64）等配置的保存上限
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()

	var req struct {
		Phone string `json:"phone"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if req.Phone == "" {
		RespError(w, RespParamErr, errors.New("手机号不能为空"))
		return
	}

	smsCfg := &dbdata.SettingSms{}
	if err := dbdata.SettingGet(smsCfg); err != nil {
		RespError(w, RespInternalErr, errors.New("短信配置未找到"))
		return
	}
	if smsCfg.Provider == "" {
		RespError(w, RespParamErr, errors.New("短信服务未启用"))
		return
	}

	if err := notify.SendSmsTest(smsCfg, req.Phone); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	RespSucess(w, "短信发送成功")
}

// 返回品牌展示配置，供管理后台侧边栏与登录页在 8800 端口直接加载。
func AdminPortalLoginConfig(w http.ResponseWriter, r *http.Request) {
	brand := dbdata.SettingPortalBrand{}
	_ = dbdata.SettingGet(&brand)
	RespSucess(w, map[string]any{
		"title":   brand.Title,
		"logo":    brand.Logo,
		"favicon": brand.Favicon,
		"desc":    brand.Desc,
		"footer":  brand.Footer,
	})
}
