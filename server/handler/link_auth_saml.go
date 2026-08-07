// SSO OAuth 认证端点：企业微信 / 飞书扫码登录。
package handler

import (
	"encoding/base64"
	"fmt"
	"html"
	"net/http"
	"net/url"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

const ssoStatePrefixLen = 32 // SSO state 随机串长度

// SAML 服务提供商登录入口（根据 ssotype 分发到企微/飞书 OAuth）
func SAMLSPLogin(w http.ResponseWriter, r *http.Request) {
	tgname := r.URL.Query().Get("tgname")
	if tgname == "" {
		base.Error("缺少组名参数")
		http.Error(w, "缺少组名参数", http.StatusBadRequest)
		return
	}

	// 校验该组的认证配置确实包含请求的 SSO 类型
	groupData := &dbdata.Group{}
	if err := dbdata.One("Name", tgname, groupData); err != nil {
		if dbdata.CheckErrNotFound(err) {
			base.Error("组不存在:", tgname)
			http.Error(w, "组不存在", http.StatusBadRequest)
			return
		}
		base.Error("查询组失败:", tgname, err)
		http.Error(w, "系统繁忙，请稍后重试", http.StatusServiceUnavailable)
		return
	}

	ssotype := r.URL.Query().Get("ssotype")
	redirect := r.URL.Query().Get("redirect")

	if ssotype == "feishu" {
		if !dbdata.HasAuthType(groupData.AuthProfile, "feishu") {
			base.Error("组未配置飞书认证:", tgname)
			http.Error(w, "组未配置飞书认证", http.StatusBadRequest)
			return
		}
		startSSO(w, r, tgname, "feishu", redirect)
		return
	}

	if ssotype == "dingtalk" {
		if !dbdata.HasAuthType(groupData.AuthProfile, "dingtalk") {
			base.Error("组未配置钉钉认证:", tgname)
			http.Error(w, "组未配置钉钉认证", http.StatusBadRequest)
			return
		}
		startSSO(w, r, tgname, "dingtalk", redirect)
		return
	}

	// 默认企微
	if !dbdata.HasAuthType(groupData.AuthProfile, "wxwork") {
		base.Error("组未配置企微认证:", tgname)
		http.Error(w, "组未配置企微认证", http.StatusBadRequest)
		return
	}
	startSSO(w, r, tgname, "wxwork", redirect)
}

// 企业微信 OAuth2 回调
func WXAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		base.Error("企微认证回调缺少参数")
		http.Error(w, "缺少认证参数", http.StatusBadRequest)
		return
	}

	pending, groupname, ok := verifySAMLPending(w, r, state)
	if !ok {
		return
	}

	wxworkConfig, err := dbdata.GetAuthWework(groupname)
	if err != nil {
		base.Error("获取企微配置失败", err)
		SAMLError(w, err)
		return
	}

	userID, err := wxworkConfig.GetWeworkUser(code)
	if err != nil {
		base.Error("用户信息获取失败", err)
		SAMLError(w, err)
		return
	}

	// 部门过滤：在回调阶段完成校验，避免后续管道绕过
	allowedDepts := wxworkConfig.ParseDepartments()
	if len(allowedDepts) > 0 {
		ok, err := wxworkConfig.CheckUserDepartment(userID, allowedDepts)
		if err != nil {
			base.Error("验证部门失败", err)
			SAMLError(w, err)
			return
		}
		if !ok {
			base.Error("用户不在允许的部门范围内:", userID)
			SAMLError(w, fmt.Errorf("用户不在允许的部门范围内"))
			return
		}
	}

	// 用户ID拒绝清单：在回调阶段完成校验，避免后续管道绕过
	blockedUserIDs := wxworkConfig.ParseBlockedUserIDs()
	if len(blockedUserIDs) > 0 && wxworkConfig.CheckUserID(userID, blockedUserIDs) {
		base.Error("用户在被拒绝的用户ID列表中:", userID)
		SAMLError(w, fmt.Errorf("用户在被拒绝的用户ID列表中"))
		return
	}

	finishSAMLOAuth(w, r, state, "wxwork", userID, pending)
}

// SAML 断言消费者端点（sso-v2-login-final）
func SAMLACLogin(w http.ResponseWriter, r *http.Request) {
	encodedToken, err := GetCookie(r, "acSamlv2Token")
	if err != nil || encodedToken == "" {
		base.Error("认证信息丢失,获取Cookie失败")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("认证失败：无法获取认证信息"))
		return
	}

	tokenBytes, err := base64.StdEncoding.DecodeString(encodedToken)
	if err != nil {
		base.Error("Cookie解码失败", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	token := string(tokenBytes)

	if isAnyConnectInternalBrowser(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(samlSuccessHTML))
		return
	}
	// WebAuth认证成功后直接跳转到门户
	isWebAuth := false
	if samlSession, err := AuthSessionManager.Get(token); err == nil {
		if samlSession.Ctx != nil && samlSession.Ctx.SSO != nil && samlSession.Ctx.SSO.WebAuthCompleted {
			isWebAuth = true
		}
	}

	returnURL := r.URL.Query().Get("return")
	if returnURL == "" {
		if isWebAuth {
			returnURL = fmt.Sprintf("%s/ui/#/portal", getServerAddr(r))
		} else {
			returnURL = fmt.Sprintf("%s/+CSCOE+/saml/sp/done", getServerAddr(r))
		}
	}

	localAPIURL := fmt.Sprintf("http://localhost:29786/api/sso/%s?return=%s",
		url.QueryEscape(token),
		url.QueryEscape(returnURL))
	http.Redirect(w, r, localAPIURL, http.StatusFound)
}

// SAML 认证完成页面
func SAMLDone(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(samlSuccessHTML))
}

// 企业微信域名验证端点（对应 wxwork 认证源配置的 verify_file_content）
func SAMLTest(w http.ResponseWriter, r *http.Request) {
	content, ok := wxworkVerifyFileByPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Write([]byte(content))
}

// 根据请求路径匹配启用的 wxwork 认证源的验证文件
func wxworkVerifyFileByPath(path string) (string, bool) {
	for _, name := range dbdata.ProviderNamesByType("wxwork") {
		cfg, err := dbdata.ResolveProviderConfig(name, "wxwork")
		if err != nil {
			continue
		}
		fn, _ := cfg["verify_file_name"].(string)
		fc, _ := cfg["verify_file_content"].(string)
		if fn != "" && path == "/"+fn {
			return fc, true
		}
	}
	return "", false
}

// 飞书 OAuth 登录入口已合并到 SAMLSPLogin（按 ssotype 参数分发），
// 无需单独的登录端点。回调端点 FeishuAuthCallback 保持不变。

// 飞书 OAuth2 回调
func FeishuAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		base.Error("飞书认证回调缺少参数")
		http.Error(w, "缺少认证参数", http.StatusBadRequest)
		return
	}

	pending, groupname, ok := verifySAMLPending(w, r, state)
	if !ok {
		return
	}

	feishuConfig, err := dbdata.GetAuthFeishu(groupname)
	if err != nil {
		base.Error("获取飞书配置失败", err)
		SAMLError(w, err)
		return
	}

	userID, err := feishuConfig.GetFeishuUser(code)
	if err != nil {
		base.Error("飞书用户信息获取失败", err)
		SAMLError(w, err)
		return
	}

	// 部门过滤：在回调阶段完成校验，避免后续管道绕过
	allowedDepts := feishuConfig.ParseDepartments()
	if len(allowedDepts) > 0 {
		accessToken, err := feishuConfig.GetAppAccessToken()
		if err != nil {
			base.Error("获取飞书 access_token 失败", err)
			SAMLError(w, err)
			return
		}
		ok, err := feishuConfig.CheckUserDepartment(accessToken, userID, allowedDepts)
		if err != nil {
			base.Error("验证部门失败", err)
			SAMLError(w, err)
			return
		}
		if !ok {
			base.Error("用户不在允许的部门范围内:", userID)
			SAMLError(w, fmt.Errorf("用户不在允许的部门范围内"))
			return
		}
	}

	// 拒绝名单：在回调阶段完成校验，避免后续管道绕过
	blockedUserIDs := feishuConfig.ParseBlockedUserIDs()
	if len(blockedUserIDs) > 0 && feishuConfig.CheckUserID(userID, blockedUserIDs) != nil {
		base.Error("用户在拒绝名单中:", userID)
		SAMLError(w, fmt.Errorf("用户已被拒绝登录"))
		return
	}

	finishSAMLOAuth(w, r, state, "feishu", userID, pending)
}

// DingtalkAuthCallback 钉钉 OAuth 扫码登录回调。
// 与 WXAuthCallback / FeishuAuthCallback 对称：在回调阶段完成部门过滤与用户ID拒绝清单校验。
func DingtalkAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		base.Error("钉钉认证回调缺少参数")
		http.Error(w, "缺少认证参数", http.StatusBadRequest)
		return
	}

	pending, groupname, ok := verifySAMLPending(w, r, state)
	if !ok {
		return
	}

	dingtalkConfig, err := dbdata.GetAuthDingtalk(groupname)
	if err != nil {
		base.Error("获取钉钉配置失败", err)
		SAMLError(w, err)
		return
	}

	userID, accessToken, err := dingtalkConfig.GetDingtalkUser(code)
	if err != nil {
		base.Error("钉钉用户信息获取失败", err)
		SAMLError(w, err)
		return
	}

	// 部门过滤：在回调阶段完成校验，避免后续管道绕过
	allowedDepts := dingtalkConfig.ParseDepartments()
	if len(allowedDepts) > 0 {
		ok, err := dingtalkConfig.CheckUserDepartment(accessToken, userID, allowedDepts)
		if err != nil {
			base.Error("验证部门失败", err)
			SAMLError(w, err)
			return
		}
		if !ok {
			base.Error("用户不在允许的部门范围内:", userID)
			SAMLError(w, fmt.Errorf("用户不在允许的部门范围内"))
			return
		}
	}

	// 用户ID拒绝清单：在回调阶段完成校验，避免后续管道绕过
	if err := dingtalkConfig.CheckUserID(userID, dingtalkConfig.ParseBlockedUserIDs()); err != nil {
		base.Error("用户在被拒绝的用户ID列表中:", userID)
		SAMLError(w, err)
		return
	}

	// 创建完整 SSO 会话（覆盖 pending 状态）
	finishSAMLOAuth(w, r, state, "dingtalk", userID, pending)
}
func SAMLError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	errMsg := html.EscapeString(err.Error())
	errorPage := `
<!DOCTYPE html>
<html>
<head>
    <title>认证失败</title>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            background: #f5f5f5;
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
            margin: 0;
            padding: 20px;
        }
        .container {
            background: white;
            border-radius: 8px;
            box-shadow: 0 2px 15px rgba(0,0,0,0.2);
            padding: 40px;
            max-width: 450px;
            width: 100%;
            text-align: center;
        }
        .icon {
            font-size: 56px;
            color: #f44336; 
            margin-bottom: 24px;
        }
        h1 {
            color: #303133;
            font-size: 24px;
            margin-bottom: 15px;
        }
        .error-message {
            color: #e6a23c;
            font-size: 16px;
            margin-bottom: 20px;
        }
        .detail {
            color: #606266;
            font-size: 15px;
            margin-bottom: 25px;
        }
        .note {
            color: #909399;
            font-size: 13px;
            margin-top: 30px;
            border-top: 1px solid #e4e7ed;
            padding-top: 20px;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="icon">❌</div>
        <h1>认证失败</h1>
        <div class="error-message">` + errMsg + `</div>
        <p class="detail">请确认您所在的部门是否在允许的范围内，或联系管理员解决。</p>
        <div class="note">
            提示：由于浏览器安全限制，网页可能无法自动关闭。<br>
            请手动关闭此页面并返回 AnyConnect 客户端。
        </div>
    </div>
</body>
</html>`
	w.Write([]byte(errorPage))
}
