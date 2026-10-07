package ticket

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTicketSingleUse(t *testing.T) {
	s := NewStore(30 * time.Second)
	c := Claims{Sid: "s1", Username: "alice", App: "ssh1", ClientIP: "10.0.0.1", Purpose: "term"}
	tk, err := s.Issue(c)
	if err != nil || len(tk) < 40 {
		t.Fatalf("签发失败: %q %v", tk, err)
	}
	got, err := s.Consume(tk, c)
	if err != nil || got != c {
		t.Fatalf("首次兑换应成功: %+v %v", got, err)
	}
	if _, err := s.Consume(tk, c); !errors.Is(err, ErrInvalid) {
		t.Fatalf("二次兑换必须失败: %v", err)
	}
}

func TestTicketExpire(t *testing.T) {
	s := NewStore(30 * time.Second)
	now := time.Now()
	s.now = func() time.Time { return now }
	tk, _ := s.Issue(Claims{Sid: "s1"})
	s.now = func() time.Time { return now.Add(31 * time.Second) }
	if _, err := s.Consume(tk, Claims{}); err == nil {
		t.Fatal("过期票据必须失败")
	}
	if s.Len() != 0 {
		t.Fatal("过期票据应被清理")
	}
}

func TestTicketMismatchBurns(t *testing.T) {
	s := NewStore(0)
	c := Claims{Sid: "s1", Username: "alice", App: "ssh1", ClientIP: "10.0.0.1", Purpose: "term"}
	for _, bad := range []Claims{
		{Username: "bob"}, {App: "other"}, {ClientIP: "10.0.0.2"}, {Sid: "s2"}, {Purpose: "fs"},
	} {
		tk, _ := s.Issue(c)
		if _, err := s.Consume(tk, bad); !errors.Is(err, ErrMismatch) {
			t.Errorf("不匹配 %+v 应失败: %v", bad, err)
		}
		// 不匹配的尝试同样作废票据，防止枚举
		if _, err := s.Consume(tk, c); !errors.Is(err, ErrInvalid) {
			t.Errorf("不匹配后票据应已作废: %v", err)
		}
	}
	if _, err := s.Consume("", c); err == nil {
		t.Error("空票据应失败")
	}
	if _, err := s.Consume("forged", c); err == nil {
		t.Error("伪造票据应失败")
	}
}

func TestTicketConcurrentConsumeOnce(t *testing.T) {
	s := NewStore(0)
	tk, _ := s.Issue(Claims{Sid: "s1"})
	var ok atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Consume(tk, Claims{Sid: "s1"}); err == nil {
				ok.Add(1)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 1 {
		t.Fatalf("并发兑换只能成功一次，实际 %d", ok.Load())
	}
}
