package admin

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/geoip"
)

// 上传 mmdb 的大小上限（Country 库通常 < 10MB，压缩包更小；留足余量）
const geoipUploadMax = 128 << 20

// GeoIP 数据库状态：是否加载、路径、数据库类型、构建时间、记录数等
func GeoipStatus(w http.ResponseWriter, r *http.Request) {
	RespSucess(w, geoip.GetStatus())
}

// 上传 GeoIP 国家库（multipart 字段 file）。
// 支持 .mmdb、.mmdb.gz（DB-IP 下载格式）、.tar.gz（MaxMind GeoLite2 下载格式，自动提取其中的 .mmdb）。
// 校验通过后原子替换配置路径上的文件并立即热加载
func GeoipUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, geoipUploadMax+(1<<20))
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		RespError(w, RespParamErr, "上传失败: ", err)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		RespError(w, RespParamErr, "请选择 mmdb 文件")
		return
	}
	defer file.Close()

	d := geoip.Default()
	if d == nil {
		RespError(w, RespInternalErr, "GeoIP 未初始化")
		return
	}
	dst := d.Path()
	if dst == "" {
		RespError(w, RespParamErr, "未配置 geoip_db 路径")
		return
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	src, err := geoipUnpack(file, header.Filename)
	if err != nil {
		RespError(w, RespParamErr, err)
		return
	}

	// 写入同目录临时文件 → 校验 → rename 原子替换（不能原地覆盖正在被 mmap 的旧文件）
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".geoip-*.mmdb")
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	n, err := io.Copy(tmp, io.LimitReader(src, geoipUploadMax+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if n == 0 || n > geoipUploadMax {
		RespError(w, RespParamErr, "文件为空或超过大小上限")
		return
	}
	meta, err := geoip.Validate(tmpName)
	if err != nil {
		RespError(w, RespParamErr, "不是有效的 Country mmdb: ", err)
		return
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if err := os.Rename(tmpName, dst); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if err := geoip.Reload(); err != nil {
		RespError(w, RespInternalErr, "加载失败: ", err)
		return
	}
	dbdata.AdminLog("系统设置", "GeoIP数据库", fmt.Sprintf("上传了GeoIP数据库(%s, 构建于 %s)", meta.DatabaseType, meta.BuildTime().Format("2006-01-02")), r.RemoteAddr)
	RespSucess(w, geoip.GetStatus())
}

// 按文件头识别 gzip / tar.gz / 裸 mmdb，返回 mmdb 内容流
func geoipUnpack(f io.Reader, name string) (io.Reader, error) {
	br := bufio.NewReaderSize(f, 4096)
	head, _ := br.Peek(2)
	if len(head) < 2 || head[0] != 0x1f || head[1] != 0x8b {
		return br, nil // 非 gzip，按裸 mmdb 处理
	}
	gz, err := gzip.NewReader(br)
	if err != nil {
		return nil, fmt.Errorf("gzip 解压失败: %w", err)
	}
	inner := bufio.NewReaderSize(gz, 4096)
	// tar 头在偏移 257 处有 "ustar" 魔数
	h, _ := inner.Peek(262)
	if len(h) >= 262 && string(h[257:262]) == "ustar" || strings.HasSuffix(strings.ToLower(name), ".tar.gz") || strings.HasSuffix(strings.ToLower(name), ".tgz") {
		tr := tar.NewReader(inner)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				return nil, errors.New("压缩包中未找到 .mmdb 文件")
			}
			if err != nil {
				return nil, fmt.Errorf("读取 tar 失败: %w", err)
			}
			if hdr.Typeflag == tar.TypeReg && strings.HasSuffix(strings.ToLower(hdr.Name), ".mmdb") {
				return tr, nil
			}
		}
	}
	return inner, nil
}
