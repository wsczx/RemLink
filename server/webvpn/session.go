package webvpn

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/jwt"
	"github.com/wsczx/remlink/pkg/utils"
)

type AuthSessions struct {
	userMu  sync.Mutex
	grantMu sync.Mutex

	userCache map[string]*userCacheEntry
	userTTL   time.Duration

	userCacheMaxSize  int
	userCacheMinClean time.Duration
	userLastClean     time.Time
}
type userCacheEntry struct {
	user   *dbdata.User
	expire time.Time
}

const (
	sessionTTLDefaultMin         = 60
	sessionMaxLifetimeDefaultMin = 480
	grantTTLSec                  = 3600 * 3
)

type crossSiteCookieContextKey struct{}

func WithCrossSiteCookie(r *http.Request, enabled bool) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), crossSiteCookieContextKey{}, enabled))
}
func NewAuthSessions() *AuthSessions {
	return &AuthSessions{
		userCache:         make(map[string]*userCacheEntry),
		userTTL:           60 * time.Second,
		userCacheMaxSize:  1000,
		userCacheMinClean: time.Minute,
	}
}
func (m *AuthSessions) ttl() time.Duration {
	min := base.GetCfg().WebVpnSessionTTL
	if min <= 0 {
		min = sessionTTLDefaultMin
	}
	return time.Duration(min) * time.Minute
}
func (m *AuthSessions) maxLifetime() time.Duration {
	min := base.GetCfg().WebVpnSessionMaxLifetime
	if min <= 0 {
		min = sessionMaxLifetimeDefaultMin
	}
	return time.Duration(min) * time.Minute
}

// 签发会话并写入 cookie
func (m *AuthSessions) Issue(w http.ResponseWriter, r *http.Request, user *dbdata.User, issuedAt int64) (string, error) {
	now := time.Now()
	if issuedAt <= 0 {
		issuedAt = now.Unix()
	}
	expiresAt := now.Add(m.ttl()).Unix()
	iat := now.Unix()
	if before := dbdata.WebVpn.RevokeBeforeOf(user.Username); before >= iat {
		iat = before + 1
	}
	token, err := jwt.SetJwtData(map[string]any{
		"iat":           iat,
		"webvpn_user":   user.Username,
		"webvpn_type":   user.Type,
		"webvpn_groups": user.Groups,
		"webvpn_issued": issuedAt,
	}, expiresAt)
	if err != nil {
		return "", err
	}
	m.setSessionCookie(w, r, token)
	return token, nil
}

// 签发绑定门户会话的一次性授权
func (m *AuthSessions) IssueGrant(w http.ResponseWriter, r *http.Request, user *dbdata.User, portalJTI string) (string, error) {
	expiresAt := time.Now().Add(grantTTLSec * time.Second).Unix()
	iat := time.Now().Unix()
	if before := dbdata.WebVpn.RevokeBeforeOf(user.Username); before >= iat {
		iat = before + 1
	}
	token, err := jwt.SetJwtData(map[string]any{
		"iat":               iat,
		"webvpn_grant_user": user.Username,
		"webvpn_grant_type": user.Type,
		"webvpn_grant_jti":  portalJTI, // 绑定门户会话，门户登出后该 grant 也应随之失效
		"webvpn_grant_ip":   ClientIP(r),
		"webvpn_groups":     user.Groups,
	}, expiresAt)
	if err != nil {
		return "", err
	}
	m.setGrantCookie(w, r, token)
	return token, nil
}

// 先兑换一次性授权，失败回退门户会话
func (m *AuthSessions) ExchangeGrant(w http.ResponseWriter, r *http.Request) (string, *dbdata.User, bool) {
	// 优先兑换一次性 grant；URL 参数用于跨门户子域回跳，Cookie 作为兼容回退
	grantValue := ""
	if value := r.URL.Query().Get(grantCookieName); value != "" {
		grantValue = value
	} else if c, err := r.Cookie(grantCookieName); err == nil && c.Value != "" {
		grantValue = c.Value
	}
	if grantValue != "" {
		m.grantMu.Lock()
		defer m.grantMu.Unlock()

		data, err := jwt.GetJwtData(grantValue)
		if err != nil {
			// 确定失效，清 cookie 避免重复兑换
			m.ClearGrantCookie(w, r)
		} else {
			username, _ := data["webvpn_grant_user"].(string)
			grantJTI, _ := data["webvpn_grant_jti"].(string)
			portal, portalOK := m.portalSessionData(r)
			portalJTI, _ := portal["jti"].(string)
			// 子域收不到主门户 host-only cookie，无门户会话也允许兑换；有则校验 JTI
			grantValid := username != "" && grantJTI != "" && (!portalOK || grantJTI == portalJTI)
			// grant cookie 为 wildcard 域、可跨子域携带，URL 与 cookie 两条路径都必须校验客户端绑定
			if grantValid {
				grantIP, _ := data["webvpn_grant_ip"].(string)
				if grantIP == "" || grantIP != ClientIP(r) {
					grantValid = false
				}
			}
			if grantValid {
				if before := dbdata.WebVpn.RevokeBeforeOf(username); before > 0 && jwtInt64(data, "iat") <= before {
					grantValid = false
				}
			}
			if !grantValid {
				m.ClearGrantCookie(w, r)
			} else {
				user := m.userFromDB(username)
				if user == nil {
					// 本地库查不到时回退到 grant 携带的三方身份
					user = m.externalUserFromClaims(username, data)
				}
				if user == nil {
					m.ClearGrantCookie(w, r)
				} else if user.IsExpired() {
					m.ClearGrantCookie(w, r)
				} else {
					token, err := m.Issue(w, r, user, 0)
					if err == nil {
						// JWT 本身不可变；兑换成功后吊销其 jti，防止复制的 grant 重放
						jwt.RevokeJwtToken(grantValue)
						m.ClearGrantCookie(w, r)
						return token, user, true
					}
				}
			}
		}
	}
	if user, ok := m.userFromPortalSession(r); ok && user != nil {
		if user.IsExpired() {
			return "", nil, false
		}
		if before := dbdata.WebVpn.RevokeBeforeOf(user.Username); before > 0 && m.portalIssuedAt(r) <= before {
			return "", nil, false
		}
		token, err := m.Issue(w, r, user, 0)
		if err == nil {
			if _, err := r.Cookie(grantCookieName); err == nil {
				m.ClearGrantCookie(w, r)
			}
			return token, user, true
		}
	}
	return "", nil, false
}
func (m *AuthSessions) portalSessionData(r *http.Request) (map[string]any, bool) {
	c, err := r.Cookie("portal_session")
	if err != nil || c.Value == "" {
		return nil, false
	}
	data, err := jwt.GetJwtData(c.Value)
	if err != nil {
		return nil, false
	}
	return data, true
}

// 无有效门户会话时返回 0
func (m *AuthSessions) portalIssuedAt(r *http.Request) int64 {
	data, ok := m.portalSessionData(r)
	if !ok {
		return 0
	}
	return jwtInt64(data, "iat")
}

// 无有效门户会话时返回 (nil, false)
func (m *AuthSessions) userFromPortalSession(r *http.Request) (*dbdata.User, bool) {
	data, ok := m.portalSessionData(r)
	if !ok {
		return nil, false
	}
	username, _ := data["portal_user"].(string)
	if username == "" {
		return nil, false
	}
	// 本地库能查到则用，查不到回退 JWT 携带的三方身份
	if u := m.userFromDB(username); u != nil {
		return u, true
	}
	if eu := m.externalUserFromClaims(username, data); eu != nil {
		return eu, true
	}
	return nil, false
}

// 从请求解析当前会话用户
func (m *AuthSessions) UserFromRequest(r *http.Request) (*dbdata.User, bool) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return nil, false
	}
	return m.UserFromToken(c.Value)
}

// 校验 token 并返回当前用户
func (m *AuthSessions) UserFromToken(token string) (*dbdata.User, bool) {
	data, err := jwt.GetJwtData(token)
	if err != nil {
		return nil, false
	}
	username, _ := data["webvpn_user"].(string)
	if username == "" {
		return nil, false
	}

	// 整用户踢出：该时间戳及之前签发的会话一律失效
	if iat := jwtInt64(data, "iat"); iat > 0 {
		if before := dbdata.WebVpn.RevokeBeforeOf(username); before > 0 && iat <= before {
			return nil, false
		}
	}

	// 绝对寿命上限：自首次登录起算，持续活跃也会到期
	if issued := jwtInt64(data, "webvpn_issued"); issued > 0 {
		if max := m.maxLifetime(); max > 0 {
			if time.Since(time.Unix(issued, 0)) > max {
				return nil, false
			}
		}
	}

	user := m.cachedUser(username)
	if user == nil || user.Status != 1 {
		if eu := m.externalUserFromClaims(username, data); eu != nil {
			user = eu
		} else {
			return nil, false
		}
	}
	if user.IsExpired() {
		return nil, false
	}

	// 组随每次请求 token 重新解析，不写缓存，避免不同 token 的组串味
	if g, ok := data["webvpn_groups"]; ok {
		if gs := toStringSlice(g); len(gs) > 0 {
			u2 := *user
			u2.Groups = gs
			return &u2, true
		}
	}
	return user, true
}

// 过半生命周期后按数据库状态重签并刷新 cookie
func (m *AuthSessions) Renew(w http.ResponseWriter, r *http.Request) (bool, error) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return false, nil
	}
	data, err := jwt.GetJwtData(c.Value)
	if err != nil {
		return false, nil
	}
	iat := jwtInt64(data, "iat")
	if iat <= 0 {
		return false, nil
	}
	ttl := m.ttl()
	if ttl <= 0 || time.Since(time.Unix(iat, 0)) <= ttl/2 {
		return false, nil
	}
	username, _ := data["webvpn_user"].(string)
	if before := dbdata.WebVpn.RevokeBeforeOf(username); before > 0 && iat <= before {
		m.ClearCookie(w, r)
		return false, nil
	}
	issued := jwtInt64(data, "webvpn_issued")
	fresh := m.userFromDB(username)
	if fresh == nil {
		fresh = m.externalUserFromClaims(username, data)
	}
	if fresh != nil && (fresh.Status != 1 || fresh.IsExpired()) {
		fresh = nil
	}
	if fresh == nil {
		return false, nil
	}
	if _, err := m.Issue(w, r, fresh, issued); err != nil {
		return false, err
	}
	return true, nil
}

// 吊销当前 token
func (m *AuthSessions) RevokeFromRequest(r *http.Request) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return
	}
	if jti, jerr := jwt.JtiOf(c.Value); jerr == nil {
		if data, err := jwt.GetJwtData(c.Value); err == nil {
			jwt.RevokeJwt(jti, jwtInt64(data, "exp"))
		}
	}
}

func (m *AuthSessions) RevokeGrantFromRequest(r *http.Request) {
	values := make([]string, 0, 2)
	if value := r.URL.Query().Get(grantCookieName); value != "" {
		values = append(values, value)
	}
	if c, err := r.Cookie(grantCookieName); err == nil && c.Value != "" {
		values = append(values, c.Value)
	}
	for _, value := range values {
		jwt.RevokeJwtToken(value)
	}
}

// 清除一次性授权 cookie
func (m *AuthSessions) ClearGrantCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     grantCookieName,
		Value:    "",
		Path:     "/",
		Domain:   wildcardDomain(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   utils.IsTLS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// 清空用户缓存
func (m *AuthSessions) ResetCache() {
	m.userMu.Lock()
	m.userCache = make(map[string]*userCacheEntry)
	m.userMu.Unlock()
}

// 清除会话 cookie
func (m *AuthSessions) ClearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Domain:   CookieDomain(r.Host),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   utils.IsTLS(r),
		SameSite: sessionSameSite(r),
	})
}

// 抬高吊销阈值使旧会话失效，并清缓存
func (m *AuthSessions) RevokeUser(username string) {
	if err := dbdata.WebVpn.RevokeUser(username); err != nil {
		base.Error("WebVPN 会话吊销持久化失败:", username, err)
	}
	m.userMu.Lock()
	delete(m.userCache, username)
	m.userMu.Unlock()
}

func (m *AuthSessions) setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, m.cookie(sessionCookieName, token, r))
}

func (m *AuthSessions) setGrantCookie(w http.ResponseWriter, r *http.Request, token string) {
	c := m.cookie(grantCookieName, token, r)
	c.Domain = wildcardDomain()
	http.SetCookie(w, c)
}

func (m *AuthSessions) cookie(name, token string, r *http.Request) *http.Cookie {
	sameSite := http.SameSiteLaxMode
	if name == sessionCookieName {
		sameSite = sessionSameSite(r)
	}
	return &http.Cookie{
		Name:     name,
		Value:    token,
		Path:     "/",
		Domain:   CookieDomain(r.Host),
		HttpOnly: true,
		Secure:   utils.IsTLS(r),
		SameSite: sameSite,
	}
}

func sessionSameSite(r *http.Request) http.SameSite {
	if enabled, _ := r.Context().Value(crossSiteCookieContextKey{}).(bool); enabled && utils.IsTLS(r) {
		return http.SameSiteNoneMode
	}
	return http.SameSiteLaxMode
}

func (m *AuthSessions) cachedUser(username string) *dbdata.User {
	m.userMu.Lock()
	if e, ok := m.userCache[username]; ok && e.expire.After(time.Now()) {
		user := e.user
		m.userMu.Unlock()
		return user
	}
	m.userMu.Unlock()

	u := &dbdata.User{}
	if err := dbdata.Users.GetByUsername(username, u); err != nil || u.Status != 1 {
		m.userMu.Lock()
		m.userCache[username] = &userCacheEntry{user: nil, expire: time.Now().Add(m.userTTL)}
		m.cleanIfNeeded(time.Now())
		m.userMu.Unlock()
		return nil
	}
	m.userMu.Lock()
	m.userCache[username] = &userCacheEntry{user: u, expire: time.Now().Add(m.userTTL)}
	m.cleanIfNeeded(time.Now())
	m.userMu.Unlock()
	return u
}

// local/ldap 必须落库；三方身份要求 groups 非空
func (m *AuthSessions) externalUserFromClaims(username string, data map[string]any) *dbdata.User {
	if username == "" {
		return nil
	}
	userType, _ := data["webvpn_grant_type"].(string)
	if userType == "" {
		userType, _ = data["webvpn_type"].(string)
	}
	if userType == "" {
		userType, _ = data["portal_type"].(string)
	}
	if userType == "local" || userType == "ldap" {
		return nil
	}
	groups := toStringSlice(data["webvpn_groups"])
	if len(groups) == 0 {
		groups = toStringSlice(data["portal_groups"])
	}
	if len(groups) == 0 {
		return nil
	}
	return &dbdata.User{
		Type:     userType,
		Username: username,
		Groups:   groups,
		Status:   1,
	}
}

// 查库失败返回 nil
func (m *AuthSessions) userFromDB(username string) *dbdata.User {
	if username == "" {
		return nil
	}
	u := &dbdata.User{}
	if err := dbdata.Users.GetByUsername(username, u); err == nil && u.Status == 1 {
		return u
	}
	return nil
}

func (m *AuthSessions) cleanIfNeeded(now time.Time) {
	if len(m.userCache) <= m.userCacheMaxSize {
		return
	}
	if now.Sub(m.userLastClean) < m.userCacheMinClean {
		return
	}
	m.userLastClean = now
	for k, e := range m.userCache {
		if !e.expire.After(now) {
			delete(m.userCache, k)
		}
	}
}

// 兼容 JWT 数字字段的多种解码类型
func jwtInt64(data map[string]any, key string) int64 {
	switch v := data[key].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case int32:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	}
	return 0
}

func toStringSlice(v any) []string {
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		out := make([]string, 0, len(s))
		for _, it := range s {
			if str, ok := it.(string); ok {
				out = append(out, str)
			}
		}
		return out
	case string:
		if s == "" {
			return nil
		}
		return strings.Split(s, ",")
	}
	return nil
}
