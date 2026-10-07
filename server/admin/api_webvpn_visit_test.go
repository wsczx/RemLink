package admin

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/dbdata"
)

func TestParseVisitQuery(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/v1/webvpn/visits?start=2026-10-01+08:00:00&end=2026-10-02&q=+alice+&protocol=SSH&online=1&has_recording=true", nil)
	vq := parseVisitQuery(r)
	assert.Equal(t, time.Date(2026, 10, 1, 8, 0, 0, 0, time.Local), vq.start)
	assert.Equal(t, time.Date(2026, 10, 2, 0, 0, 0, 0, time.Local), vq.end)
	assert.False(t, vq.endIsNow)
	assert.Equal(t, "alice", vq.q)
	assert.Equal(t, "ssh", vq.protocol)
	assert.True(t, vq.online)
	assert.True(t, vq.hasRec)
	assert.False(t, vq.hasFileOps)

	// 缺省：最近 24 小时，结束为当前
	vq = parseVisitQuery(httptest.NewRequest("GET", "/api/v1/webvpn/visits", nil))
	assert.True(t, vq.endIsNow)
	assert.InDelta(t, visitDefaultRange.Seconds(), vq.end.Sub(vq.start).Seconds(), 1)

	// 起点晚于终点 → 回退默认；跨度超过上限 → 截断
	vq = parseVisitQuery(httptest.NewRequest("GET", "/x?start=2030-01-01&end=2026-01-01", nil))
	assert.InDelta(t, visitDefaultRange.Seconds(), vq.end.Sub(vq.start).Seconds(), 1)
	vq = parseVisitQuery(httptest.NewRequest("GET", "/x?start=2000-01-01&end=2026-01-01", nil))
	assert.Equal(t, visitMaxRange, vq.end.Sub(vq.start))
}

func TestHTTPVisitOnline(t *testing.T) {
	now := time.Now()
	h := &dbdata.WebVpnHTTPVisit{FirstId: 3, LastId: 9, Username: "a", AppName: "oa", ClientIP: "1.2.3.4",
		StartedAt: now.Add(-20 * time.Minute), EndedAt: now.Add(-2 * time.Minute), Requests: 5}
	v := httpVisit(h, now)
	assert.Equal(t, "http:3-9", v.Id)
	assert.True(t, v.Online)
	assert.Equal(t, int64(18*60), v.Duration)
	assert.Equal(t, dbdata.RecReasonHTTP, v.Recording.Reason)

	h.EndedAt = now.Add(-10 * time.Minute)
	assert.False(t, httpVisit(h, now).Online)
	assert.Equal(t, "管理员断开", closeReasonText("kicked"))
	assert.Equal(t, "xyz", closeReasonText("xyz"))
}
