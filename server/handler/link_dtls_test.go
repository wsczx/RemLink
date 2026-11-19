package handler

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/sessdata"
)

type stubAddr string

func (a stubAddr) Network() string { return "udp" }
func (a stubAddr) String() string  { return string(a) }

type stubConn struct{ addr stubAddr }

func (c *stubConn) Read([]byte) (int, error)        { return 0, nil }
func (c *stubConn) Write(b []byte) (int, error)     { return len(b), nil }
func (c *stubConn) Close() error                    { return nil }
func (c *stubConn) LocalAddr() net.Addr             { return c.addr }
func (c *stubConn) RemoteAddr() net.Addr            { return c.addr }
func (c *stubConn) SetDeadline(time.Time) error     { return nil }
func (c *stubConn) SetReadDeadline(time.Time) error { return nil }
func (c *stubConn) SetWriteDeadline(time.Time) error {
	return nil
}

func TestDtlsDecoderDisconnectClosesChannel(t *testing.T) {
	base.Test()
	ast := assert.New(t)
	dSess := &sessdata.DtlsSession{CloseChan: make(chan struct{})}
	cs := &sessdata.ConnSession{CloseChan: make(chan struct{})}

	dec := &dtlsDecoder{sess: cs, conn: &stubConn{addr: stubAddr("1.2.3.4:5678")}, dSess: dSess}
	pl := &sessdata.Payload{Data: []byte{0x05}}

	ast.True(dec.decode(pl, 1), "DISCONNECT 应终止读循环")
	select {
	case <-dSess.CloseChan:
	default:
		t.Fatal("DISCONNECT 应关闭 dSess.CloseChan，否则 LinkDTLS 永久阻塞")
	}
}
