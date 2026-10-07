package gateway

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

const maxRecordBytes = 512 << 20 // 单会话录像上限，超出后停止记录

// asciicast v2 录像（仅记录输出与窗口变化，不记录键盘输入，避免明文口令落盘）
// https://docs.asciinema.org/manual/asciicast/v2/
type recorder struct {
	mu      sync.Mutex
	f       *os.File
	w       *bufio.Writer
	start   time.Time
	carry   []byte // 跨块未完整的 UTF-8 尾字节
	written int64
	stopped bool
	ticker  *time.Ticker
	quit    chan struct{}
}

func newRecorder(full string, cols, rows int, title string) (*recorder, error) {
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(full, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	r := &recorder{f: f, w: bufio.NewWriterSize(f, 64<<10), start: time.Now(), quit: make(chan struct{})}
	hdr, _ := json.Marshal(map[string]any{
		"version":   2,
		"width":     cols,
		"height":    rows,
		"timestamp": r.start.Unix(),
		"title":     title,
		"env":       map[string]string{"TERM": "xterm-256color"},
	})
	r.writeLine(hdr)
	r.ticker = time.NewTicker(2 * time.Second)
	go func() {
		for {
			select {
			case <-r.ticker.C:
				r.mu.Lock()
				if !r.stopped {
					_ = r.w.Flush()
				}
				r.mu.Unlock()
			case <-r.quit:
				return
			}
		}
	}()
	return r, nil
}

func (r *recorder) writeLine(b []byte) {
	if r.stopped {
		return
	}
	n, _ := r.w.Write(b)
	_ = r.w.WriteByte('\n')
	r.written += int64(n + 1)
	if r.written > maxRecordBytes {
		r.stopped = true
		_ = r.w.Flush()
		base.Warn("WebVPN 录像超过上限，停止记录:", r.f.Name())
	}
}

func (r *recorder) event(kind, data string) {
	t := time.Since(r.start).Seconds()
	b, _ := json.Marshal([]any{float64(int64(t*1e6)) / 1e6, kind, data})
	r.writeLine(b)
}

// 记录输出：处理跨块截断的多字节 UTF-8 字符
func (r *recorder) Output(p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return
	}
	buf := append(r.carry, p...)
	cut := len(buf)
	// 回退至多 3 字节，找出末尾不完整的 UTF-8 序列
	for i := 1; i <= 3 && i <= len(buf); i++ {
		c := buf[len(buf)-i]
		if utf8.RuneStart(c) {
			if !utf8.FullRune(buf[len(buf)-i:]) {
				cut = len(buf) - i
			}
			break
		}
	}
	r.carry = append([]byte(nil), buf[cut:]...)
	if cut > 0 {
		r.event("o", string(buf[:cut]))
	}
}

func (r *recorder) Resize(cols, rows int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.event("r", strconv.Itoa(cols)+"x"+strconv.Itoa(rows))
}

func (r *recorder) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return
	}
	r.ticker.Stop()
	close(r.quit)
	if len(r.carry) > 0 && !r.stopped {
		r.event("o", string(r.carry))
	}
	_ = r.w.Flush()
	_ = r.f.Close()
	r.f = nil
	r.stopped = true
}

// 终端首次附着时按需创建录像（同一会话的多次附着追加到同一文件）
func (s *rsession) recorderFor(cols, rows int) *recorder {
	if !s.app.Record {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rec != nil {
		s.rec.Resize(cols, rows)
		return s.rec
	}
	rel := filepath.ToSlash(filepath.Join(time.Now().Format("20060102"), s.ls.Sid+".cast"))
	full, err := dbdata.WebVpnRecordFullPath(rel)
	if err == nil {
		var rec *recorder
		if rec, err = newRecorder(full, cols, rows, s.ls.Username+"@"+s.ls.AppName); err == nil {
			s.rec = rec
			s.recPath = rel
			go s.markRecording(rel)
			return rec
		}
	}
	base.Error("WebVPN 创建录像失败:", err)
	if !s.recFailed {
		s.recFailed = true
		go s.markRecordErr(err.Error())
	}
	return nil
}

// 录像状态落库（异步，避免持锁写库）：管理端据此区分「已录像 / 未打开终端 / 录像失败」
func (s *rsession) markRecording(rel string) {
	if err := dbdata.SetWebVpnSessionRecording(s.ls.Sid, rel); err != nil {
		base.Warn("WebVPN 录像路径写入失败:", s.ls.Sid, err)
	}
}

func (s *rsession) markRecordErr(msg string) {
	if err := dbdata.SetWebVpnSessionRecordErr(s.ls.Sid, msg); err != nil {
		base.Warn("WebVPN 录像失败原因写入失败:", s.ls.Sid, err)
	}
}
