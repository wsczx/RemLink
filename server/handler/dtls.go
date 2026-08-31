package handler

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/pion/dtls/v3"
	"github.com/pion/dtls/v3/pkg/crypto/selfsign"
	"github.com/pion/logging"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/sessdata"
)

// 每个客户端 IP 允许的最大并发 DTLS 握手数。
// 用于防御 DTLS 不可用环境下客户端的重连风暴：避免同一 IP 瞬间打满握手协程 / 耗尽 FD。
const maxDtlsHandshakePerIP = 16

var (
	dtlsHsMux   sync.Mutex
	dtlsHsPerIP = make(map[string]int)
)

// 为某客户端 IP 占一个并发握手，成功返回 true。
func dtlsHandshakeBegin(ip string) bool {
	dtlsHsMux.Lock()
	defer dtlsHsMux.Unlock()
	if dtlsHsPerIP[ip] >= maxDtlsHandshakePerIP {
		return false
	}
	dtlsHsPerIP[ip]++
	return true
}

// 释放一个并发握手，计数归零时清理 map 条目。
func dtlsHandshakeEnd(ip string) {
	dtlsHsMux.Lock()
	defer dtlsHsMux.Unlock()
	if dtlsHsPerIP[ip] > 0 {
		dtlsHsPerIP[ip]--
		if dtlsHsPerIP[ip] == 0 {
			delete(dtlsHsPerIP, ip)
		}
	}
}

func startDtls() {
	if !base.GetCfg().ServerDTLS {
		return
	}

	// rsa 兼容 open connect
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic("DTLS RSA 密钥生成失败: " + err.Error())
	}
	certificate, err := selfsign.SelfSign(priv)
	if err != nil {
		panic(err)
	}

	logf := logging.NewDefaultLoggerFactory()
	logf.Writer = base.GetBaseLw()
	logf.DefaultLogLevel = logging.LogLevelInfo

	// https://github.com/pion/dtls/pull/369
	sessStore := &sessionStore{}

	serverOptions := []dtls.ServerOption{
		dtls.WithSessionStore(sessStore),
		dtls.WithCertificates(certificate),
		dtls.WithExtendedMasterSecret(dtls.DisableExtendedMasterSecret),
		dtls.WithCipherSuites(func() []dtls.CipherSuiteID {
			cs := make([]dtls.CipherSuiteID, 0, len(dtlsCipherSuites))
			for _, vv := range dtlsCipherSuites {
				cs = append(cs, vv)
			}
			return cs
		}()...),
		dtls.WithLoggerFactory(logf),
		dtls.WithMTU(BufferSize),
	}

	addr, err := net.ResolveUDPAddr("udp", base.FormatListenAddr(base.GetCfg().ServerDTLSAddr))
	if err != nil {
		panic(err)
	}
	ln, err := dtls.ListenWithOptions("udp", addr, serverOptions...)
	if err != nil {
		panic(err)
	}

	base.Info("listen DTLS server", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			base.Error("DTLS Accept error", err)
			continue
		}

		// 按客户端 IP 限制并发握手数，挡住重连风暴
		ip := ""
		if ua, ok := conn.RemoteAddr().(*net.UDPAddr); ok {
			ip = ua.IP.String()
		}
		if ip != "" && !dtlsHandshakeBegin(ip) {
			base.Warn("DTLS 握手并发超限，拒绝连接:", ip)
			_ = conn.Close()
			continue
		}

		go func(c net.Conn, ip string) {
			// 兜底：握手协程任何异常（如畸形 ClientHello 触发 panic）都不应泄漏 FD 或拖垮 accept 循环
			defer func() {
				if r := recover(); r != nil {
					base.Error("DTLS 握手协程 panic，已恢复:", r)
				}
				_ = c.Close()
				if ip != "" {
					dtlsHandshakeEnd(ip)
				}
			}()
			cc := c.(*dtls.Conn)
			// v3 的 Accept 不阻塞握手（握手惰性，由首个 Handshake/Read/Write 触发），
			// 必须显式驱动握手完成后再取 ConnectionState，否则拿到空 SessionID 会误关连接
			hctx, hcancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer hcancel()
			if err := cc.HandshakeContext(hctx); err != nil {
				base.Error("DTLS 握手失败", err)
				return
			}
			state, ok := cc.ConnectionState()
			if !ok {
				return
			}
			did := hex.EncodeToString(state.SessionID)
			cSess := sessdata.Dtls2CSess(did)
			if cSess == nil {
				return
			}
			LinkDtls(c, cSess)
		}(conn, ip)
	}
}

// https://github.com/pion/dtls/blob/master/session.go
type sessionStore struct{}

func (ms *sessionStore) Set(key []byte, s dtls.Session) error {
	return nil
}

func (ms *sessionStore) Get(key []byte) (dtls.Session, error) {
	k := hex.EncodeToString(key)
	secret := sessdata.Dtls2MasterSecret(k)
	if secret == "" {
		return dtls.Session{}, errors.New("Dtls2MasterSecret is nil")
	}

	masterSecret, err := hex.DecodeString(secret)
	if err != nil {
		return dtls.Session{}, fmt.Errorf("Dtls2MasterSecret hex 解码失败: %w", err)
	}
	return dtls.Session{ID: key, Secret: masterSecret}, nil
}

func (ms *sessionStore) Del(key []byte) error {
	return nil
}

// 客户端和服务端映射 X-DTLS12-CipherSuite
var dtlsCipherSuites = map[string]dtls.CipherSuiteID{
	// "ECDHE-ECDSA-AES256-GCM-SHA384": dtls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	// "ECDHE-ECDSA-AES128-GCM-SHA256": dtls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	"ECDHE-RSA-AES256-GCM-SHA384": dtls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	"ECDHE-RSA-AES128-GCM-SHA256": dtls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
}

func checkDtls12Ciphersuite(ciphersuite string) string {
	csArr := strings.SplitSeq(ciphersuite, ":")

	for v := range csArr {
		if _, ok := dtlsCipherSuites[v]; ok {
			return v
		}
	}
	// 返回默认值
	return "ECDHE-RSA-AES128-GCM-SHA256"
}
