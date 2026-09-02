// admin:后台管理接口
package admin

import (
	"crypto/tls"
	"embed"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/arl/statsviz"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	_ "github.com/wsczx/remlink/auth/authsrv" // 触发认证器 init() 注册
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
)

var UiData embed.FS

// 开启服务
func StartAdmin() {
	r := mux.NewRouter()
	r.Use(recoverHttp, authMiddleware, handlers.CompressHandler)
	// 所有路由添加安全头
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			utils.SetSecureHeader(w)
			w.Header().Set("Server", "RemLinkAdminOpenSource")
			next.ServeHTTP(w, req)
		})
	})

	// 监控检测
	r.HandleFunc("/status.html", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}).Name("index")

	r.Handle("/", http.RedirectHandler("/ui/", http.StatusFound)).Name("index")
	r.PathPrefix("/ui/").Handler(ServeUI()).Name("static")
	r.HandleFunc("/base/login", Login).Name("login")
	r.HandleFunc("/base/login/otp", LoginOTP).Name("login_otp")
	r.HandleFunc("/base/auth_check", AuthCheck).Name("auth_check")
	r.HandleFunc("/base/logout", Logout).Name("logout")
	// 品牌配置免鉴权：供管理后台侧边栏与登录页在 8800 端口直接加载
	r.HandleFunc("/portal/api/login-config", AdminPortalLoginConfig).Methods(http.MethodGet).Name("login_config")
	r.HandleFunc("/set/change_password", ChangeAdminPassword).Methods(http.MethodPost)
	r.HandleFunc("/set/otp_qr", AdminOtpQr).Methods(http.MethodGet, http.MethodPost)
	r.HandleFunc("/set/otp_generate", AdminOtpGenerate).Methods(http.MethodPost)
	r.HandleFunc("/set/otp_confirm", AdminOtpConfirm).Methods(http.MethodPost)
	r.HandleFunc("/set/otp_disable", AdminOtpDisable).Methods(http.MethodPost)

	r.HandleFunc("/set/home", SetHome)
	r.HandleFunc("/set/system", SetSystem)
	r.HandleFunc("/set/soft", SetSoft)
	r.HandleFunc("/set/soft/status", SetSoftStatus)
	r.HandleFunc("/set/soft/edit", SetSoftEdit).Methods(http.MethodPost)
	r.HandleFunc("/set/soft/ipv4", SetIPv4Config).Methods(http.MethodPost)
	r.HandleFunc("/set/profile", SetProfile)
	r.HandleFunc("/set/profile/edit", SetProfileEdit).Methods(http.MethodPost)
	r.HandleFunc("/set/restart", SetRestart).Methods(http.MethodPost)
	r.HandleFunc("/set/upgrade/check", CheckUpgrade).Methods(http.MethodGet)
	r.HandleFunc("/set/upgrade/start", StartUpgrade).Methods(http.MethodPost)
	r.HandleFunc("/set/upgrade/status", UpgradeStatusHandler).Methods(http.MethodGet)
	r.HandleFunc("/set/db/table_sizes", SetDbTableSizes)
	r.HandleFunc("/set/db/backup", SetDbBackup).Methods(http.MethodPost)
	r.HandleFunc("/set/db/backups", SetDbBackups)
	r.HandleFunc("/set/db/restore", SetDbRestore).Methods(http.MethodPost)
	r.HandleFunc("/set/db/backup/delete", SetDbBackupDelete).Methods(http.MethodPost)
	r.HandleFunc("/set/db/test_connection", SetDbTestConnection)
	r.HandleFunc("/set/db/switch", SetDbSwitch).Methods(http.MethodPost)
	r.HandleFunc("/set/other", SetOther)
	r.HandleFunc("/set/other/edit", SetOtherEdit).Methods(http.MethodPost)
	r.HandleFunc("/set/other/smtp", SetOtherSmtp)
	r.HandleFunc("/set/other/smtp/edit", SetOtherSmtpEdit).Methods(http.MethodPost)
	r.HandleFunc("/set/other/sms", SetOtherSms)
	r.HandleFunc("/set/other/sms/edit", SetOtherSmsEdit).Methods(http.MethodPost)
	r.HandleFunc("/set/other/sms/test", SetOtherSmsTest).Methods(http.MethodPost)
	r.HandleFunc("/set/other/audit_log", SetOtherAuditLog)
	r.HandleFunc("/set/other/audit_log/edit", SetOtherAuditLogEdit).Methods(http.MethodPost)
	r.HandleFunc("/set/portal_brand", SetPortalBrand)
	r.HandleFunc("/set/portal_brand/edit", SetPortalBrandEdit).Methods(http.MethodPost)
	r.HandleFunc("/set/portal_dashboard", SetPortalDashboard)

	// WebVPN 应用管理
	r.HandleFunc("/webvpn/domain", WebVpnDomain)
	r.HandleFunc("/webvpn/app/list", WebVpnAppList)
	r.HandleFunc("/webvpn/app/detail", WebVpnAppDetail)
	r.HandleFunc("/webvpn/app/set", WebVpnAppSet).Methods(http.MethodPost)
	r.HandleFunc("/webvpn/app/del", WebVpnAppDel).Methods(http.MethodPost)
	r.HandleFunc("/webvpn/audit/list", WebVpnAuditList)
	r.HandleFunc("/webvpn/audit/export", WebVpnAuditExport)
	r.HandleFunc("/webvpn/session/kick", WebVpnSessionKick).Methods(http.MethodPost)
	r.HandleFunc("/set/portal_dashboard/edit", SetPortalDashboardEdit).Methods(http.MethodPost)
	r.HandleFunc("/set/audit/list", SetAuditList)
	r.HandleFunc("/set/audit/export", SetAuditExport)
	r.HandleFunc("/set/audit/act_log_list", UserActLogList)
	r.HandleFunc("/set/audit/admin_op_log_list", AdminOpLogList)
	r.HandleFunc("/set/syslog/ws", SyslogWS) // WebSocket 系统日志实时推送
	r.HandleFunc("/set/syslog/history_enabled", SyslogHistoryEnabled)
	r.HandleFunc("/set/syslog/history_dates", SyslogHistoryDates)
	r.HandleFunc("/set/syslog/history_list", SyslogHistoryList)
	r.HandleFunc("/set/other/createcert", CreatCert).Methods(http.MethodPost)
	r.HandleFunc("/set/other/getcertset", GetCertSetting)
	r.HandleFunc("/set/other/customcert", CustomCert).Methods(http.MethodPost)
	r.HandleFunc("/set/client_cert/ca_status", CheckCAStatus)
	r.HandleFunc("/set/client_cert/init_ca", InitClientCA).Methods(http.MethodPost)
	r.HandleFunc("/set/client_cert/generate", GenerateClientCert).Methods(http.MethodPost)
	r.HandleFunc("/set/client_cert/batch_generate", BatchGenerateClientCert).Methods(http.MethodPost)
	r.HandleFunc("/set/client_cert/renew", RenewClientCert).Methods(http.MethodPost)
	r.HandleFunc("/set/client_cert/download", DownloadClientP12)
	r.HandleFunc("/set/client_cert/list", GetClientCertList)
	r.HandleFunc("/set/client_cert/changecertstatus", ChangeClientCertStatus).Methods(http.MethodPost)
	r.HandleFunc("/set/client_cert/delete", DeleteClientCert).Methods(http.MethodPost)
	r.HandleFunc("/set/client_cert/batch_delete", BatchDeleteClientCert).Methods(http.MethodPost)
	r.HandleFunc("/set/client_cert/batch_download", BatchDownloadClientP12)
	r.HandleFunc("/set/client_cert/cert_mail_template", GetCertMailTemplate)
	r.HandleFunc("/set/client_cert/user_cert_info", UserCertInfo)
	r.HandleFunc("/set/client_cert/unbind_device", UnbindDevice).Methods(http.MethodPost)
	r.HandleFunc("/set/client_cert/update_max_devices", UpdateClientCertMaxDevices).Methods(http.MethodPost)
	r.HandleFunc("/set/client_cert/update_device_binding", UpdateClientCertDeviceBinding).Methods(http.MethodPost)
	r.HandleFunc("/set/client_cert/send_mail", SendClientCertMail).Methods(http.MethodPost)

	r.HandleFunc("/user/list", UserList)
	r.HandleFunc("/user/detail", UserDetail)
	r.HandleFunc("/user/set", UserSet).Methods(http.MethodPost)
	r.HandleFunc("/user/uploaduser", UserUpload).Methods(http.MethodPost)
	r.HandleFunc("/user/uploaduser_template", UserUploadTemplate)
	r.HandleFunc("/user/del", UserDel).Methods(http.MethodPost)
	r.HandleFunc("/user/online", UserOnline)
	r.HandleFunc("/user/offline", UserOffline).Methods(http.MethodPost)
	r.HandleFunc("/user/reline", UserReline).Methods(http.MethodPost)
	r.HandleFunc("/user/reset_traffic", UserResetTraffic).Methods(http.MethodPost)
	r.HandleFunc("/user/otp_qr", UserOtpQr)
	r.HandleFunc("/user/ip_map/list", UserIpMapList)
	r.HandleFunc("/user/ip_map/detail", UserIpMapDetail)
	r.HandleFunc("/user/ip_map/set", UserIpMapSet).Methods(http.MethodPost)
	r.HandleFunc("/user/ip_map/del", UserIpMapDel).Methods(http.MethodPost)
	r.HandleFunc("/user/ip_map/batch_del", UserIpMapBatchDel).Methods(http.MethodPost)
	r.HandleFunc("/user/batch/send_email", UserBatchSendEmail).Methods(http.MethodPost)
	r.HandleFunc("/user/batch/delete", UserBatchDelete).Methods(http.MethodPost)

	r.HandleFunc("/group/list", GroupList)
	r.HandleFunc("/group/names", GroupNames)
	r.HandleFunc("/group/names_ids", GroupNamesIds)
	r.HandleFunc("/group/detail", GroupDetail)
	r.HandleFunc("/group/ifaces", GroupIfaces)
	r.HandleFunc("/group/set", GroupSet).Methods(http.MethodPost)
	r.HandleFunc("/group/del", GroupDel).Methods(http.MethodPost)
	r.HandleFunc("/group/auth_login", GroupAuthLogin)
	r.HandleFunc("/group/cert_check", GroupCertCheck)
	r.HandleFunc("/group/cert_auth_check", GroupCertAuthCheck)

	// 策略管理
	r.HandleFunc("/policy/list", PolicyList)
	r.HandleFunc("/policy/names", PolicyNames)
	r.HandleFunc("/policy/all_names", AllPolicyNames)
	r.HandleFunc("/policy/detail", PolicyDetail)
	r.HandleFunc("/policy/set", PolicySet).Methods(http.MethodPost)
	r.HandleFunc("/policy/del", PolicyDel).Methods(http.MethodPost)
	r.HandleFunc("/policy/copy", PolicyCopy).Methods(http.MethodPost)
	r.HandleFunc("/policy/used_by", PolicyUsedBy)
	r.HandleFunc("/policy/apply_to_groups", PolicyApplyToGroups).Methods(http.MethodPost)
	r.HandleFunc("/policy/apply_to_users", PolicyApplyToUsers).Methods(http.MethodPost)
	r.HandleFunc("/policy/remove_from_groups", PolicyRemoveFromGroups).Methods(http.MethodPost)
	r.HandleFunc("/policy/remove_from_users", PolicyRemoveFromUsers).Methods(http.MethodPost)

	r.HandleFunc("/provider/list", ProviderList)
	r.HandleFunc("/provider/names", ProviderNames)
	r.HandleFunc("/provider/detail", ProviderDetail)
	r.HandleFunc("/provider/set", ProviderSet).Methods(http.MethodPost)
	r.HandleFunc("/provider/del", ProviderDel).Methods(http.MethodPost)
	r.HandleFunc("/provider/test_login", ProviderAuthLogin).Methods(http.MethodPost)
	r.HandleFunc("/provider/sync_users", ProviderSyncUsers).Methods(http.MethodPost)

	r.HandleFunc("/set/secret/status", SecretStatus)
	r.HandleFunc("/set/secret/enable", SecretEnable).Methods(http.MethodPost)
	r.HandleFunc("/set/secret/upload", SecretUpload).Methods(http.MethodPost)
	r.HandleFunc("/set/secret/disable", SecretDisable).Methods(http.MethodPost)

	r.HandleFunc("/statsinfo/list", StatsInfoList)
	r.HandleFunc("/locksinfo/list", GetLocksInfo)
	r.HandleFunc("/locksinfo/unlok", UnlockUser).Methods(http.MethodPost)

	// pprof - 始终注册路由，运行时根据配置动态开关
	pprofGuard := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !base.GetCfg().Pprof {
				http.Error(w, "pprof is disabled", http.StatusForbidden)
				return
			}
			next(w, r)
		}
	}
	r.HandleFunc("/debug/pprof/cmdline", pprofGuard(pprof.Cmdline)).Name("debug")
	r.HandleFunc("/debug/pprof/profile", pprofGuard(pprof.Profile)).Name("debug")
	r.HandleFunc("/debug/pprof/symbol", pprofGuard(pprof.Symbol)).Name("debug")
	r.HandleFunc("/debug/pprof/trace", pprofGuard(pprof.Trace)).Name("debug")
	r.HandleFunc("/debug/pprof", pprofGuard(location("/debug/pprof/"))).Name("debug")
	r.PathPrefix("/debug/pprof/").HandlerFunc(pprofGuard(pprof.Index)).Name("debug")
	// statsviz
	vizSrv, _ := statsviz.NewServer()
	r.Path("/debug/statsviz/ws").Name("debug").HandlerFunc(pprofGuard(vizSrv.Ws()))
	r.PathPrefix("/debug/statsviz/").Name("debug").Handler(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !base.GetCfg().Pprof {
				http.Error(w, "statsviz is disabled", http.StatusForbidden)
				return
			}
			vizSrv.Index().ServeHTTP(w, r)
		}),
	)
	r.NotFoundHandler = NotFound()

	base.Info("Listen admin", base.GetCfg().AdminAddr)

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
	srv := &http.Server{
		Addr:              base.FormatListenAddr(base.GetCfg().AdminAddr),
		Handler:           r,
		TLSConfig:         tlsConfig,
		ErrorLog:          base.GetServerLog(),
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
	}
	err := srv.ListenAndServeTLS("", "")
	if err != nil {
		base.Fatal(err)
	}
}

func location(url string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", url)
		w.WriteHeader(http.StatusFound)
	}
}
