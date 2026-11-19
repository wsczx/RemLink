package app

import (
	"context"
	"embed"
	"time"

	"github.com/wsczx/remlink/admin"
	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/auth/authsrv"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/cron"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/handler"
	"github.com/wsczx/remlink/pkg/notify"
	"github.com/wsczx/remlink/service"
	"github.com/wsczx/remlink/sessdata"
	"github.com/wsczx/remlink/webvpn"
)

type App struct {
	DB        *dbdata.DB
	UI        embed.FS
	cron      *cron.Cron
	syslogHub *handler.SyslogHub
	audit     *handler.AccessAudit
	servers   *handler.Servers

	// 认证/会话/链路域全部实例，由 Start 统一构造并注入 handler
	registry    *auth.ProviderRegistry
	sessions    *sessdata.SessionManager
	locks       *auth.LockManager
	webvpn      *webvpn.WebVPNManager
	authSession *handler.AuthSessionManager
	tap         *handler.TapLink
	dtlsLimiter *handler.DTLSHandshakeLimiter
	tunnel      *handler.Tunnel
	sso         *handler.SSOHandler
	webAuth     *handler.WebAuthService
	linkAuth    *handler.LinkAuthService
	portal      *handler.PortalService
	services    handler.Services
	stop        context.CancelFunc
}

func New(ui embed.FS) *App {
	return &App{UI: ui}
}

func (a *App) Init(ver, commit, buildDate string) {
	admin.SetUIData(a.UI)
	base.SetVersion(ver, commit, buildDate)
	a.registry = auth.NewProviderRegistry()
	auth.SetRegistry(a.registry)
	authsrv.RegisterProviders(a.registry)
	base.ValidateConfigMetas()
	base.Start()
}

func (a *App) OpenStore() error {
	if err := dbdata.Start(); err != nil {
		return err
	}
	a.DB = dbdata.GetDB()
	notify.SetLoader(dbdata.NotifyLoader{})
	return nil
}

func (a *App) SetDB(s *dbdata.DB) {
	dbdata.SetDB(s)
	a.DB = s
}

func (a *App) Start(ctx context.Context) error {
	if a.servers != nil {
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	rollback := func(err error) error {
		cancel()
		if a.cron != nil {
			a.cron.Stop()
			a.cron = nil
		}
		return err
	}

	a.syslogHub = handler.NewSyslogHub()
	a.sessions = sessdata.NewSessionManager()
	a.audit = handler.NewAccessAudit(a.sessions)
	sessdata.SetPayloads(sessdata.NewBufferPool())
	dbdata.SetBackups(dbdata.NewBackupManager())

	a.locks = auth.NewLockManager()
	a.webvpn = webvpn.NewManager()
	a.authSession = handler.NewAuthSessionManager()
	a.tap = handler.NewTapLink(a.sessions)
	a.dtlsLimiter = handler.NewDTLSHandshakeLimiter()
	a.tunnel = handler.NewTunnel(a.sessions, a.tap)
	a.sso = handler.NewSSOHandler(a.authSession)
	a.webAuth = handler.NewWebAuthService(a.authSession, a.locks, a.webvpn, a.sso)
	a.linkAuth = handler.NewLinkAuthService(a.sessions, a.authSession, a.locks, a.webAuth)
	a.portal = handler.NewPortalService(a.sessions, a.authSession, a.locks, a.webvpn, a.sso)
	a.sso.UsePortal(a.portal)

	a.services = handler.Services{
		SyslogHub:    a.syslogHub,
		AccessAudit:  a.audit,
		Sessions:     a.sessions,
		Locks:        a.locks,
		Webvpn:       a.webvpn,
		AuthSessions: a.authSession,
		Tunnel:       a.tunnel,
		Tap:          a.tap,
		DTLSLimiter:  a.dtlsLimiter,
		Portal:       a.portal,
		LinkAuth:     a.linkAuth,
		WebAuth:      a.webAuth,
		SSO:          a.sso,
	}

	service.SetActions(service.ProductionActions(a.sessions))

	c, err := cron.New()
	if err != nil {
		return rollback(err)
	}
	a.cron = c
	if err := c.Start(); err != nil {
		return rollback(err)
	}

	authsrv.StartAdminOTPCleaner(runCtx)
	authsrv.StartOtpCleaner(runCtx)
	authsrv.StartSmsCleaner(runCtx)

	servers, err := handler.Start(runCtx, a.services)
	if err != nil {
		return rollback(err)
	}
	a.servers = servers
	a.stop = cancel
	return nil
}

func (a *App) Run(ctx context.Context) error {
	if err := a.OpenStore(); err != nil {
		return err
	}
	if err := a.Start(ctx); err != nil {
		a.Stop()
		return err
	}
	<-ctx.Done()
	a.Stop()
	return nil
}

func (a *App) Stop() {
	if a.stop != nil {
		a.stop()
		a.stop = nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if a.servers != nil {
		a.servers.Shutdown(ctx)
		a.servers = nil
	}
	if a.cron != nil {
		a.cron.Stop()
		a.cron = nil
	}
	handler.Stop(a.services)
}
