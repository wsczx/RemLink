// 在线升级：通过 GitHub Releases 检查更新、下载、替换、重启
package base

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	upgradeRepoOwner = "wsczx"
	upgradeRepoName  = "RemLink"
)

type ReleaseInfo struct {
	Version       string `json:"version"`
	Body          string `json:"body"`
	URL           string `json:"url"`
	UpgradeSource string `json:"upgrade_source"`
	Size          int64  `json:"size"`
	PublishedAt   string `json:"published_at"`
}

type UpgradeProgress struct {
	Stage      string `json:"stage"`
	Progress   int    `json:"progress"`
	Downloaded int64  `json:"downloaded"`
	Total      int64  `json:"total"`
	Error      string `json:"error,omitempty"`
}

type githubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Body        string        `json:"body"`
	PublishedAt string        `json:"published_at"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type UpgradeStatus struct {
	Running  bool            `json:"running"`
	Progress UpgradeProgress `json:"progress"`
}

var (
	upgradeRunning  atomic.Bool
	upgradeProgress UpgradeProgress
	upgradeProgMux  sync.RWMutex
)

// 检查最新版本，返回 ReleaseInfo、是否需要更新
func CheckUpdate(source string) (*ReleaseInfo, bool, error) {
	var apiURL string
	switch source {
	case "github":
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", upgradeRepoOwner, upgradeRepoName)
	default: // gitee
		apiURL = fmt.Sprintf("https://gitee.com/api/v5/repos/%s/%s/releases/latest", upgradeRepoOwner, upgradeRepoName)
		source = "gitee"
	}
	ri, need, err := getLatestRelease(apiURL)
	if err != nil {
		return nil, false, fmt.Errorf("更新源 %s 不可用: %w", source, err)
	}
	ri.UpgradeSource = source
	return ri, need, nil
}

// 从指定 releases/latest API 获取最新版本信息
func getLatestRelease(apiURL string) (*ReleaseInfo, bool, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, false, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "RemLink-Upgrade")

	resp, err := client.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("访问更新源失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("更新源返回状态码: %d", resp.StatusCode)
	}

	var gr githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return nil, false, fmt.Errorf("解析更新源数据失败: %w", err)
	}

	// asset 格式: remlink-{os}-{arch}
	currentOS := runtime.GOOS
	currentArch := runtime.GOARCH

	var downloadAsset *githubAsset
	for i := range gr.Assets {
		a := &gr.Assets[i]
		name := strings.ToLower(a.Name)
		if strings.Contains(name, currentOS) && strings.Contains(name, currentArch) && !strings.HasSuffix(name, ".sha256") {
			downloadAsset = &gr.Assets[i]
			break
		}
	}

	if downloadAsset == nil {
		return nil, false, fmt.Errorf("未找到适用于当前平台 %s-%s 的下载资源", currentOS, currentArch)
	}

	latestVersion := strings.TrimPrefix(gr.TagName, "v")
	currentVersion := APP_VER
	needUpgrade := compareVersion(latestVersion, currentVersion) > 0

	ri := &ReleaseInfo{
		Version:     gr.TagName,
		Body:        gr.Body,
		URL:         downloadAsset.BrowserDownloadURL,
		Size:        downloadAsset.Size,
		PublishedAt: gr.PublishedAt,
	}
	return ri, needUpgrade, nil
}

// 执行升级：下载 → 替换 → 重启
func DoUpgrade(info *ReleaseInfo, progressCh chan<- UpgradeProgress) {
	defer close(progressCh)

	if upgradeRunning.Load() {
		progressCh <- UpgradeProgress{Stage: "error", Error: "已有升级任务在运行"}
		return
	}
	upgradeRunning.Store(true)
	defer func() { upgradeRunning.Store(false) }()

	// 阶段1：下载（更新源由用户配置决定，无备用源回退）
	sendProgress(progressCh, "downloading", 0, 0, info.Size)
	Info("在线升级：从更新源 ", strings.ToUpper(info.UpgradeSource), " 开始下载 (", info.URL, ")")
	binFile, err := downloadBinary(info.URL, info.Size, progressCh)
	if err == nil {
		Info("在线升级：下载完成")
	}
	if err != nil {
		sendError(progressCh, "下载失败: "+err.Error())
		return
	}
	defer os.Remove(binFile)

	// 阶段2：替换
	sendProgress(progressCh, "replacing", 98, info.Size, info.Size)
	exePath, err := replaceBinary(binFile)
	if err != nil {
		sendError(progressCh, "替换二进制文件失败: "+err.Error())
		return
	}

	// 阶段3：重启
	sendProgress(progressCh, "restarting", 100, info.Size, info.Size)
	sendProgress(progressCh, "done", 100, info.Size, info.Size)

	time.Sleep(500 * time.Millisecond)
	// 关闭 tun/tap/macvtap 设备 fd，避免泄漏到新进程（详见 restart.go）
	closeDeviceFDs()
	if err := syscall.Exec(exePath, os.Args, os.Environ()); err != nil {
		Error("在线升级重启失败:", err)
	}
}

// 下载二进制到临时文件，通过 progressCh 推送进度
func downloadBinary(url string, totalSize int64, progressCh chan<- UpgradeProgress) (string, error) {
	client := &http.Client{
		Timeout: 15 * time.Minute,
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
		},
	}

	var resp *http.Response
	var err error
	for retry := range 2 {
		req, reqErr := http.NewRequest("GET", url, nil)
		if reqErr != nil {
			return "", fmt.Errorf("创建下载请求失败: %w", reqErr)
		}
		req.Header.Set("User-Agent", "RemLink-Upgrade")

		resp, err = client.Do(req)
		if err == nil && resp != nil && resp.StatusCode == http.StatusOK {
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		if retry < 1 {
			time.Sleep(2 * time.Second)
		}
	}
	if err != nil {
		return "", fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载返回状态码: %d", resp.StatusCode)
	}

	tmpDir := os.TempDir()
	if free, err := diskFree(tmpDir); err == nil && totalSize > 0 {
		if free < totalSize+50*1024*1024 {
			return "", fmt.Errorf("磁盘空间不足，需要 %s，剩余 %s", humanSize(totalSize), humanSize(free))
		}
	}

	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("remlink_upgrade_%d", time.Now().Unix()))
	f, err := os.Create(tmpFile)
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer f.Close()

	buf := make([]byte, 32*1024)
	var downloaded int64
	lastReport := time.Now()

	total := resp.ContentLength
	if total <= 0 {
		total = totalSize
	}

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				return "", fmt.Errorf("写入临时文件失败: %w", writeErr)
			}
			downloaded += int64(n)

			if time.Since(lastReport) > 200*time.Millisecond {
				sendProgress(progressCh, "downloading", calcPercent(downloaded, total), downloaded, total)
				lastReport = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", fmt.Errorf("下载过程出错: %w", readErr)
		}
	}

	sendProgress(progressCh, "downloading", 100, downloaded, total)
	Info("在线升级: 下载完成 ", tmpFile, " 大小:", humanSize(downloaded))
	return tmpFile, nil
}

// 复制到同名临时文件 → rename 替换 → 备份旧文件
func replaceBinary(newFile string) (string, error) {
	app, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("获取当前可执行文件路径失败: %w", err)
	}

	bakFile := app + ".bak"
	if err := copyFile(app, bakFile); err != nil {
		Warn("备份旧二进制失败:", err)
	} else {
		Info("在线升级: 旧二进制已备份到 ", bakFile)
	}

	tmpDest := app + ".new"
	if err := copyFile(newFile, tmpDest); err != nil {
		return app, fmt.Errorf("复制新文件失败: %w", err)
	}

	if err := os.Rename(tmpDest, app); err != nil {
		os.Remove(tmpDest)
		return app, fmt.Errorf("替换二进制文件失败: %w", err)
	}

	if err := os.Chmod(app, 0755); err != nil {
		Warn("设置执行权限失败:", err)
	}

	Info("在线升级: 二进制替换完成")
	return app, nil
}

func copyFile(src, dst string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer s.Close()

	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer d.Close()

	if _, err := io.Copy(d, s); err != nil {
		return err
	}
	return d.Sync()
}

// 所在磁盘可用空间
func diskFree(path string) (int64, error) {
	var stat sysStatfs
	if err := statfs(path, &stat); err != nil {
		return 0, err
	}
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}

type sysStatfs struct {
	Bsize  uint64
	Bavail uint64
}

// 字节数转为可读格式
func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func sendProgress(ch chan<- UpgradeProgress, stage string, progress int, downloaded, total int64) {
	p := UpgradeProgress{
		Stage:      stage,
		Progress:   progress,
		Downloaded: downloaded,
		Total:      total,
	}
	upgradeProgMux.Lock()
	upgradeProgress = p
	upgradeProgMux.Unlock()
	select {
	case ch <- p:
	default:
	}
}

func sendError(ch chan<- UpgradeProgress, errMsg string) {
	Error("在线升级: ", errMsg)
	p := UpgradeProgress{
		Stage: "error",
		Error: errMsg,
	}
	upgradeProgMux.Lock()
	upgradeProgress = p
	upgradeProgMux.Unlock()
	select {
	case ch <- p:
	default:
	}
}

func calcPercent(downloaded, total int64) int {
	if total <= 0 {
		return 0
	}
	p := min(int(downloaded*100/total), 100)
	return p
}

// compareVersion 比较两个语义化版本号
// 返回值: >0 表示 v1 > v2, <0 表示 v1 < v2, 0 表示相等
func compareVersion(v1, v2 string) int {
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	// 分离预发布后缀
	v1Base, v1Pre := splitVersion(v1)
	v2Base, v2Pre := splitVersion(v2)

	// 比较基础版本号
	if c := compareBase(v1Base, v2Base); c != 0 {
		return c
	}

	// 基础版本相同，比较预发布
	// 无预发布 > 有预发布（正式版 > 测试版）
	if v1Pre == "" && v2Pre == "" {
		return 0
	}
	if v1Pre == "" {
		return 1
	}
	if v2Pre == "" {
		return -1
	}
	if v1Pre < v2Pre {
		return -1
	}
	if v1Pre > v2Pre {
		return 1
	}
	return 0
}

// 将版本号拆分为基础版本和预发布后缀
func splitVersion(v string) (base, pre string) {
	for i, c := range v {
		if c == '-' {
			return v[:i], v[i+1:]
		}
	}
	return v, ""
}

// 比较 x.y.z 形式的基础版本号
func compareBase(a, b string) int {
	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")
	maxLen := max(len(partsB), len(partsA))
	for i := range maxLen {
		na, nb := 0, 0
		if i < len(partsA) {
			na, _ = strconv.Atoi(partsA[i])
		}
		if i < len(partsB) {
			nb, _ = strconv.Atoi(partsB[i])
		}
		if na > nb {
			return 1
		}
		if na < nb {
			return -1
		}
	}
	return 0
}

func statfs(path string, stat *sysStatfs) error {
	var s syscall.Statfs_t
	if err := syscall.Statfs(path, &s); err != nil {
		return err
	}
	stat.Bsize = uint64(s.Bsize)
	stat.Bavail = uint64(s.Bavail)
	return nil
}
