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
	r.HandleFunc("/api/v1/base/login", Login).Name("login")
	r.HandleFunc("/api/v1/base/login/otp", LoginOTP).Name("login_otp")
	r.HandleFunc("/api/v1/base/auth_check", AuthCheck).Name("auth_check")
	r.HandleFunc("/api/v1/base/logout", Logout).Name("logout")
	// 品牌配置免鉴权：供管理后台侧边栏与登录页在 8800 端口直接加载
	r.HandleFunc("/api/v1/portal/login-config", AdminPortalLoginConfig).Methods(http.MethodGet).Name("login_config")
	r.HandleFunc("/api/v1/set/change_password", ChangeAdminPassword).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/otp_qr", AdminOtpQr).Methods(http.MethodGet, http.MethodPost)
	r.HandleFunc("/api/v1/set/otp_generate", AdminOtpGenerate).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/otp_confirm", AdminOtpConfirm).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/otp_disable", AdminOtpDisable).Methods(http.MethodPost)

	r.HandleFunc("/api/v1/set/home", SetHome)
	r.HandleFunc("/api/v1/set/system", SetSystem)
	r.HandleFunc("/api/v1/set/soft", SetSoft)
	r.HandleFunc("/api/v1/set/soft/status", SetSoftStatus)
	r.HandleFunc("/api/v1/set/soft/edit", SetSoftEdit).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/soft/ipv4", SetIPv4Config).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/profile", SetProfile)
	r.HandleFunc("/api/v1/set/profile/edit", SetProfileEdit).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/restart", SetRestart).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/upgrade/check", CheckUpgrade).Methods(http.MethodGet)
	r.HandleFunc("/api/v1/set/upgrade/start", StartUpgrade).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/upgrade/status", UpgradeStatusHandler).Methods(http.MethodGet)
	r.HandleFunc("/api/v1/set/db/table_sizes", SetDbTableSizes)
	r.HandleFunc("/api/v1/set/db/backup", SetDbBackup).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/db/backups", SetDbBackups)
	r.HandleFunc("/api/v1/set/db/restore", SetDbRestore).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/db/backup/delete", SetDbBackupDelete).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/db/test_connection", SetDbTestConnection)
	r.HandleFunc("/api/v1/set/db/switch", SetDbSwitch).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/other", SetOther)
	r.HandleFunc("/api/v1/set/other/edit", SetOtherEdit).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/other/smtp", SetOtherSmtp)
	r.HandleFunc("/api/v1/set/other/smtp/edit", SetOtherSmtpEdit).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/other/sms", SetOtherSms)
	r.HandleFunc("/api/v1/set/other/sms/edit", SetOtherSmsEdit).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/other/sms/test", SetOtherSmsTest).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/other/audit_log", SetOtherAuditLog)
	r.HandleFunc("/api/v1/set/other/audit_log/edit", SetOtherAuditLogEdit).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/portal_brand", SetPortalBrand)
	r.HandleFunc("/api/v1/set/portal_brand/edit", SetPortalBrandEdit).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/portal_dashboard", SetPortalDashboard)

	// WebVPN 应用管理
	r.HandleFunc("/api/v1/webvpn/domain", WebVpnDomain)
	r.HandleFunc("/api/v1/webvpn/app/list", WebVpnAppList)
	r.HandleFunc("/api/v1/webvpn/app/detail", WebVpnAppDetail)
	r.HandleFunc("/api/v1/webvpn/app/set", WebVpnAppSet).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/webvpn/app/del", WebVpnAppDel).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/webvpn/audit/list", WebVpnAuditList)
	r.HandleFunc("/api/v1/webvpn/audit/export", WebVpnAuditExport)
	r.HandleFunc("/api/v1/webvpn/session/kick", WebVpnSessionKick).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/webvpn/remote/online", WebVpnLiveSessionList).Methods(http.MethodGet)
	r.HandleFunc("/api/v1/webvpn/remote/kick", WebVpnLiveSessionKick).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/webvpn/remote/sessions", WebVpnSessionHistoryList).Methods(http.MethodGet)
	r.HandleFunc("/api/v1/webvpn/remote/file-logs", WebVpnFileLogList).Methods(http.MethodGet)
	r.HandleFunc("/api/v1/webvpn/remote/recording", WebVpnRecordingDownload).Methods(http.MethodGet)
	r.HandleFunc("/api/v1/webvpn/visits", WebVpnVisitList).Methods(http.MethodGet)
	r.HandleFunc("/api/v1/webvpn/visits/summary", WebVpnVisitSummary).Methods(http.MethodGet)
	r.HandleFunc("/api/v1/webvpn/visits/detail", WebVpnVisitDetail).Methods(http.MethodGet)
	r.HandleFunc("/api/v1/webvpn/visits/export", WebVpnVisitExport).Methods(http.MethodGet)
	r.HandleFunc("/api/v1/set/portal_dashboard/edit", SetPortalDashboardEdit).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/audit/list", SetAuditList)
	r.HandleFunc("/api/v1/set/audit/export", SetAuditExport)
	r.HandleFunc("/api/v1/set/audit/act_log_list", UserActLogList)
	r.HandleFunc("/api/v1/set/audit/access_flow", AccessFlowList)
	r.HandleFunc("/api/v1/set/audit/admin_op_log_list", AdminOpLogList)
	r.HandleFunc("/api/v1/set/audit/daily", SetAuditDaily)
	r.HandleFunc("/api/v1/set/audit/act_log_daily", UserActLogDaily)
	r.HandleFunc("/api/v1/set/audit/admin_op_log_daily", AdminOpLogDaily)
	r.HandleFunc("/api/v1/set/syslog/ws", SyslogWS) // WebSocket 系统日志实时推送
	r.HandleFunc("/api/v1/set/syslog/history_enabled", SyslogHistoryEnabled)
	r.HandleFunc("/api/v1/set/syslog/history_dates", SyslogHistoryDates)
	r.HandleFunc("/api/v1/set/syslog/history_list", SyslogHistoryList)
	r.HandleFunc("/api/v1/set/other/createcert", CreatCert).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/other/getcertset", GetCertSetting)
	r.HandleFunc("/api/v1/set/cert/info", CertInfo).Methods(http.MethodGet)
	r.HandleFunc("/api/v1/set/geoip/status", GeoipStatus).Methods(http.MethodGet)
	r.HandleFunc("/api/v1/set/geoip/upload", GeoipUpload).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/other/customcert", CustomCert).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/client_cert/ca_status", CheckCAStatus)
	r.HandleFunc("/api/v1/set/client_cert/init_ca", InitClientCA).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/client_cert/generate", GenerateClientCert).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/client_cert/batch_generate", BatchGenerateClientCert).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/client_cert/renew", RenewClientCert).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/client_cert/download", DownloadClientP12)
	r.HandleFunc("/api/v1/set/client_cert/list", GetClientCertList)
	r.HandleFunc("/api/v1/set/client_cert/changecertstatus", ChangeClientCertStatus).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/client_cert/delete", DeleteClientCert).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/client_cert/batch_delete", BatchDeleteClientCert).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/client_cert/batch_download", BatchDownloadClientP12)
	r.HandleFunc("/api/v1/set/client_cert/cert_mail_template", GetCertMailTemplate)
	r.HandleFunc("/api/v1/set/client_cert/user_cert_info", UserCertInfo)
	r.HandleFunc("/api/v1/set/client_cert/unbind_device", UnbindDevice).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/client_cert/update_max_devices", UpdateClientCertMaxDevices).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/client_cert/update_device_binding", UpdateClientCertDeviceBinding).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/client_cert/send_mail", SendClientCertMail).Methods(http.MethodPost)

	r.HandleFunc("/api/v1/user/list", UserList)
	r.HandleFunc("/api/v1/user/detail", UserDetail)
	r.HandleFunc("/api/v1/user/set", UserSet).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/user/uploaduser", UserUpload).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/user/uploaduser_template", UserUploadTemplate)
	r.HandleFunc("/api/v1/user/del", UserDel).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/user/online", UserOnline)
	r.HandleFunc("/api/v1/user/offline", UserOffline).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/user/reline", UserReline).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/user/reset_traffic", UserResetTraffic).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/user/otp_qr", UserOtpQr)
	r.HandleFunc("/api/v1/user/ip_map/list", UserIpMapList)
	r.HandleFunc("/api/v1/user/ip_map/detail", UserIpMapDetail)
	r.HandleFunc("/api/v1/user/ip_map/set", UserIpMapSet).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/user/ip_map/del", UserIpMapDel).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/user/ip_map/batch_del", UserIpMapBatchDel).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/user/batch/send_email", UserBatchSendEmail).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/user/batch/delete", UserBatchDelete).Methods(http.MethodPost)

	r.HandleFunc("/api/v1/group/list", GroupList)
	r.HandleFunc("/api/v1/group/names", GroupNames)
	r.HandleFunc("/api/v1/group/names_ids", GroupNamesIds)
	r.HandleFunc("/api/v1/group/detail", GroupDetail)
	r.HandleFunc("/api/v1/group/ifaces", GroupIfaces)
	r.HandleFunc("/api/v1/group/set", GroupSet).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/group/del", GroupDel).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/group/auth_login", GroupAuthLogin)
	r.HandleFunc("/api/v1/group/cert_check", GroupCertCheck)
	r.HandleFunc("/api/v1/group/cert_auth_check", GroupCertAuthCheck)

	// 策略管理
	r.HandleFunc("/api/v1/policy/list", PolicyList)
	r.HandleFunc("/api/v1/policy/names", PolicyNames)
	r.HandleFunc("/api/v1/policy/all_names", AllPolicyNames)
	r.HandleFunc("/api/v1/policy/detail", PolicyDetail)
	r.HandleFunc("/api/v1/policy/set", PolicySet).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/policy/del", PolicyDel).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/policy/copy", PolicyCopy).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/policy/used_by", PolicyUsedBy)
	r.HandleFunc("/api/v1/policy/apply_to_groups", PolicyApplyToGroups).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/policy/apply_to_users", PolicyApplyToUsers).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/policy/remove_from_groups", PolicyRemoveFromGroups).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/policy/remove_from_users", PolicyRemoveFromUsers).Methods(http.MethodPost)

	r.HandleFunc("/api/v1/provider/list", ProviderList)
	r.HandleFunc("/api/v1/provider/names", ProviderNames)
	r.HandleFunc("/api/v1/provider/detail", ProviderDetail)
	r.HandleFunc("/api/v1/provider/set", ProviderSet).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/provider/del", ProviderDel).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/provider/test_login", ProviderAuthLogin).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/provider/sync_users", ProviderSyncUsers).Methods(http.MethodPost)

	r.HandleFunc("/api/v1/set/secret/status", SecretStatus)
	r.HandleFunc("/api/v1/set/secret/enable", SecretEnable).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/secret/upload", SecretUpload).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/set/secret/disable", SecretDisable).Methods(http.MethodPost)

	r.HandleFunc("/api/v1/statsinfo/list", StatsInfoList)
	r.HandleFunc("/api/v1/statsinfo/geo", StatsGeo)
	r.HandleFunc("/api/v1/locksinfo/list", GetLocksInfo)
	r.HandleFunc("/api/v1/locksinfo/unlok", UnlockUser).Methods(http.MethodPost)

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
