package admin

import (
	"errors"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/utils"
)

// 已吊销的 JWT jti => 过期时间(unix)，登出/改密时写入，GetJwtData 校验时查询
// 条目在原 JWT 过期后即无保留意义，吊销时顺带懒清理，避免集合无限增长
var (
	jwtMu      sync.Mutex
	jwtRevoked = make(map[string]int64)
)

// 已签发登录 JWT 的主体索引：jti => 主体+过期时间
// 用于按主体批量吊销（改密/停用/删用户）
var (
	jwtIssuedMu sync.Mutex
	jwtIssued   = make(map[string]jwtIssuedRecord)
)

type jwtIssuedRecord struct {
	Subject   string // 吊销主体：admin_user 或 portal_user 的值
	ExpiresAt int64
}

// 登记数超过该值时触发一次过期清理（摊销 O(1)，避免索引无限增长）
const jwtIssuedPurgeThreshold = 10000

// JWT 有效期常量（秒），集中管理避免散落字面量
const (
	JwtAdminLoginTTLSec     = 3600 * 3      // 管理员登录 JWT：3 小时
	JwtUserResetTTLSec      = 3600 * 24 * 3 // 用户重置/通知 JWT：3 天
	JwtRevokeFallbackTTLSec = 3600 * 24     // 吊销时未知过期时间则保留 24 小时
)

// 吊销指定 jti 的 JWT，expiresAt 为该 JWT 的过期时间（用于清理，0 表示未知则保留 24h）
func RevokeJwt(jti string, expiresAt int64) {
	if jti == "" {
		return
	}
	now := time.Now().Unix()
	if expiresAt <= 0 {
		expiresAt = now + JwtRevokeFallbackTTLSec
	}
	jwtMu.Lock()
	// 剔除已过期条目
	for k, exp := range jwtRevoked {
		if exp < now {
			delete(jwtRevoked, k)
		}
	}
	jwtRevoked[jti] = expiresAt
	jwtMu.Unlock()
}

// 吊销整条 JWT（解析出 jti 后吊销），用于登出等场景
func RevokeJwtToken(tokenString string) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		return []byte(base.GetCfg().JwtSecret), nil
	})
	if err != nil {
		return
	}
	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		if jti, _ := claims["jti"].(string); jti != "" {
			exp, _ := claims["exp"].(float64)
			RevokeJwt(jti, int64(exp))
		}
	}
}

// 登记已签发的登录 JWT（按主体索引），供改密/停用时批量吊销
// 仅登录令牌需要登记；otp_token、重置令牌等短期挑战令牌不登记
func RegisterJwt(subject, tokenString string) {
	if subject == "" || tokenString == "" {
		return
	}
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		return []byte(base.GetCfg().JwtSecret), nil
	})
	if err != nil {
		return
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return
	}
	jti, _ := claims["jti"].(string)
	if jti == "" {
		return
	}
	exp, _ := claims["exp"].(float64)

	jwtIssuedMu.Lock()
	jwtIssued[jti] = jwtIssuedRecord{Subject: subject, ExpiresAt: int64(exp)}
	if len(jwtIssued) > jwtIssuedPurgeThreshold {
		// 登记量超阈值时清理一次过期条目
		now := time.Now().Unix()
		for k, rec := range jwtIssued {
			if rec.ExpiresAt < now {
				delete(jwtIssued, k)
			}
		}
	}
	jwtIssuedMu.Unlock()
}

// 批量吊销某主体（管理员/门户用户）全部未过期的 JWT
// 用于改密、停用、重置密码、删除用户等场景。返回吊销数量
// 注意：会连同当前会话一并吊销（无法区分"当前"与"被窃"令牌）
func RevokeSubjectJwt(subject string) int {
	if subject == "" {
		return 0
	}
	type revokeItem struct {
		jti string
		exp int64
	}
	var items []revokeItem

	jwtIssuedMu.Lock()
	now := time.Now().Unix()
	for jti, rec := range jwtIssued {
		if rec.ExpiresAt < now {
			delete(jwtIssued, jti)
			continue
		}
		if rec.Subject == subject {
			items = append(items, revokeItem{jti: jti, exp: rec.ExpiresAt})
			delete(jwtIssued, jti)
		}
	}
	jwtIssuedMu.Unlock()

	for _, it := range items {
		RevokeJwt(it.jti, it.exp)
	}
	return len(items)
}

func SetJwtData(data map[string]any, expiresAt int64) (string, error) {
	// 用于支持单条 JWT 吊销
	jti := utils.RandomRunes(16)
	jwtData := jwt.MapClaims{"exp": expiresAt, "jti": jti, "iat": time.Now().Unix()}
	maps.Copy(jwtData, data)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtData)

	// 使用密钥获取完整的编码令牌字符串
	tokenString, err := token.SignedString([]byte(base.GetCfg().JwtSecret))
	return tokenString, err
}

// 从 JWT 字符串解析 jti（用于单条/全量吊销），解析失败返回空串
func JtiOf(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		return []byte(base.GetCfg().JwtSecret), nil
	})
	if err != nil {
		return "", err
	}
	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		if jti, _ := claims["jti"].(string); jti != "" {
			return jti, nil
		}
	}
	return "", errors.New("JWT 无 jti")
}

// 从 WebVPN 会话 JWT 解析 webvpn_user（全量登出时定位索引）
func UsernameOf(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		return []byte(base.GetCfg().JwtSecret), nil
	})
	if err != nil {
		return "", err
	}
	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		if u, _ := claims["webvpn_user"].(string); u != "" {
			return u, nil
		}
	}
	return "", errors.New("JWT 无 webvpn_user")
}

func GetJwtData(jwtToken string) (map[string]any, error) {
	token, err := jwt.Parse(jwtToken, func(token *jwt.Token) (any, error) {
		// 签发侧固定使用 HS256，验证侧强制只接受 HS256
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("不支持的签名算法: %v", token.Header["alg"])
		}
		return []byte(base.GetCfg().JwtSecret), nil
	})

	if err != nil || !token.Valid {
		if err == nil {
			err = errors.New("JWT token 无效")
		}
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("data is parse err")
	}

	// 吊销校验：被吊销的 JWT 即使未过期也拒绝
	if jti, _ := claims["jti"].(string); jti != "" {
		jwtMu.Lock()
		_, revoked := jwtRevoked[jti]
		jwtMu.Unlock()
		if revoked {
			return nil, errors.New("JWT 已吊销")
		}
	}

	return claims, nil
}
