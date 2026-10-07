package admin

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pion/dtls/v3/pkg/crypto/selfsign"
)

func TestBuildCertInfoSelfSigned(t *testing.T) {
	c, err := selfsign.GenerateSelfSignedWithDNS("vpn.example.com", "alt.example.com")
	if err != nil {
		t.Fatal(err)
	}
	info, err := buildCertInfo("main", &c)
	if err != nil {
		t.Fatal(err)
	}
	if !info.SelfSigned || info.Source != "self_signed" {
		t.Fatalf("want self-signed, got %+v", info)
	}
	if info.SubjectCN != "vpn.example.com" || len(info.SANs) != 2 {
		t.Fatalf("subject/sans: %+v", info)
	}
	if info.ChainLength != 1 || info.DaysRemaining < 0 || info.Expired {
		t.Fatalf("chain/days: %+v", info)
	}
	if len(info.SHA256) != 32*3-1 || strings.ToUpper(info.SHA256) != info.SHA256 {
		t.Fatalf("sha256 format: %s", info.SHA256)
	}
	if _, err := buildCertInfo("main", &tls.Certificate{}); err == nil {
		t.Fatal("empty cert should error")
	}
}

func TestGeoipUnpack(t *testing.T) {
	payload := []byte("MMDB-CONTENT")
	read := func(r io.Reader, err error) string {
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(r)
		return string(b)
	}
	// 裸 mmdb
	if got := read(geoipUnpack(bytes.NewReader(payload), "a.mmdb")); got != string(payload) {
		t.Fatalf("raw: %q", got)
	}
	// .mmdb.gz
	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	gw.Write(payload)
	gw.Close()
	if got := read(geoipUnpack(bytes.NewReader(gzBuf.Bytes()), "a.mmdb.gz")); got != string(payload) {
		t.Fatalf("gz: %q", got)
	}
	// .tar.gz（MaxMind 格式：目录 + LICENSE + mmdb）
	var tgz bytes.Buffer
	gw = gzip.NewWriter(&tgz)
	tw := tar.NewWriter(gw)
	tw.WriteHeader(&tar.Header{Name: "GeoLite2-Country_20260101/LICENSE.txt", Mode: 0o644, Size: 3, Typeflag: tar.TypeReg})
	tw.Write([]byte("lic"))
	tw.WriteHeader(&tar.Header{Name: "GeoLite2-Country_20260101/GeoLite2-Country.mmdb", Mode: 0o644, Size: int64(len(payload)), Typeflag: tar.TypeReg})
	tw.Write(payload)
	tw.Close()
	gw.Close()
	if got := read(geoipUnpack(bytes.NewReader(tgz.Bytes()), "GeoLite2-Country.tar.gz")); got != string(payload) {
		t.Fatalf("tgz: %q", got)
	}
}

func TestStatsGeoNoDB(t *testing.T) {
	w := httptest.NewRecorder()
	StatsGeo(w, httptest.NewRequest("GET", "/api/v1/statsinfo/geo", nil))
	if !strings.Contains(w.Body.String(), `"datas":[]`) {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}
