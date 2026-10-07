package dbdata

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func auditAt(id int64, user, app, ip string, t time.Time, status int) WebVpnAudit {
	return WebVpnAudit{Id: id, Username: user, AppName: app, ClientIP: ip, CreatedAt: t, StatusCode: status,
		BytesSent: 100, BytesRecv: 10, Method: "GET", Path: "/", Host: app + ".wv"}
}

func TestAggregateWebVpnHTTPVisits(t *testing.T) {
	base := time.Date(2026, 10, 1, 9, 0, 0, 0, time.Local)
	rows := []WebVpnAudit{
		// alice@oa 同一 IP：0、10、40 分钟（间隔 30 分钟内）→ 一次访问；100 分钟（间隔 60 分钟）→ 新访问
		auditAt(1, "alice", "oa", "1.1.1.1", base, 200),
		auditAt(3, "alice", "oa", "1.1.1.1", base.Add(10*time.Minute), 404),
		auditAt(5, "alice", "oa", "1.1.1.1", base.Add(40*time.Minute), 200),
		auditAt(9, "alice", "oa", "1.1.1.1", base.Add(100*time.Minute), 500),
		// 同用户同应用不同 IP → 单独访问
		auditAt(2, "alice", "oa", "2.2.2.2", base.Add(time.Minute), 200),
		// 同用户不同应用 → 单独访问
		auditAt(4, "alice", "wiki", "1.1.1.1", base.Add(5*time.Minute), 200),
		// 恰好间隔 30 分钟 → 仍合并
		auditAt(6, "bob", "oa", "3.3.3.3", base, 200),
		auditAt(7, "bob", "oa", "3.3.3.3", base.Add(30*time.Minute), 200),
	}
	// 输入乱序不影响结果
	rows[0], rows[3] = rows[3], rows[0]
	vs := AggregateWebVpnHTTPVisits(rows, WebVpnHTTPVisitGap)
	require.Len(t, vs, 5)

	// 按开始时间倒序
	for i := 1; i < len(vs); i++ {
		assert.False(t, vs[i].StartedAt.After(vs[i-1].StartedAt))
	}
	find := func(user, app, ip string, start time.Time) *WebVpnHTTPVisit {
		for i := range vs {
			v := &vs[i]
			if v.Username == user && v.AppName == app && v.ClientIP == ip && v.StartedAt.Equal(start) {
				return v
			}
		}
		return nil
	}
	a := find("alice", "oa", "1.1.1.1", base)
	require.NotNil(t, a)
	assert.Equal(t, 3, a.Requests)
	assert.Equal(t, 1, a.Errors)
	assert.Equal(t, int64(1), a.FirstId)
	assert.Equal(t, int64(5), a.LastId)
	assert.Equal(t, "http:1-5", a.ID())
	assert.Equal(t, base.Add(40*time.Minute), a.EndedAt)
	assert.Equal(t, int64(300), a.BytesOut)
	assert.Equal(t, int64(30), a.BytesIn)

	a2 := find("alice", "oa", "1.1.1.1", base.Add(100*time.Minute))
	require.NotNil(t, a2)
	assert.Equal(t, 1, a2.Requests)
	assert.Equal(t, 1, a2.Errors)

	assert.NotNil(t, find("alice", "oa", "2.2.2.2", base.Add(time.Minute)))
	assert.NotNil(t, find("alice", "wiki", "1.1.1.1", base.Add(5*time.Minute)))
	b := find("bob", "oa", "3.3.3.3", base)
	require.NotNil(t, b)
	assert.Equal(t, 2, b.Requests)

	assert.Empty(t, AggregateWebVpnHTTPVisits(nil, WebVpnHTTPVisitGap))
}

func TestWebVpnRecordingOf(t *testing.T) {
	yes := func(string) bool { return true }
	no := func(string) bool { return false }
	cases := []struct {
		name   string
		s      WebVpnSession
		exists func(string) bool
		avail  bool
		reason string
		format string
	}{
		{"ssh 已录像", WebVpnSession{Protocol: "ssh", RecordMode: "on", RecordingPath: "20261001/a.cast"}, yes, true, RecReasonRecorded, "asciicast"},
		{"rdp 已录像", WebVpnSession{Protocol: "rdp", RecordMode: "on", RecordingPath: "20261001/a.guac"}, yes, true, RecReasonRecorded, "guac"},
		{"录像文件被清理", WebVpnSession{Protocol: "ssh", RecordMode: "on", RecordingPath: "20261001/a.cast"}, no, false, RecReasonFileMissing, "asciicast"},
		{"应用未开启录像", WebVpnSession{Protocol: "ssh", RecordMode: "off"}, yes, false, RecReasonDisabled, "asciicast"},
		{"vnc 未开启录像", WebVpnSession{Protocol: "vnc", RecordMode: "off"}, yes, false, RecReasonDisabled, "guac"},
		{"开启但未打开终端", WebVpnSession{Protocol: "ssh", RecordMode: "on"}, yes, false, RecReasonNoTerminal, "asciicast"},
		{"开启但录像失败", WebVpnSession{Protocol: "ssh", RecordMode: "on", RecordErr: "permission denied"}, yes, false, RecReasonFailed, "asciicast"},
		{"sftp", WebVpnSession{Protocol: "sftp", RecordMode: "na"}, yes, false, RecReasonFileProtocol, ""},
		{"smb 旧数据", WebVpnSession{Protocol: "smb"}, yes, false, RecReasonFileProtocol, ""},
		{"ftp 旧数据", WebVpnSession{Protocol: "ftp"}, yes, false, RecReasonFileProtocol, ""},
		{"ssh 旧数据无路径", WebVpnSession{Protocol: "ssh"}, yes, false, RecReasonUnknown, "asciicast"},
		{"ssh 旧数据有路径", WebVpnSession{Protocol: "ssh", RecordingPath: "x/a.cast"}, yes, true, RecReasonRecorded, "asciicast"},
		{"rdp 旧数据路径文件缺失", WebVpnSession{Protocol: "rdp", RecordingPath: "x/a.guac"}, no, false, RecReasonFileMissing, "guac"},
		{"http", WebVpnSession{Protocol: "http"}, yes, false, RecReasonHTTP, ""},
		{"不检查文件", WebVpnSession{Protocol: "ssh", RecordMode: "on", RecordingPath: "x/a.cast"}, nil, true, RecReasonRecorded, "asciicast"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := WebVpnRecordingOf(&c.s, c.exists)
			assert.Equal(t, c.avail, r.Available)
			assert.Equal(t, c.reason, r.Reason)
			assert.Equal(t, c.format, r.Format)
		})
	}
	r := WebVpnRecordingOf(&WebVpnSession{Protocol: "ssh", RecordMode: "on", RecordErr: "disk full"}, yes)
	assert.Equal(t, "disk full", r.Error)
}

func TestWebVpnRecordModeOf(t *testing.T) {
	assert.Equal(t, WebVpnRecordOn, WebVpnRecordModeOf(&WebVpnApp{Protocol: "ssh", Record: true}))
	assert.Equal(t, WebVpnRecordOff, WebVpnRecordModeOf(&WebVpnApp{Protocol: "rdp"}))
	assert.Equal(t, WebVpnRecordNA, WebVpnRecordModeOf(&WebVpnApp{Protocol: "sftp", Record: true}))
	assert.Equal(t, WebVpnRecordNA, WebVpnRecordModeOf(&WebVpnApp{Protocol: ""}))
}

func TestWebVpnRecordingParts(t *testing.T) {
	files := map[string]bool{"d/s.guac": true, "d/s.2.guac": true, "d/s.3.guac": true, "d/s.5.guac": true, "d/c.cast": true}
	ex := func(p string) bool { return files[p] }
	assert.Equal(t, 3, WebVpnRecordingParts("d/s.guac", ex))
	assert.Equal(t, 1, WebVpnRecordingParts("d/c.cast", ex))
	assert.Equal(t, 0, WebVpnRecordingParts("d/x.guac", ex))
	assert.Equal(t, 0, WebVpnRecordingParts("", ex))
}

func TestWebVpnVisitDB(t *testing.T) {
	preIpData(t)
	defer closeIpdata()

	now := time.Now().Truncate(time.Second)
	t0 := now.Add(-3 * time.Hour)
	var rows []WebVpnAudit
	// carol@oa：t0 起每 10 分钟一次，共 4 次（一次访问）；2 小时后再 2 次（第二次访问）
	for i := 0; i < 4; i++ {
		rows = append(rows, auditAt(0, "carol", "oa", "10.0.0.1", t0.Add(time.Duration(i)*10*time.Minute), 200))
	}
	rows = append(rows, auditAt(0, "dave", "wiki", "10.0.0.2", t0.Add(5*time.Minute), 403))
	rows = append(rows, auditAt(0, "carol", "oa", "10.0.0.1", t0.Add(150*time.Minute), 200))
	rows = append(rows, auditAt(0, "carol", "oa", "10.0.0.1", t0.Add(160*time.Minute), 500))
	// 窗口外
	rows = append(rows, auditAt(0, "carol", "oa", "10.0.0.1", now.Add(-48*time.Hour), 200))
	for i := range rows {
		_, err := xdb.NoAutoTime().InsertOne(&rows[i])
		require.NoError(t, err)
	}

	vs, truncated, err := ScanWebVpnHTTPVisits(now.Add(-24*time.Hour), now, "", 0)
	require.NoError(t, err)
	assert.False(t, truncated)
	require.Len(t, vs, 3)
	assert.Equal(t, "carol", vs[0].Username)
	assert.Equal(t, 2, vs[0].Requests)
	assert.Equal(t, 1, vs[0].Errors)

	// 模糊搜索
	vs2, _, err := ScanWebVpnHTTPVisits(now.Add(-24*time.Hour), now, "wik", 0)
	require.NoError(t, err)
	require.Len(t, vs2, 1)
	assert.Equal(t, "dave", vs2[0].Username)

	// 扫描上限：只保留最新的 3 条请求
	vs3, truncated, err := ScanWebVpnHTTPVisits(now.Add(-24*time.Hour), now, "", 3)
	require.NoError(t, err)
	assert.True(t, truncated)
	total := 0
	for _, v := range vs3 {
		total += v.Requests
	}
	assert.Equal(t, 3, total)

	// 按 ID 还原访问与请求列表
	var first *WebVpnHTTPVisit
	for i := range vs {
		if vs[i].Username == "carol" && vs[i].Requests == 4 {
			first = &vs[i]
		}
	}
	require.NotNil(t, first)
	got, err := GetWebVpnHTTPVisit(first.FirstId, first.LastId)
	require.NoError(t, err)
	assert.Equal(t, 4, got.Requests)
	assert.Equal(t, int64(400), got.BytesOut)
	assert.Equal(t, 0, got.Errors)
	assert.True(t, got.StartedAt.Equal(first.StartedAt))
	assert.True(t, got.EndedAt.Equal(first.EndedAt))
	reqs, err := WebVpnHTTPVisitRequests(got, 1, 3)
	require.NoError(t, err)
	require.Len(t, reqs, 3)
	assert.True(t, reqs[0].Id < reqs[1].Id)
	for _, r := range reqs {
		assert.Equal(t, "carol", r.Username) // 区间内 dave 的请求被排除
	}
	_, err = GetWebVpnHTTPVisit(99999, 99999)
	assert.Error(t, err)

	// 会话与文件操作计数
	require.NoError(t, AddWebVpnSession(&WebVpnSession{Sid: "s1", Username: "carol", AppName: "box", Protocol: "ssh",
		Target: "h:22", StartedAt: now.Add(-time.Hour), RecordMode: WebVpnRecordOn}))
	require.NoError(t, AddWebVpnSession(&WebVpnSession{Sid: "s2", Username: "erin", AppName: "files", Protocol: "sftp",
		Target: "h:22", StartedAt: now.Add(-72 * time.Hour), RecordMode: WebVpnRecordNA}))
	require.NoError(t, EndWebVpnSession("s2", 1, 2, "idle", ""))
	for _, op := range []string{"list", "upload", "delete"} {
		require.NoError(t, AddWebVpnFileLog(&WebVpnFileLog{Sid: "s1", Username: "carol", AppName: "box", Op: op, Path: "/a", Result: "ok"}))
	}
	ss, err := ListWebVpnSessionsForVisits(now.Add(-24*time.Hour), now, "", "", 0)
	require.NoError(t, err)
	require.Len(t, ss, 1) // s2 在窗口外且已结束
	assert.Equal(t, "s1", ss[0].Sid)
	cnt, err := WebVpnFileOpCounts([]string{"s1", "s2"})
	require.NoError(t, err)
	assert.Equal(t, 2, cnt["s1"]) // 不含 list
	assert.Equal(t, 0, cnt["s2"])
	n, err := WebVpnFileOpCountSince(now.Add(-time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)

	// 录像路径尽早写入；结束时不覆盖
	require.NoError(t, SetWebVpnSessionRecording("s1", "d/s1.cast"))
	require.NoError(t, SetWebVpnSessionRecording("s1", "d/other.cast"))
	require.NoError(t, EndWebVpnSession("s1", 5, 6, "user_close", ""))
	s1, err := GetWebVpnSession("s1")
	require.NoError(t, err)
	assert.Equal(t, "d/s1.cast", s1.RecordingPath)
	assert.Equal(t, WebVpnRecordOn, s1.RecordMode)
	require.NoError(t, SetWebVpnSessionRecordErr("s1", "boom"))
	s1, _ = GetWebVpnSession("s1")
	assert.Equal(t, "boom", s1.RecordErr)
}
