package authsrv

import (
	"fmt"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

func init() {
	auth.Registry.Register("cert", func() auth.Authenticator {
		return &CertAuth{}
	})
}

// 证书步骤仅验证客户端证书的有效性（未过期、未被吊销、匹配已签发的证书记录），
// 证书身份缺失时以 CN 回填用户名
type CertAuth struct {
	auth.NopChallenger
}

func (a *CertAuth) Name() string { return "cert" }

func (a *CertAuth) Authenticate(ctx *auth.Context) (auth.StepResult, error) {
	if ctx.Conn.TLS == nil || len(ctx.Conn.TLS.PeerCertificates) == 0 {
		return auth.StepFail, fmt.Errorf("客户端未提供证书")
	}

	clientCert := ctx.Conn.TLS.PeerCertificates[0]
	username := clientCert.Subject.CommonName
	if len(clientCert.Subject.OrganizationalUnit) == 0 {
		return auth.StepFail, fmt.Errorf("客户端证书缺少 OU 信息")
	}
	groupname := clientCert.Subject.OrganizationalUnit[0]

	if username == "" || groupname == "" {
		return auth.StepFail, fmt.Errorf("客户端证书缺少用户或组信息")
	}

	if groupname != ctx.Conn.GroupName {
		return auth.StepFail, fmt.Errorf("证书绑定组(%s)与认证组(%s)不一致", groupname, ctx.Conn.GroupName)
	}

	deviceID := ctx.Conn.DeviceID
	if !dbdata.ValidateClientCert(clientCert, deviceID) {
		return auth.StepFail, fmt.Errorf("客户端证书验证失败")
	}

	// 以证书 CN 作为认证身份
	if ctx.Conn.Username == "" {
		ctx.Conn.Username = username
	}
	// 记录证书身份断言，供管道末尾一致性检查
	ctx.Identity = username
	// 加载用户信息（含过期时间），供后续步骤与会话创建后 checkSession 检测在线期间到期
	u := &dbdata.User{}
	if dbdata.One("Username", username, u) == nil {
		ctx.SetUserInfo(u.ToAuthInfo())
		ctx.Conn.Nickname = u.Nickname
	}
	base.Info("用户通过证书认证:", dbdata.UserLabel(username, u.Nickname))
	return auth.StepPass, nil
}
