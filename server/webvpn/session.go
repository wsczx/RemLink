package webvpn

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wsczx/remlink/admin"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

type AuthSessionManager struct {
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

func NewSessionManager() *AuthSessionManager {
	return &AuthSessionManager{
		userCache:         make(map[string]*userCacheEntry),
		userTTL:           60 * time.Second,
		userCacheMaxSize:  1000,
		userCacheMinClean: time.Minute,
	}
}

func (m *AuthSessionManager) sessionTTL() time.Duration {
	min := base.GetCfg().WebVpnSessionTTL
	if min <= 0 {
		min = sessionTTLDefaultMin
	}
	return time.Duration(min) * time.Minute
}

func (m *AuthSessionManager) sessionMaxLifetime() time.Duration {
	min := base.GetCfg().WebVpnSessionMaxLifetime
	if min <= 0 {
		min = sessionMaxLifetimeDefaultMin
	}
	return time.Duration(min) * time.Minute
}

// 签发 WebVPN 会话并写入 cookie
func (m *AuthSessionManager) Issue(w http.ResponseWriter, r *http.Request, user *dbdata.User, issuedAt int64) (string, error) {
	now := time.Now()
	if issuedAt <= 0 {
		issuedAt = now.Unix()
	}
	expiresAt := now.Add(m.sessionTTL()).Unix()
	token, err := admin.SetJwtData(map[string]any{
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

// 签发绑定门户会话的 WebVPN 免登授权
func (m *AuthSessionManager) IssueGrant(w http.ResponseWriter, r *http.Request, user *dbdata.User, portalJTI string) (string, error) {
	expiresAt := time.Now().Add(grantTTLSec * time.Second).Unix()
	token, err := admin.SetJwtData(map[string]any{
		"webvpn_grant_user": user.Username,
		"webvpn_grant_type": user.Type,
		"webvpn_grant_jti":  portalJTI, // 绑定门户会话，门户登出后该 grant 也应随之失效
		"webvpn_groups":     user.Groups,
	}, expiresAt)
	if err != nil {
		return "", err
	}
	m.setGrantCookie(w, r, token)
	return token, nil
}

// 用免登授权换取 WebVPN 会话，失败时回退到门户会话
func (m *AuthSessionManager) ExchangeGrant(w http.ResponseWriter, r *http.Request) (string, *dbdata.User, bool) {
	// 优先兑换一次性 grant；若携带门户会话则校验其 JTI。
	if c, err := r.Cookie(grantCookieName); err == nil && c.Value != "" {
		m.grantMu.Lock()
		defer m.grantMu.Unlock()

		data, err := admin.GetJwtData(c.Value)
		if err == nil {
			username, _ := data["webvpn_grant_user"].(string)
			grantJTI, _ := data["webvpn_grant_jti"].(string)
			portal, portalOK := m.portalSessionData(r)
			portalJTI, _ := portal["jti"].(string)
			// 子域通常收不到主门户的 host-only cookie，因此无门户会话时仍允许兑换；
			// 若携带门户会话，则必须校验 JTI 一致。
			grantValid := username != "" && grantJTI != "" && (!portalOK || grantJTI == portalJTI)
			if grantValid {
				if before := dbdata.WebVpnRevokeBeforeOf(username); before > 0 && jwtInt64(data, "iat") <= before {
					grantValid = false
				}
			}
			if grantValid {
				user := m.freshUser(username)
				if user == nil {
					// 本地库查不到时回退到 grant 携带的三方身份
					user = m.externalUserFromClaims(username, data)
				}
				if user != nil {
					token, err := m.Issue(w, r, user, 0)
					if err == nil {
						// JWT 本身不可变；兑换成功后吊销其 jti，防止复制的 grant 重放。
						admin.RevokeJwtToken(c.Value)
						m.ClearGrantCookie(w, r)
						return token, user, true
					}
				}
			}
		}
	}
	// 回退到门户会话
	if user, ok := m.userFromPortalSession(r); ok && user != nil {
		if before := dbdata.WebVpnRevokeBeforeOf(user.Username); before > 0 && m.portalIssuedAt(r) <= before {
			return "", nil, false
		}
		token, err := m.Issue(w, r, user, 0)
		if err == nil {
			return token, user, true
		}
	}
	return "", nil, false
}

func (m *AuthSessionManager) portalSessionData(r *http.Request) (map[string]any, bool) {
	c, err := r.Cookie("portal_session")
	if err != nil || c.Value == "" {
		return nil, false
	}
	data, err := admin.GetJwtData(c.Value)
	if err != nil {
		return nil, false
	}
	return data, true
}

// 返回有效门户会话的签发时间；不存在时返回 0。
func (m *AuthSessionManager) portalIssuedAt(r *http.Request) int64 {
	data, ok := m.portalSessionData(r)
	if !ok {
		return 0
	}
	return jwtInt64(data, "iat")
}

// 免登授权失效时，用仍有效的门户会话解析用户身份。返回 (nil, false) 表示无有效门户会话。
func (m *AuthSessionManager) userFromPortalSession(r *http.Request) (*dbdata.User, bool) {
	data, ok := m.portalSessionData(r)
	if !ok {
		return nil, false
	}
	username, _ := data["portal_user"].(string)
	if username == "" {
		return nil, false
	}
	// 本地库能查到则用，查不到回退 JWT 携带的三方身份
	if u := m.freshUser(username); u != nil {
		return u, true
	}
	if eu := m.externalUserFromClaims(username, data); eu != nil {
		return eu, true
	}
	return nil, false
}

// 从请求解析当前 WebVPN 会话用户（校验 token 合法、未被踢出、未超绝对寿命）。
func (m *AuthSessionManager) CurrentUser(r *http.Request) (*dbdata.User, bool) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return nil, false
	}
	return m.UserFromToken(c.Value)
}

// 解析并校验 WebVPN 会话 token，返回当前用户（供测试与内部复用）
func (m *AuthSessionManager) UserFromToken(token string) (*dbdata.User, bool) {
	data, err := admin.GetJwtData(token)
	if err != nil {
		return nil, false
	}
	username, _ := data["webvpn_user"].(string)
	if username == "" {
		return nil, false
	}

	// 整用户踢出：该时间戳及之前签发的会话一律失效
	if iat := jwtInt64(data, "iat"); iat > 0 {
		if before := dbdata.WebVpnRevokeBeforeOf(username); before > 0 && iat <= before {
			return nil, false
		}
	}

	// 绝对寿命上限：自首次登录起算，持续活跃也会到期
	if issued := jwtInt64(data, "webvpn_issued"); issued > 0 {
		if max := m.sessionMaxLifetime(); max > 0 {
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

// 会话已使用过半生命周期时，以数据库当前状态重签并刷新 cookie。
func (m *AuthSessionManager) Renew(w http.ResponseWriter, r *http.Request) (bool, error) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return false, nil
	}
	data, err := admin.GetJwtData(c.Value)
	if err != nil {
		return false, nil
	}
	iat := jwtInt64(data, "iat")
	if iat <= 0 {
		return false, nil
	}
	ttl := m.sessionTTL()
	if ttl <= 0 || time.Since(time.Unix(iat, 0)) <= ttl/2 {
		return false, nil
	}
	username, _ := data["webvpn_user"].(string)
	issued := jwtInt64(data, "webvpn_issued")
	if fresh := m.freshUser(username); fresh != nil {
		if _, err := m.Issue(w, r, fresh, issued); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// 吊销当前会话（单点登出）。
func (m *AuthSessionManager) RevokeCurrent(r *http.Request) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return
	}
	if jti, jerr := admin.JtiOf(c.Value); jerr == nil {
		if data, e := admin.GetJwtData(c.Value); e == nil {
			admin.RevokeJwt(jti, jwtInt64(data, "exp"))
		}
	}
}

// 清除一次性免登授权 cookie。
func (m *AuthSessionManager) ClearGrantCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     grantCookieName,
		Value:    "",
		Path:     "/",
		Domain:   wildcardDomain(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
	})
}

// 清空进程内用户缓存（仅测试隔离用）。
func (m *AuthSessionManager) ResetCache() {
	m.userMu.Lock()
	m.userCache = make(map[string]*userCacheEntry)
	m.userMu.Unlock()
}

// 清除客户端 webvpn_session cookie。
func (m *AuthSessionManager) ClearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Domain:   CookieDomain(r.Host),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
	})
}

// 整用户踢出（全量登出）：抬高吊销阈值使旧会话 O(1) 失效，并清缓存。
func (m *AuthSessionManager) RevokeUser(username string) {
	dbdata.WebVpnRevokeUser(username)
	m.userMu.Lock()
	delete(m.userCache, username)
	m.userMu.Unlock()
}

func (m *AuthSessionManager) setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	if w == nil {
		return
	}
	http.SetCookie(w, m.cookie(sessionCookieName, token, r))
}

func (m *AuthSessionManager) setGrantCookie(w http.ResponseWriter, r *http.Request, token string) {
	if w == nil {
		return
	}
	c := m.cookie(grantCookieName, token, r)
	c.Domain = wildcardDomain()
	http.SetCookie(w, c)
}

func (m *AuthSessionManager) cookie(name, token string, r *http.Request) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    token,
		Path:     "/",
		Domain:   CookieDomain(r.Host),
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
	}
}

func (m *AuthSessionManager) cachedUser(username string) *dbdata.User {
	m.userMu.Lock()
	if e, ok := m.userCache[username]; ok && e.expire.After(time.Now()) {
		user := e.user
		m.userMu.Unlock()
		return user
	}
	m.userMu.Unlock()

	u := &dbdata.User{}
	if err := dbdata.One("Username", username, u); err != nil || u.Status != 1 {
		m.userMu.Lock()
		m.userCache[username] = &userCacheEntry{user: nil, expire: time.Now().Add(m.userTTL)}
		m.maybeCleanLocked(time.Now())
		m.userMu.Unlock()
		return nil
	}
	m.userMu.Lock()
	m.userCache[username] = &userCacheEntry{user: u, expire: time.Now().Add(m.userTTL)}
	m.maybeCleanLocked(time.Now())
	m.userMu.Unlock()
	return u
}

// 本地库查不到时从 JWT claims 重建三方身份；type 为 local/ldap 必须落库，组非空才放行。
func (m *AuthSessionManager) externalUserFromClaims(username string, data map[string]any) *dbdata.User {
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

// 从数据库重载用户当前状态，用于续期/兑换时以服务端权威数据重签；查库失败返回 nil。
func (m *AuthSessionManager) freshUser(username string) *dbdata.User {
	if username == "" {
		return nil
	}
	u := &dbdata.User{}
	if err := dbdata.One("Username", username, u); err == nil && u.Status == 1 {
		return u
	}
	return nil
}

func (m *AuthSessionManager) maybeCleanLocked(now time.Time) {
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

// 兼容 JWT 数字字段多种类型（float64/int64/json.Number/string）。
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
