package base

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	LogLevelTrace = iota
	LogLevelDebug
	LogLevelInfo
	LogLevelWarn
	LogLevelError
	LogLevelFatal
)

// 按 key 对日志做限频，避免暴力破解等高频场景下日志被刷爆。
// 同一 key 在 interval 窗口内最多放行一次（首次），其余直接丢弃。
type WarnLimiter struct {
	mu       sync.Mutex
	last     map[string]time.Time
	interval time.Duration
}

// 创建一个限频器，interval 为同一 key 的最小记录间隔。
func NewWarnLimiter(interval time.Duration) *WarnLimiter {
	return &WarnLimiter{last: make(map[string]time.Time), interval: interval}
}

// 返回 true 表示本次允许输出日志（并刷新时间戳）。
func (t *WarnLimiter) Allow(key string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if lt, ok := t.last[key]; ok && now.Sub(lt) < t.interval {
		return false
	}
	t.last[key] = now
	return true
}

// 清除超过 interval 的历史时间戳，防止 last 随不同 key 无限增长。
func (t *WarnLimiter) Clear(now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for k, v := range t.last {
		if now.Sub(v) >= t.interval {
			delete(t.last, k)
		}
	}
}

var (
	baseLwPtr  atomic.Pointer[logWriter]
	baseLogPtr atomic.Pointer[log.Logger]
	baseLevel  atomic.Int32
	levels     = map[int]string{
		LogLevelTrace: "Trace",
		LogLevelDebug: "Debug",
		LogLevelInfo:  "Info",
		LogLevelWarn:  "Warn",
		LogLevelError: "Error",
		LogLevelFatal: "Fatal",
	}

	dateFormat = "2006-01-02"
	logName    = "remlink.log"

	// BroadcastSyslogFunc WebSocket 实时日志推送回调
	BroadcastSyslogFunc func(level int, msg string)
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

// 实现 os.Writer 接口
type logWriter struct {
	mu        sync.Mutex
	UseStdout bool
	FileName  string
	File      *os.File
	NowDate   string
}

// 实现日志文件的切割
func (lw *logWriter) Write(p []byte) (n int, err error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	if lw.UseStdout {
		return lw.File.Write(p)
	}

	date := time.Now().Format(dateFormat)
	if lw.NowDate != date {
		_ = lw.File.Close()
		_ = os.Rename(lw.FileName, lw.FileName+"."+lw.NowDate)
		lw.NowDate = date
		lw.newFile()
	}
	return lw.File.Write(p)
}

// 创建新文件
func (lw *logWriter) newFile() {
	if lw.UseStdout {
		lw.File = os.Stdout
		return
	}

	f, err := os.OpenFile(lw.FileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] 无法打开日志文件 %s: %v，回退到标准输出\n", lw.FileName, err)
		lw.File = os.Stdout
		lw.UseStdout = true
		return
	}
	lw.File = f
}

func initLog() {
	cfg := GetCfg()
	lw := &logWriter{
		UseStdout: cfg.LogPath == "",
		FileName:  path.Join(cfg.LogPath, logName),
		NowDate:   time.Now().Format(dateFormat),
	}

	lw.newFile()
	baseLwPtr.Store(lw)
	baseLevel.Store(int32(logLevel2Int(cfg.LogLevel)))
	baseLogPtr.Store(log.New(lw, "", log.LstdFlags|log.Lshortfile))

	serverLog = log.New(&sLogWriter{}, "[http_server]", log.LstdFlags|log.Lshortfile)
}

func GetBaseLw() *logWriter {
	return baseLwPtr.Load()
}

var serverLog *log.Logger

type sLogWriter struct{}

func (w *sLogWriter) Write(p []byte) (n int, err error) {
	if GetCfg().HttpServerLog {
		return os.Stderr.Write(p)
	}
	return 0, nil
}

// 获取 log.Logger
func GetServerLog() *log.Logger {
	return serverLog
}

func GetLogLevel() int {
	return int(baseLevel.Load())
}

// 获取日志级别的字符串名称
func GetLogLevelName(l int) string {
	if name, ok := levels[l]; ok {
		return name
	}
	return "Unknown"
}

func logLevel2Int(l string) int {
	lvl := LogLevelInfo
	for k, v := range levels {
		if strings.EqualFold(l, v) {
			lvl = k
		}
	}
	return lvl
}

func output(l int, s ...any) {
	lvl := fmt.Sprintf("[%s] ", levels[l])
	msg := fmt.Sprintln(s...)
	_ = baseLogPtr.Load().Output(3, lvl+msg)

	// 实时推送到 WebSocket
	broadcastSyslogIfSet(l, msg)
}

func broadcastSyslogIfSet(l int, msg string) {
	if BroadcastSyslogFunc != nil {
		BroadcastSyslogFunc(l, msg)
	}
}

// 重新初始化日志输出
func ReinitLog() {
	cfg := GetCfg()
	oldLw := baseLwPtr.Load()

	if cfg.LogPath != "" {
		CreateDir(cfg.LogPath)
	}

	newLw := &logWriter{
		UseStdout: cfg.LogPath == "",
		FileName:  path.Join(cfg.LogPath, logName),
		NowDate:   time.Now().Format(dateFormat),
	}
	newLw.newFile()

	baseLwPtr.Store(newLw)
	baseLogPtr.Store(log.New(newLw, "", log.LstdFlags|log.Lshortfile))
	baseLevel.Store(int32(logLevel2Int(cfg.LogLevel)))

	// 持有旧 writer 的锁后安全关闭，确保正在进行的写入已完成
	if oldLw != nil && !oldLw.UseStdout && oldLw.File != nil {
		oldLw.mu.Lock()
		_ = oldLw.File.Close()
		oldLw.mu.Unlock()
	}

	if !newLw.UseStdout {
		Info("日志文件路径已切换: " + newLw.FileName)
	}
}

func Trace(v ...any) {
	l := LogLevelTrace
	if int(baseLevel.Load()) > l {
		return
	}
	output(l, v...)
}

func Debug(v ...any) {
	l := LogLevelDebug
	if int(baseLevel.Load()) > l {
		return
	}
	output(l, v...)
}

func Info(v ...any) {
	l := LogLevelInfo
	if int(baseLevel.Load()) > l {
		return
	}
	output(l, v...)
}

func Warn(v ...any) {
	l := LogLevelWarn
	if int(baseLevel.Load()) > l {
		return
	}
	output(l, v...)
}

func Error(v ...any) {
	l := LogLevelError
	if int(baseLevel.Load()) > l {
		return
	}
	output(l, v...)
}

func Fatal(v ...any) {
	l := LogLevelFatal
	if int(baseLevel.Load()) > l {
		return
	}
	output(l, v...)
	os.Exit(1)
}
