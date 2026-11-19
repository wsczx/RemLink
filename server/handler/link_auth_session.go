// 认证会话管理：存储、CRUD、Cookie 操作

package handler

import (
	"crypto/md5"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/wsczx/remlink/sessdata"
)

// 认证会话来源标记：确保 SSO token 只能兑换 SSO 流程产生的会话
// 杜绝“等待第二因素”的认证管道会话被当作 SSO 会话重放冒用
const (
	AuthKindPipeline   = "pipeline"     // 原生客户端/门户认证管道会话（含 OTP/RADIUS/SMS 等待断点）
	AuthKindSSOPending = "sso-pending"  // SSO OAuth 已发起、等待回调
	AuthKindSSODone    = "sso-done"     // SSO 回调已完成（completeSSO）
	AuthKindWebAuth    = "webauth-done" // WebAuth 门户管道已全部通过
)

// 临时认证会话存储
// 步骤状态、用户信息、连接信息、Resume 断点均保存在 Ctx 内
type AuthSession struct {
	SessionID  string             // 认证会话ID
	Ctx        *auth.Context      // 认证上下文
	UserActLog *dbdata.UserActLog // 审计日志
	ForcePwd   bool               // 强制改密会话
	CreatedAt  time.Time
	Kind       string // 会话来源：pipeline / sso-pending / sso-done / webauth-done
}

// 会话管理器（单例）
type AuthSessionManager struct {
	mu              sync.Mutex
	sessions        map[string]*AuthSession
	ttl             time.Duration
	cleanupInterval time.Duration
	stopCh          chan struct{}
	wg              sync.WaitGroup
	started         bool // 由 mu 保护，标记清理协程是否在运行
}

// TTL 5 分钟、清理周期 1 分钟
func NewAuthSessionManager() *AuthSessionManager {
	return &AuthSessionManager{
		sessions:        make(map[string]*AuthSession),
		ttl:             5 * time.Minute,
		cleanupInterval: 1 * time.Minute,
	}
}

// 启动后台过期清理协程。可安全重复调用，运行中重复调用无效
func (m *AuthSessionManager) Start() {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	m.stopCh = make(chan struct{})
	stopCh := m.stopCh
	m.wg.Add(1)
	m.mu.Unlock()

	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(m.cleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.cleanExpired()
			case <-stopCh:
				return
			}
		}
	}()
}

// 停止后台清理协程并等待其退出。可安全重复调用，未启动时为空操作
func (m *AuthSessionManager) Stop() {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return
	}
	m.started = false
	close(m.stopCh)
	m.mu.Unlock()

	m.wg.Wait()
}

// 保存会话，自动记录创建时间与会话ID
func (m *AuthSessionManager) Save(id string, data *AuthSession) {
	data.CreatedAt = time.Now()
	data.SessionID = id
	m.mu.Lock()
	m.sessions[id] = data
	m.mu.Unlock()
}

// 获取认证会话并做 TTL 过期校验
func (m *AuthSessionManager) Get(id string) (*AuthSession, error) {
	m.mu.Lock()
	s, ok := m.sessions[id]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("会话未找到")
	}
	if time.Since(s.CreatedAt) > m.ttl {
		m.Delete(id)
		return nil, fmt.Errorf("会话过期")
	}
	s.SessionID = id
	return s, nil
}

// 删除会话
func (m *AuthSessionManager) Delete(id string) {
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
}

// 清理超过 TTL 的会话
func (m *AuthSessionManager) cleanExpired() {
	m.mu.Lock()
	now := time.Now()
	for id, s := range m.sessions {
		if now.Sub(s.CreatedAt) > m.ttl {
			delete(m.sessions, id)
		}
	}
	m.mu.Unlock()
}

func (s *LinkAuthService) CreateSession(w http.ResponseWriter, authSession *AuthSession) {
	ua := authSession.UserActLog
	ctx := authSession.Ctx

	token := utils.RandomHex(32)
	uniqueMac := true
	macHw, err := net.ParseMAC(ctx.Conn.MacAddr)
	if err != nil {
		var sum [16]byte
		if ctx.Conn.DeviceID != "" {
			sum = md5.Sum([]byte(ctx.Conn.DeviceID))
		} else {
			sum = md5.Sum([]byte(token))
			uniqueMac = false
		}
		macHw = sum[0:5]
		macHw = append([]byte{0x02}, macHw...)
	}

	sess := &sessdata.Session{
		Token:           token,
		Sid:             fmt.Sprintf("%d", time.Now().Unix()),
		Username:        ctx.Conn.Username,
		Group:           ctx.Conn.GroupName,
		UniqueIdGlobal:  ctx.Conn.DeviceID,
		UserAgent:       ctx.Conn.UserAgent,
		DeviceType:      ua.DeviceType,
		PlatformVersion: ua.PlatformVersion,
		Os:              ua.Os,
		RemoteAddr:      ctx.Conn.RemoteAddr,
		UniqueMac:       uniqueMac,
		MacAddr:         macHw.String(),
		MacHw:           macHw,
	}
	if ctx.UserInfo != nil {
		sess.LimitTime = ctx.UserInfo.LimitTime
	}

	other := &dbdata.SettingOther{}
	dbdata.Settings.Get(other)
	profileHash, err := dbdata.Settings.GetProfileHash()
	if err != nil {
		base.Error(err)
		http.Error(w, "profile err", http.StatusInternalServerError)
		return
	}
	s.sessions.Add(sess)
	rd := RequestData{
		SessionId:    sess.Sid,
		SessionToken: sess.Sid + "@" + sess.Token,
		ProfileName:  base.GetCfg().ProfileName,
		ProfileHash:  profileHash,
		CertHash:     s.CertHash(),
	}
	if other.BannerEnable {
		rd.Banner = other.Banner
	}

	w.WriteHeader(http.StatusOK)
	tplRequest(tpl_complete, w, rd)
	base.Info("login", dbdata.Users.Label(ctx.Conn.Username, ctx.Conn.Nickname), ctx.Conn.UserAgent)
}

// 设置认证会话 Cookie
func SetCookie(w http.ResponseWriter, name, value string, maxAge int) {
	cookie := &http.Cookie{
		Name:     name,
		Value:    value,
		MaxAge:   maxAge,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)
}

// 获取指定名称的 Cookie 值
func GetCookie(r *http.Request, name string) (string, error) {
	cookie, err := r.Cookie(name)
	if err != nil {
		return "", fmt.Errorf("failed to get cookie: %w", err)
	}
	return cookie.Value, nil
}

// 删除认证会话 Cookie
func DeleteCookie(w http.ResponseWriter, name string) {
	cookie := &http.Cookie{
		Name:     name,
		Value:    "",
		MaxAge:   -1,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	}
	http.SetCookie(w, cookie)
}

// 生成 32 位随机会话 ID
func GenerateSessionID() string {
	return utils.RandomRunes(32)
}
