package geoip

import (
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestParseAndPublic(t *testing.T) {
	cases := map[string]bool{
		"8.8.8.8":            true,
		"8.8.8.8:443":        true,
		"[2001:4860::1]:443": true,
		"::ffff:8.8.8.8":     true,
		"10.0.0.1":           false,
		"192.168.1.1:1234":   false,
		"127.0.0.1":          false,
		"100.64.1.1":         false,
		"fe80::1":            false,
		"fd00::1":            false,
		"":                   false,
		"not-an-ip":          false,
	}
	for in, want := range cases {
		a, ok := parseAddr(in)
		got := ok && isPublic(a)
		if got != want {
			t.Errorf("%q: got %v want %v", in, got, want)
		}
	}
}

func TestMissingFileSilentlyDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "none.mmdb")
	d := New(func() string { return path })
	if err := d.Reload(); err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if got := d.Lookup("8.8.8.8"); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
	st := d.Status()
	if st.Enabled || st.FileExists || st.Error != "" {
		t.Fatalf("unexpected status %+v", st)
	}
}

func TestInvalidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.mmdb")
	if err := os.WriteFile(path, []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	d := New(func() string { return path })
	if err := d.Reload(); err == nil {
		t.Fatal("want error for invalid mmdb")
	}
	if got := d.Lookup("8.8.8.8"); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
	if st := d.Status(); st.Enabled || st.Error == "" {
		t.Fatalf("unexpected status %+v", st)
	}
	if _, err := Validate(path); err == nil {
		t.Fatal("Validate should fail")
	}
}

func TestUninitializedDefault(t *testing.T) {
	if Lookup("8.8.8.8") != "" {
		t.Fatal("uninitialized default must return empty")
	}
}

func TestLRU(t *testing.T) {
	c := newLRU(2)
	a, b, x := netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("2.2.2.2"), netip.MustParseAddr("3.3.3.3")
	c.put(a, "AU")
	c.put(b, "US")
	c.get(a) // a 变为最近使用
	c.put(x, "CN")
	if _, ok := c.get(b); ok {
		t.Fatal("b should be evicted")
	}
	if v, ok := c.get(a); !ok || v != "AU" {
		t.Fatal("a should remain")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				c.put(a, "AU")
				c.get(x)
			}
		}()
	}
	wg.Wait()
}

// 设置 GEOIP_TEST_DB 指向真实 Country mmdb 时做端到端验证
func TestRealDB(t *testing.T) {
	path := os.Getenv("GEOIP_TEST_DB")
	if path == "" {
		t.Skip("GEOIP_TEST_DB not set")
	}
	d := New(func() string { return path })
	if err := d.Reload(); err != nil {
		t.Fatal(err)
	}
	if got := d.Lookup("8.8.8.8"); got != "US" {
		t.Fatalf("8.8.8.8 => %q", got)
	}
	if _, err := Validate(path); err != nil {
		t.Fatal(err)
	}
}
