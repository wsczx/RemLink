package handler

import (
	"crypto/sha1"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/pires/go-proxyproto"
	"github.com/wsczx/remlink/admin"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
)

func startTls() {

	var (
		err error

		addr = base.GetCfg().ServerAddr
		ln   net.Listener
	)

	// 判断证书文件
	// _, err = os.Stat(certFile)
	// if errors.Is(err, os.ErrNotExist) {
	//	// 自动生成证书
	//	certs[0], err = selfsign.GenerateSelfSignedWithDNS("vpn.remlink")
	// } else {
	//	// 使用自定义证书
	//	certs[0], err = tls.LoadX509KeyPair(certFile, keyFile)
	// }

	tlscert, _, err := dbdata.ParseCert()
	if err != nil {
		base.Fatal("证书加载失败", err)
	}
	// 主证书作为 default；WebVPN 泛域名证书并存
	certs := []*tls.Certificate{tlscert}
	if wildCert, _, werr := dbdata.ParseCertWild(); werr == nil && wildCert != nil {
		certs = append(certs, wildCert)
	}
	dbdata.LoadCertificates(certs)

	// 证书的 SHA1 指纹，只用主证书
	s1 := sha1.New()
	s1.Write(tlscert.Certificate[0])
	h2s := hex.EncodeToString(s1.Sum(nil))
	certHash.Store(strings.ToUpper(h2s))
	base.Debug("certHash", certHash.Load())

	// 仅启用安全的 TLS 密码套件
	cipherSuites := tls.CipherSuites()
	selectedCipherSuites := make([]uint16, 0, len(cipherSuites))
	for _, s := range cipherSuites {
		selectedCipherSuites = append(selectedCipherSuites, s.ID)
	}

	// 设置tls信息
	tlsConfig := &tls.Config{
		NextProtos:   []string{"http/1.1"},
		MinVersion:   tls.VersionTLS12,
		CipherSuites: selectedCipherSuites,
		GetCertificate: func(chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return dbdata.GetCertificateBySNI(chi.ServerName)
		},
	}
	// 客户端证书请求，支持热更新
	tlsConfig.GetConfigForClient = func(chi *tls.ClientHelloInfo) (*tls.Config, error) {
		if !dbdata.AnyGroupHasCertAuth() {
			return nil, nil // 无 cert 组不请求证书
		}
		c := tlsConfig.Clone()
		c.ClientAuth = tls.RequestClientCert    // 请求客户端证书
		c.ClientCAs = dbdata.LoadClientCAPool() // 加载客户端CA证书
		return c, nil
	}
	srv := &http.Server{
		Addr:         addr,
		Handler:      webVpnRootHandler(),
		TLSConfig:    tlsConfig,
		ErrorLog:     base.GetServerLog(),
		ReadTimeout:  100 * time.Second,
		WriteTimeout: 100 * time.Second,
	}

	ln, err = net.Listen("tcp", base.FormatListenAddr(addr))
	if err != nil {
		base.Error("VPN 服务监听失败，请到管理后台「软件配置→服务监听→VPN 服务地址」修改端口后重启:", err)
		return
	}
	defer ln.Close()

	if base.GetCfg().ProxyProtocol {
		ln = &proxyproto.Listener{
			Listener:          ln,
			ReadHeaderTimeout: 30 * time.Second,
		}
	}

	base.Info("listen server", addr)
	err = srv.ServeTLS(ln, "", "")
	if err != nil {
		base.Error("VPN 服务运行异常:", err)
		return
	}
}

// 外层路由：先判断是否为 WebVPN 子域请求（*.WebVpnDomain）。
// 是则进入 WebVPN 分支（独立处理，不套全局安全头，避免 CSP 污染被代理内容）；
// 否则 delegate 回 initRoute()（现有 CSTP/portal/WebAuth 等路由零侵入）。
func webVpnRootHandler() http.Handler {
	root := initRoute()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if WebVpnHandler(w, r) {
			return
		}
		root.ServeHTTP(w, r)
	})
}

func initRoute() http.Handler {
	r := mux.NewRouter()
	// 所有路由添加安全头
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			utils.SetSecureHeader(w)
			next.ServeHTTP(w, req)
		})
	})

	r.HandleFunc("/", LinkHome).Methods(http.MethodGet)
	r.HandleFunc("/", LinkAuth).Methods(http.MethodPost)
	r.HandleFunc("/portal", PortalHome).Methods(http.MethodGet)
	r.HandleFunc("/portal/", PortalHome).Methods(http.MethodGet)
	// 子域名登录可放行的门户接口：与 WebVpnHandler 的放行判断共用 portalLoginEndpoints 单一来源，
	// 新增子域名登录必需的门户接口只需改 portalLoginEndpoints（webvpn.go）。
	for _, ep := range portalLoginEndpoints {
		r.HandleFunc(ep.path, ep.handler).Methods(ep.method)
	}
	r.HandleFunc("/portal/api/my_groups", PortalMyGroups).Methods(http.MethodGet)
	r.HandleFunc("/portal/api/change_password", PortalChangePassword).Methods(http.MethodPost)
	r.HandleFunc("/portal/api/force_change_password", PortalForceChangePassword).Methods(http.MethodPost)
	r.HandleFunc("/portal/api/logout", PortalLogout).Methods(http.MethodPost)
	r.HandleFunc("/portal/api/otp/regenerate", PortalOTPRegenerate).Methods(http.MethodPost)
	r.HandleFunc("/portal/api/forgot_password", PortalForgotPassword).Methods(http.MethodPost)
	r.HandleFunc("/portal/api/reset_password/verify", PortalResetPasswordVerify).Methods(http.MethodGet)
	r.HandleFunc("/portal/api/reset_password", PortalResetPassword).Methods(http.MethodPost)
	r.HandleFunc("/portal/api/certs", PortalCertList).Methods(http.MethodGet)
	r.HandleFunc("/portal/api/certs/download", PortalCertDownload).Methods(http.MethodPost)
	r.HandleFunc("/portal/api/devices", PortalDevices).Methods(http.MethodGet)
	r.HandleFunc("/portal/api/devices/offline", PortalDeviceOffline).Methods(http.MethodPost)
	// WebVPN 用户侧端点（独立会话：webvpn_session）
	r.HandleFunc("/webvpn/login-config", webVpnLoginConfig).Methods(http.MethodGet)
	r.HandleFunc("/webvpn/me", webVpnMe).Methods(http.MethodGet)
	r.HandleFunc("/webvpn/my-apps", webVpnMyApps).Methods(http.MethodGet)
	r.HandleFunc("/webvpn/logout", webVpnLogout).Methods(http.MethodGet, http.MethodPost)
	// WebAuth 端点
	r.HandleFunc("/web-auth/start", WebAuthStart).Methods(http.MethodGet)
	r.HandleFunc("/web-auth/identify", WebAuthIdentify).Methods(http.MethodPost)
	r.HandleFunc("/web-auth/groups", WebAuthSelectGroup).Methods(http.MethodPost)
	r.HandleFunc("/web-auth/step", WebAuthStep).Methods(http.MethodPost)
	r.HandleFunc("/web-auth/sms/resend", WebAuthSmsResend).Methods(http.MethodPost)
	r.HandleFunc("/web-auth/continue", WebAuthContinue).Methods(http.MethodPost)
	r.HandleFunc("/web-auth/sso-callback", WebAuthSSOCallback).Methods(http.MethodGet)
	r.HandleFunc("/web-auth/complete", WebAuthComplete).Methods(http.MethodGet)
	r.HandleFunc("/web-auth/change_password", WebAuthChangePassword).Methods(http.MethodPost)
	r.HandleFunc("/+CSCOE+/web-auth/sp/login", WebAuthSPLogin).Methods(http.MethodGet)
	r.PathPrefix("/ui/").Handler(admin.ServeUI())
	r.HandleFunc("/CSCOSSLC/tunnel", LinkTunnel).Methods(http.MethodConnect)
	r.HandleFunc("/otp_qr", LinkOtpQr).Methods(http.MethodGet)
	r.HandleFunc("/otp-verification", LinkAuth_otp).Methods(http.MethodPost)
	// 添加Cisco AnyConnect兼容的SAML端点
	r.HandleFunc("/+CSCOE+/saml/sp/login", SAMLSPLogin).Methods(http.MethodGet)
	r.HandleFunc("/+CSCOE+/saml_ac_login.html", SAMLACLogin).Methods(http.MethodGet)
	r.HandleFunc("/+CSCOE+/saml/sp/done", SAMLDone)
	r.HandleFunc("/+CSCOE+/force-pwd", ForcePwdPage).Methods(http.MethodGet)
	r.HandleFunc("/+CSCOE+/force-pwd/submit", ForcePwdSubmit).Methods(http.MethodPost)
	// 添加企业微信回调路由
	r.HandleFunc("/WXAuth/callback", WXAuthCallback).Methods(http.MethodGet)
	// 添加飞书回调路由
	r.HandleFunc("/FeishuAuth/callback", FeishuAuthCallback).Methods(http.MethodGet)
	// 添加钉钉回调路由
	r.HandleFunc("/DingtalkAuth/callback", DingtalkAuthCallback).Methods(http.MethodGet)
	// 企业微信验证路由（运行时动态匹配文件名，遍历启用的 wxwork 认证源）
	r.MatcherFunc(func(r *http.Request, rm *mux.RouteMatch) bool {
		_, ok := wxworkVerifyFileByPath(r.URL.Path)
		return ok
	}).HandlerFunc(SAMLTest).Methods(http.MethodGet)

	// Profile 路由（运行时动态匹配文件名）
	r.MatcherFunc(func(r *http.Request, rm *mux.RouteMatch) bool {
		return r.URL.Path == "/"+base.GetCfg().ProfileName+".xml"
	}).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := dbdata.GetProfileXML()
		if err != nil {
			base.Error(err)
			http.Error(w, "profile err", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Write(b)
	}).Methods(http.MethodGet)
	// 静态文件服务（运行时动态读取 FilesPath）
	r.PathPrefix("/files").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			notFound(w, r)
			return
		}
		rel := strings.TrimPrefix(r.URL.Path, "/files")
		if rel == "" {
			http.Redirect(w, r, "/files/", http.StatusMovedPermanently)
			return
		}
		name := filepath.Join(base.GetCfg().FilesPath, strings.TrimPrefix(r.URL.Path, "/files/"))
		if fi, err := os.Stat(name); err != nil || fi.IsDir() {
			if r.Method == http.MethodGet {
				admin.ServeIndex(w, r)
			} else {
				notFound(w, r)
			}
			return
		}
		http.StripPrefix("/files/",
			http.FileServer(http.Dir(base.GetCfg().FilesPath)),
		).ServeHTTP(w, r)
	})
	// 健康检测
	r.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	}).Methods(http.MethodGet)
	r.PathPrefix("/portal/").HandlerFunc(PortalHome)
	r.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			admin.ServeIndex(w, r)
			return
		}
		notFound(w, r)
	})
	return r
}

func notFound(w http.ResponseWriter, r *http.Request) {
	if base.GetLogLevel() == base.LogLevelTrace {
		hd, _ := httputil.DumpRequest(r, true)
		base.Trace("NotFound: ", r.RemoteAddr, string(hd))
	}

	w.WriteHeader(http.StatusNotFound)
	fmt.Fprintln(w, "404 page not found")
}
