package jwt

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

// 已吊销的 JWT jti => 过期时间，GetJwtData 校验时查询，过期条目懒清理
var (
	jwtMu      sync.Mutex
	jwtRevoked = make(map[string]int64)
)

// 已签发登录 JWT 的主体索引：jti => 主体+过期时间，用于按主体批量吊销
var (
	jwtIssuedMu sync.Mutex
	jwtIssued   = make(map[string]jwtIssuedRecord)
)

type jwtIssuedRecord struct {
	Subject   string
	ExpiresAt int64
}

const jwtIssuedPurgeThreshold = 10000

var jwtNotBefore = time.Now().Unix()
var jwtNotBeforeExemptClaims = []string{"webvpn_user"}

const (
	JwtAdminLoginTTLSec     = 3600 * 3      // 管理员登录 JWT：3 小时
	JwtUserResetTTLSec      = 3600 * 24 * 3 // 用户重置/通知 JWT：3 天
	JwtRevokeFallbackTTLSec = 3600 * 24     // 吊销时未知过期时间则保留 24 小时
)

// 吊销指定 jti，expiresAt 为该 JWT 过期时间（0 表示未知则保留 24h）
func RevokeJwt(jti string, expiresAt int64) {
	if jti == "" {
		return
	}
	now := time.Now().Unix()
	if expiresAt <= 0 {
		expiresAt = now + JwtRevokeFallbackTTLSec
	}
	jwtMu.Lock()
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

// 登记已签发的登录 JWT（按主体索引），供改密/停用时批量吊销；
// 仅登录令牌需要登记，短期挑战令牌不登记
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
		now := time.Now().Unix()
		for k, rec := range jwtIssued {
			if rec.ExpiresAt < now {
				delete(jwtIssued, k)
			}
		}
	}
	jwtIssuedMu.Unlock()
}

// 批量吊销某主体全部未过期 JWT（含当前会话，无法区分"当前"与"被窃"令牌）
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
	jti := utils.RandomRunes(16)
	jwtData := jwt.MapClaims{"exp": expiresAt, "jti": jti, "iat": time.Now().Unix()}
	maps.Copy(jwtData, data)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtData)

	return token.SignedString([]byte(base.GetCfg().JwtSecret))
}

// 从 JWT 字符串解析 jti（用于吊销），解析失败返回空串
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
		// 签发侧固定 HS256，验证侧强制只接受 HS256
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

	if jti, _ := claims["jti"].(string); jti != "" {
		jwtMu.Lock()
		_, revoked := jwtRevoked[jti]
		jwtMu.Unlock()
		if revoked {
			return nil, errors.New("JWT 已吊销")
		}
	}

	exempt := false
	for _, key := range jwtNotBeforeExemptClaims {
		if v, _ := claims[key].(string); v != "" {
			exempt = true
			break
		}
	}
	if !exempt {
		if iat, _ := claims["iat"].(float64); int64(iat) < jwtNotBefore {
			return nil, errors.New("JWT 已吊销")
		}
	}

	return claims, nil
}
