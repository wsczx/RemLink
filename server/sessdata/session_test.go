package sessdata

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/base"
)

func TestNewSession(t *testing.T) {
	ast := assert.New(t)
	m := NewSessionManager()
	sess := m.Add(&Session{})
	token := sess.Token
	v := m.Token2Sess(token)
	ast.NotNil(v)
	ast.Equal(sess, v)
}

func TestConnSession(t *testing.T) {
	ast := assert.New(t)
	tmp := t.TempDir()
	preData(tmp)
	defer cleardata(tmp)

	m := NewSessionManager()
	sess := m.Add(&Session{})
	sess.Username = "user-test"
	sess.Group = "group1"
	sess.MacAddr = "00:15:5d:50:14:43"

	cSess := sess.NewConn()

	err := cSess.RateLimit(100, true)
	ast.Nil(err)
	ast.Equal(cSess.BandwidthUp.Load(), uint64(100))
	err = cSess.RateLimit(200, false)
	ast.Nil(err)
	ast.Equal(cSess.BandwidthDown.Load(), uint64(200))

	var (
		cmpName string
		ok      bool
	)
	base.SetCfgForTest(&base.ServerConfig{Compression: true})

	cmpName, ok = cSess.SetPickCmp("cstp", "oc-lz4,lzs")
	fmt.Println(cmpName, ok)
	ast.True(ok)
	ast.Equal(cmpName, "lzs")
	cmpName, ok = cSess.SetPickCmp("dtls", "lzs")
	ast.True(ok)
	ast.Equal(cmpName, "lzs")
	cmpName, ok = cSess.SetPickCmp("dtls", "test")
	ast.False(ok)
	ast.Equal(cmpName, "")

	// 验证 Close() 回收三通道残留 Payload，避免会话关闭时池泄漏
	cSess.PayloadIn <- Payloads.GetPayload()
	cSess.PayloadOutCstp <- Payloads.GetPayload()
	cSess.PayloadOutDtls <- Payloads.GetPayload()
	cSess.Close()
	ast.Equal(0, len(cSess.PayloadIn))
	ast.Equal(0, len(cSess.PayloadOutCstp))
	ast.Equal(0, len(cSess.PayloadOutDtls))
}

func TestNewDtlsConnConcurrent(t *testing.T) {
	ast := assert.New(t)
	sess := &Session{IsActive: true}
	cs := &ConnSession{Sess: sess, dSess: &atomic.Value{}}
	cs.dSess.Store(&DtlsSession{isActive: -1})

	var (
		wg     sync.WaitGroup
		passed atomic.Int32
	)
	for range 32 {
		wg.Go(func() {
			if cs.NewDtlsConn() != nil {
				passed.Add(1)
			}
		})
	}
	wg.Wait()

	ast.Equal(int32(1), passed.Load())
	ast.NotNil(cs.GetDtlsSession())
}
