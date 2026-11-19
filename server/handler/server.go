package handler

import (
	"crypto/tls"
	"errors"
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

func startTls(services Services) (*http.Server, error) {

	var (
		err error

		addr = base.GetCfg().ServerAddr
		ln   net.Listener
	)

	tlscert, _, err := dbdata.ParseCert()
	if err != nil {
		return nil, fmt.Errorf("证书加载失败: %w", err)
	}
	// 主证书设为 default 并登记 SHA1 指纹
	// IP 直连无 SNI，依赖 default 返回主证书（其 SAN 含本机 IP）；认证 XML 的 server-cert-hash 依赖该指纹
	dbdata.Certs.LoadCertificate(tlscert)
	// WebVPN 泛域名证书追加到 SAN 表，与主证书并存
	if wildCert, _, werr := dbdata.ParseCertWild(); werr == nil && wildCert != nil {
		dbdata.Certs.LoadCertificates([]*tls.Certificate{wildCert})
	}

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
			return dbdata.Certs.GetCertificateBySNI(chi.ServerName)
		},
	}
	// 客户端证书请求，支持热更新
	tlsConfig.GetConfigForClient = func(chi *tls.ClientHelloInfo) (*tls.Config, error) {
		if !dbdata.Groups.AnyGroupHasCertAuth() {
			return nil, nil // 无 cert 组不请求证书
		}
		c := tlsConfig.Clone()
		c.ClientAuth = tls.RequestClientCert          // 请求客户端证书
		c.ClientCAs = dbdata.Certs.LoadClientCAPool() // 加载客户端CA证书
		return c, nil
	}
	srv := &http.Server{
		Addr:         addr,
		Handler:      webVpnRootHandler(services),
		TLSConfig:    tlsConfig,
		ErrorLog:     base.GetServerLog(),
		ReadTimeout:  100 * time.Second,
		WriteTimeout: 100 * time.Second,
	}

	ln, err = net.Listen("tcp", base.FormatListenAddr(addr))
	if err != nil {
		return nil, fmt.Errorf("VPN 服务监听失败，请到管理后台「软件配置→服务监听→VPN 服务地址」修改端口后重启: %w", err)
	}

	if base.GetCfg().ProxyProtocol {
		ln = &proxyproto.Listener{
			Listener:          ln,
			ReadHeaderTimeout: 30 * time.Second,
		}
	}

	base.Info("listen server", addr)
	go func() {
		if err := srv.ServeTLS(ln, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			base.Error("VPN 服务运行异常:", err)
		}
	}()
	return srv, nil
}

// WebVPN 子域请求使用独立处理链，其他请求交给常规路由
func webVpnRootHandler(services Services) http.Handler {
	root := initRoute(services)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if WebVpnHandler(services, w, r) {
			return
		}
		root.ServeHTTP(w, r)
	})
}

func initRoute(services Services) http.Handler {
	r := mux.NewRouter()
	// 所有路由添加安全头
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			utils.SetSecureHeader(w)
			next.ServeHTTP(w, req)
		})
	})

	r.HandleFunc("/", LinkHome).Methods(http.MethodGet)
	r.HandleFunc("/", services.LinkAuth.Handle).Methods(http.MethodPost)
	r.HandleFunc("/portal", services.Portal.Home).Methods(http.MethodGet)
	r.HandleFunc("/portal/", services.Portal.Home).Methods(http.MethodGet)
	// 子域名登录可放行的门户接口：与 WebVpnHandler 的放行判断共用 portalLoginEndpoints 单一来源，
	// 新增子域名登录必需的门户接口只需改 portalLoginEndpoints（webvpn.go）
	for _, ep := range portalLoginEndpoints(services.Portal) {
		r.HandleFunc(ep.path, ep.handler).Methods(ep.method)
	}
	r.HandleFunc("/portal/api/my_groups", services.Portal.MyGroups).Methods(http.MethodGet)
	r.HandleFunc("/portal/api/change_password", services.Portal.ChangePassword).Methods(http.MethodPost)
	r.HandleFunc("/portal/api/force_change_password", services.Portal.ForceChangePassword).Methods(http.MethodPost)
	r.HandleFunc("/portal/api/logout", services.Portal.Logout).Methods(http.MethodPost)
	r.HandleFunc("/portal/api/otp/regenerate", services.Portal.RegenerateOTP).Methods(http.MethodPost)
	r.HandleFunc("/portal/api/forgot_password", services.Portal.ForgotPassword).Methods(http.MethodPost)
	r.HandleFunc("/portal/api/reset_password/verify", services.Portal.VerifyResetPassword).Methods(http.MethodGet)
	r.HandleFunc("/portal/api/reset_password", services.Portal.ResetPassword).Methods(http.MethodPost)
	r.HandleFunc("/portal/api/certs", services.Portal.CertList).Methods(http.MethodGet)
	r.HandleFunc("/portal/api/certs/download", services.Portal.DownloadCert).Methods(http.MethodPost)
	r.HandleFunc("/portal/api/devices", services.Portal.Devices).Methods(http.MethodGet)
	r.HandleFunc("/portal/api/devices/offline", services.Portal.DeviceOffline).Methods(http.MethodPost)
	// WebVPN 用户侧端点（独立会话：webvpn_session）
	r.HandleFunc("/webvpn/login-config", webVpnLoginConfig).Methods(http.MethodGet)
	r.HandleFunc("/webvpn/me", func(w http.ResponseWriter, r *http.Request) { webVpnMe(services.Webvpn, w, r) }).Methods(http.MethodGet)
	r.HandleFunc("/webvpn/my-apps", func(w http.ResponseWriter, r *http.Request) { webVpnMyApps(services.Webvpn, w, r) }).Methods(http.MethodGet)
	r.HandleFunc("/webvpn/logout", func(w http.ResponseWriter, r *http.Request) { webVpnLogout(services.Webvpn, w, r) }).Methods(http.MethodGet, http.MethodPost)
	// WebAuth 端点
	r.HandleFunc("/web-auth/start", services.WebAuth.Start).Methods(http.MethodGet)
	r.HandleFunc("/web-auth/identify", services.WebAuth.Identify).Methods(http.MethodPost)
	r.HandleFunc("/web-auth/groups", services.WebAuth.SelectGroup).Methods(http.MethodPost)
	r.HandleFunc("/web-auth/step", services.WebAuth.Step).Methods(http.MethodPost)
	r.HandleFunc("/web-auth/sms/resend", services.WebAuth.ResendSMS).Methods(http.MethodPost)
	r.HandleFunc("/web-auth/continue", services.WebAuth.Continue).Methods(http.MethodPost)
	r.HandleFunc("/web-auth/sso-callback", services.WebAuth.SSOCallback).Methods(http.MethodGet)
	r.HandleFunc("/web-auth/complete", services.WebAuth.Complete).Methods(http.MethodGet)
	r.HandleFunc("/web-auth/change_password", services.WebAuth.ChangePassword).Methods(http.MethodPost)
	r.HandleFunc("/+CSCOE+/web-auth/sp/login", services.WebAuth.SPLogin).Methods(http.MethodGet)
	r.PathPrefix("/ui/").Handler(admin.ServeUI())
	r.HandleFunc("/CSCOSSLC/tunnel", services.Tunnel.OpenCSTP).Methods(http.MethodConnect)
	r.HandleFunc("/otp_qr", services.LinkAuth.OTPQR).Methods(http.MethodGet)
	r.HandleFunc("/otp-verification", services.LinkAuth.LinkAuthOTP).Methods(http.MethodPost)
	// 添加Cisco AnyConnect兼容的SAML端点
	r.HandleFunc("/+CSCOE+/saml/sp/login", services.SSO.SPLogin).Methods(http.MethodGet)
	r.HandleFunc("/+CSCOE+/saml_ac_login.html", services.SSO.ACLogin).Methods(http.MethodGet)
	r.HandleFunc("/+CSCOE+/saml/sp/done", services.SSO.SAMLDone)
	r.HandleFunc("/+CSCOE+/force-pwd", services.LinkAuth.ForcePasswordPage).Methods(http.MethodGet)
	r.HandleFunc("/+CSCOE+/force-pwd/submit", services.LinkAuth.ForcePasswordSubmit).Methods(http.MethodPost)
	// 添加企业微信回调路由
	r.HandleFunc("/WXAuth/callback", services.SSO.WXCallback).Methods(http.MethodGet)
	// 添加飞书回调路由
	r.HandleFunc("/FeishuAuth/callback", services.SSO.FeishuCallback).Methods(http.MethodGet)
	// 添加钉钉回调路由
	r.HandleFunc("/DingtalkAuth/callback", services.SSO.DingtalkCallback).Methods(http.MethodGet)
	// 企业微信验证路由（运行时动态匹配文件名，遍历启用的 wxwork 认证源）
	r.MatcherFunc(func(r *http.Request, rm *mux.RouteMatch) bool {
		_, ok := wxworkVerifyFileByPath(r.URL.Path)
		return ok
	}).HandlerFunc(services.SSO.SAMLTest).Methods(http.MethodGet)

	// Profile 路由（运行时动态匹配文件名）
	r.MatcherFunc(func(r *http.Request, rm *mux.RouteMatch) bool {
		return r.URL.Path == "/"+base.GetCfg().ProfileName+".xml"
	}).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := dbdata.Settings.GetProfileXML()
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
		// 防路径穿越
		baseDir := filepath.Clean(base.GetCfg().FilesPath)
		name := filepath.Join(baseDir, strings.TrimPrefix(r.URL.Path, "/files/"))
		if !strings.HasPrefix(name, baseDir+string(os.PathSeparator)) {
			if r.Method == http.MethodGet {
				admin.ServeIndex(w, r)
			} else {
				notFound(w, r)
			}
			return
		}
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
	r.PathPrefix("/portal/").HandlerFunc(services.Portal.Home)
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
		base.Trace("NotFound: ", r.RemoteAddr, utils.RedactSensitive(string(hd)))
	}

	w.WriteHeader(http.StatusNotFound)
	fmt.Fprintln(w, "404 page not found")
}
