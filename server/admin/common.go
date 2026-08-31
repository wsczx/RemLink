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

// 已吊销的 JWT jti => 过期时间(unix)，登出/改密时写入，GetJwtData 校验时查询。
// 条目在原 JWT 过期后即无保留意义，吊销时顺带懒清理，避免集合无限增长。
var (
	jwtMu      sync.Mutex
	jwtRevoked = make(map[string]int64)
)

// 吊销指定 jti 的 JWT，expiresAt 为该 JWT 的过期时间（用于清理，0 表示未知则保留 24h）
func RevokeJwt(jti string, expiresAt int64) {
	if jti == "" {
		return
	}
	now := time.Now().Unix()
	if expiresAt <= 0 {
		expiresAt = now + 3600*24
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

func SetJwtData(data map[string]any, expiresAt int64) (string, error) {
	// jti 用于支持单条 JWT 吊销
	jti := utils.RandomRunes(16)
	jwtData := jwt.MapClaims{"exp": expiresAt, "jti": jti, "iat": time.Now().Unix()}
	maps.Copy(jwtData, data)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtData)

	// Sign and get the complete encoded token as a string using the secret
	tokenString, err := token.SignedString([]byte(base.GetCfg().JwtSecret))
	return tokenString, err
}

// 从 JWT 字符串解析 jti（用于单条/全量吊销），解析失败返回空串。
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

// 从 WebVPN 会话 JWT 解析 webvpn_user（全量登出时定位索引）。
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
