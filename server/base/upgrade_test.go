package base

import (
	"testing"
)

func TestCompareVersion(t *testing.T) {
	tests := []struct {
		v1, v2 string
		want   int
	}{
		{"0.14.3", "0.14.3", 0},
		{"v0.14.3", "0.14.3", 0},

		{"0.15.1", "0.14.3", 1},
		{"0.15.1-beta", "0.14.3", 1},
		{"1.0.0", "0.15.1", 1},
		{"0.15.2", "0.15.1", 1},
		{"0.10.0", "0.9.0", 1},

		{"0.14.3", "0.15.1", -1},
		{"0.14.3", "0.15.1-beta", -1},
		{"0.15.1-alpha", "0.15.1-beta", -1},

		{"0.15.1-beta", "0.15.1", -1}, // 预发布 < 正式版
		{"0.15.1", "0.15.1-beta", 1},  // 正式版 > 预发布
	}

	for _, tt := range tests {
		got := compareVersion(tt.v1, tt.v2)
		if got != tt.want {
			t.Errorf("compareVersion(%q, %q) = %d, want %d", tt.v1, tt.v2, got, tt.want)
		}
	}
}
