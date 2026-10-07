package live

import (
	"sync"
	"sync/atomic"
	"testing"
)

func newS(sid, user, app string, closed *atomic.Int32, reasons *sync.Map) *Session {
	s := &Session{Sid: sid, Username: user, AppName: app}
	s.CloseFunc = func(reason string) {
		closed.Add(1)
		reasons.Store(sid, reason)
	}
	return s
}

func TestRegistryLimitAndKick(t *testing.T) {
	r := NewRegistry()
	var closed atomic.Int32
	var reasons sync.Map
	if err := r.Register(newS("a1", "alice", "ssh1", &closed, &reasons), 2); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(newS("a2", "alice", "ssh1", &closed, &reasons), 2); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(newS("a3", "alice", "ssh1", &closed, &reasons), 2); err != ErrTooManySessions {
		t.Fatalf("应超过上限: %v", err)
	}
	// 其它应用不受该应用上限影响
	if err := r.Register(newS("a4", "alice", "sftp1", &closed, &reasons), 2); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(newS("b1", "bob", "ssh1", &closed, &reasons), 2); err != nil {
		t.Fatal(err)
	}
	if got := len(r.List("alice", "")); got != 3 {
		t.Fatalf("alice 会话数=%d", got)
	}
	if n := r.KickApp("ssh1", "alice", ReasonKicked); n != 2 {
		t.Fatalf("KickApp=%d", n)
	}
	if n := r.KickUser("alice", ReasonRevoked); n != 1 {
		t.Fatalf("KickUser=%d", n)
	}
	if v, _ := reasons.Load("a4"); v != ReasonRevoked {
		t.Fatalf("关闭原因错误: %v", v)
	}
	if r.Count() != 1 || r.Get("b1") == nil {
		t.Fatal("bob 的会话不应受影响")
	}
	// 重复关闭只回调一次
	s := r.Get("b1")
	if !r.Close(s, ReasonIdle) || r.Close(s, ReasonIdle) {
		t.Fatal("Close 应只成功一次")
	}
	if closed.Load() != 4 {
		t.Fatalf("CloseFunc 调用次数=%d", closed.Load())
	}
	if r.Count() != 0 {
		t.Fatal("注册表应为空")
	}
}

func TestRegistryRevalidate(t *testing.T) {
	r := NewRegistry()
	var closed atomic.Int32
	var reasons sync.Map
	_ = r.Register(newS("x1", "u1", "app", &closed, &reasons), 0)
	_ = r.Register(newS("x2", "u2", "app", &closed, &reasons), 0)
	n := r.Revalidate(func(s *Session) bool { return s.AppName == "app" }, func(s *Session) (bool, string) {
		if s.Username == "u1" {
			return false, ReasonUnauthorized
		}
		return true, ""
	})
	if n != 1 || r.Get("x1") != nil || r.Get("x2") == nil {
		t.Fatal("复核应只断开未授权会话")
	}
}

func TestRegistryConcurrentLimit(t *testing.T) {
	r := NewRegistry()
	var ok atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := &Session{Sid: string(rune('A'+i)) + "-sid", Username: "u", AppName: "a"}
			if r.Register(s, 3) == nil {
				ok.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if ok.Load() != 3 {
		t.Fatalf("并发注册上限失效: %d", ok.Load())
	}
}

func TestIPAllowed(t *testing.T) {
	cases := []struct {
		ip   string
		list []string
		want bool
	}{
		{"10.0.0.5", nil, true},
		{"10.0.0.5", []string{"10.0.0.0/24"}, true},
		{"10.0.1.5", []string{"10.0.0.0/24"}, false},
		{"192.168.1.1", []string{" 192.168.1.1 "}, true},
		{"bad", []string{"10.0.0.0/8"}, false},
		{"fd00::1", []string{"fd00::/64"}, true},
	}
	for _, c := range cases {
		if got := IPAllowed(c.ip, c.list); got != c.want {
			t.Errorf("IPAllowed(%s,%v)=%v", c.ip, c.list, got)
		}
	}
}
