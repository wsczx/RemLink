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

// 管理 WebVPN 会话的签发、校验、续期与吊销
// 会话以 JWT（webvpn_session cookie）承载，与门户 portal_session 完全独立的命名与域
// 通过 exchange-token（webvpn_grant）实现受控免登，彻底消除 cookie 互相踩踏
type AuthSessionManager struct {
	// 进程内用户缓存，避免每次请求回查 DB（WebVPN 反代命中率高，TTL 60s）
	userMu    sync.Mutex
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
	// 会话滑动续期周期（分钟），默认 60。
	sessionTTLDefaultMin = 60
	// 会话绝对寿命上限（分钟），默认 480（8h），自首次登录起算
	sessionMaxLifetimeDefaultMin = 480

	// grant 免登授权时效（秒）跟随门户会话寿命
	grantTTLSec = 3600 * 3
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

// 签发 WebVPN 会话 JWT 并写入 cookie（通过 w）。所有签发路径（兑换、续期）统一由此写 cookie
// issuedAt 为会话首次登录时间（unix 秒）；续期时传入旧 token 的锚点以保证绝对寿命连续
// w 为 nil 时仅返回 token 不写 cookie（防御性，正常情况下调用方均传入 w）
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

// 在门户登录成功后签发免登授权（webvpn_grant cookie）
// 绑定门户会话 jti，供 WebVPN 侧 ExchangeGrant 兑换正式会话
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

// 用免登授权换取正式 WebVPN 会话并写入 cookie（通过 w），返回 (token, user, ok)
// grant 不可用（缺失/过期）时，若门户会话（portal_session JWT）仍然有效，则基于门户身份
// 直接签发 WebVPN 会话，实现免登自动续接
func (m *AuthSessionManager) ExchangeGrant(w http.ResponseWriter, r *http.Request) (string, *dbdata.User, bool) {
	// 优先用免登授权（webvpn_grant）
	if c, err := r.Cookie(grantCookieName); err == nil && c.Value != "" {
		data, err := admin.GetJwtData(c.Value)
		if err == nil {
			username, _ := data["webvpn_grant_user"].(string)
			if username != "" {
				user := m.freshUser(username)
				if user == nil {
					// 本地库查不到：回退到 grant 携带的三方认证身份
					// （门户放行的三方用户未落库，凭 grant 自带 type+groups 重建）
					user = m.externalUserFromClaims(username, data)
				}
				if user != nil {
					token, err := m.Issue(w, r, user, 0)
					if err == nil {
						return token, user, true
					}
				}
			}
		}
	}
	// grant 不可用：回退到门户会话，门户仍登录着则自动重兑 WebVPN 会话
	if user, ok := m.userFromPortalSession(r); ok && user != nil {
		token, err := m.Issue(w, r, user, 0)
		if err == nil {
			return token, user, true
		}
	}
	return "", nil, false
}

// 在免登授权缺失/失效时，用仍有效的门户会话（portal_session JWT）
// 解析用户身份；JWT 过期或非法时返回 (nil, false)。用户有效性由 freshUser 复核。
func (m *AuthSessionManager) userFromPortalSession(r *http.Request) (*dbdata.User, bool) {
	c, err := r.Cookie("portal_session")
	if err != nil || c.Value == "" {
		return nil, false
	}
	data, err := admin.GetJwtData(c.Value)
	if err != nil {
		return nil, false
	}
	username, _ := data["portal_user"].(string)
	if username == "" {
		return nil, false
	}
	// 门户仍登录着：本地库能查到则用，查不到回退到 JWT 携带的
	// 三方用户身份（门户放行的三方用户可直接重兑 WebVPN 会话，无需落库）
	if u := m.freshUser(username); u != nil {
		return u, true
	}
	if eu := m.externalUserFromClaims(username, data); eu != nil {
		return eu, true
	}
	return nil, false
}

// 从请求解析当前 WebVPN 会话用户
// 依次校验：token 合法、未被整用户踢出、未超绝对寿命、用户在库且启用
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

	// 整用户踢出阈值：该时间戳及之前签发的会话一律视为已吊销
	if iat := jwtInt64(data, "iat"); iat > 0 {
		if before := dbdata.WebVpnRevokeBeforeOf(username); before > 0 && iat <= before {
			return nil, false
		}
	}

	// 绝对寿命上限：会话自首次登录（webvpn_issued）起算，超过上限强制重新登录
	// 即使持续活跃、不断滑动续期也会到期。无锚点（旧 token）按 iat 兜底
	if issued := jwtInt64(data, "webvpn_issued"); issued > 0 {
		if max := m.sessionMaxLifetime(); max > 0 {
			if time.Since(time.Unix(issued, 0)) > max {
				return nil, false
			}
		}
	}

	// 解析得到基础 user（缓存优先，未命中回查 DB）；组在下方统一按 token 注入
	// 本地库查不到时回退到 JWT 携带的三方用户身份（门户放行的三方用户）
	user := m.cachedUser(username)
	if user == nil || user.Status != 1 {
		if eu := m.externalUserFromClaims(username, data); eu != nil {
			user = eu
		} else {
			return nil, false
		}
	}

	// 会话令牌携带的组用于应用组授权。组随每次请求 token 重新解析，不写入缓存 user
	// 避免不同 token 的组在缓存命中时被串
	if g, ok := data["webvpn_groups"]; ok {
		if gs := toStringSlice(g); len(gs) > 0 {
			u2 := *user
			u2.Groups = gs
			return &u2, true
		}
	}
	return user, true
}

// 在距签发超过 TTL-1h 时重签并刷新 cookie，以数据库当前用户状态为准
// 返回是否实际重签；w 为 nil 时不写 cookie
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
	if time.Since(time.Unix(iat, 0)) <= m.sessionTTL()-time.Hour {
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

// 吊销当前请求的 WebVPN 会话（单点登出）
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

// 清除一次性的免登授权 cookie（兑换成功后或登出时调用）
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

// 清空进程内用户缓存（仅用于测试隔离用例间状态，生产路径不调用）
func (m *AuthSessionManager) ResetCache() {
	m.userMu.Lock()
	m.userCache = make(map[string]*userCacheEntry)
	m.userMu.Unlock()
}

// 清除客户端 webvpn_session cookie
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

// 整用户踢出（管理后台“全量登出”）。通过抬高吊销阈值实现 O(1) 失效
// 并清该用户缓存，避免命中旧 user
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

// 在本地库查不到用户时，从 JWT claims 重建身份。
// 仅当 type 非本地账户（local/ldap）且组非空才放行
// 门户放行的三方（钉钉等）用户，WebVPN 侧也应建立身份
// 否则会出现「门户能登录、WebVPN 却被当未登录踢回」的不一致
func (m *AuthSessionManager) externalUserFromClaims(username string, data map[string]any) *dbdata.User {
	if username == "" {
		return nil
	}
	// 统一认三类 JWT 的身份类型字段：免登授权、WebVPN 会话、门户会话
	userType, _ := data["webvpn_grant_type"].(string)
	if userType == "" {
		userType, _ = data["webvpn_type"].(string)
	}
	if userType == "" {
		userType, _ = data["portal_type"].(string)
	}
	// 本地账户（local/ldap）必须落库才认，避免删除后凭旧 JWT 重建
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

// 从数据库重新加载用户当前状态，用于续期/兑换时以服务端权威数据重签
// 本地账户查库失败即返回 nil；三方用户回退到 JWT 携带的
// 身份（type+groups），使门户放行的三方用户也能在 WebVPN 侧建立会话
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

// 兼容 JWT 解析后数字字段的多种类型（float64/int64/json.Number/string）
// 避免依赖具体类型断言导致整用户踢出阈值等关键逻辑失效
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
