package handler

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"text/template"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/wsczx/remlink/sessdata"
)

type Tunnel struct {
	hn       string
	sessions *sessdata.SessionManager
	tap      *TapLink
}

func NewTunnel(sessions *sessdata.SessionManager, tap *TapLink) *Tunnel {
	hn, _ := os.Hostname()
	return &Tunnel{hn: hn, sessions: sessions, tap: tap}
}

func (t *Tunnel) setHeader(w http.ResponseWriter, key, value string) {
	w.Header()[key] = []string{value}
}

func (t *Tunnel) addHeader(w http.ResponseWriter, key, value string) {
	w.Header()[key] = append(w.Header()[key], value)
}

// CSTP/DTLS 隧道参数（秒）
const (
	cstpLeaseDurationSec               = "1209600" // 地址租期
	cstpIdleTimeoutSec                 = "18000"   // 空闲超时
	cstpDisconnectedTimeoutSec         = "18000"   // 断开超时
	cstpSessionTimeoutAlertIntervalSec = "60"      // 超时提醒间隔
	cstpRekeyTimeSec                   = "86400"   // CSTP 重协商间隔
	dtlsRekeyTimeSec                   = "86400"   // DTLS 重协商间隔
)

// 承载一次隧道协商的上下文
type tunnelParams struct {
	sess        *sessdata.Session
	cSess       *sessdata.ConnSession
	baseMtu     string
	mobile      string
	exportIp4   string
	keepalive   int
	dpd         int
	dtlsPort    string
	ciphersuite string
}

// 处理 CSTP 隧道协商请求
func (t *Tunnel) OpenCSTP(w http.ResponseWriter, r *http.Request) {
	if base.GetLogLevel() == base.LogLevelTrace {
		hd, _ := httputil.DumpRequest(r, true)
		base.Trace("LinkTunnel: ", utils.RedactSensitive(string(hd)))
	}

	params, ok := t.openSession(w, r)
	if !ok {
		return
	}

	t.sendConfig(w, r, params)

	w.WriteHeader(http.StatusOK)

	hClone := w.Header().Clone()
	buf := &bytes.Buffer{}
	_ = hClone.Write(buf)
	base.Debug("LinkTunnel Response Header:", buf.String())

	hj := w.(http.Hijacker)
	conn, bufRW, err := hj.Hijack()
	if err != nil {
		base.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := t.startDataPlane(params.cSess); err != nil {
		base.Error(err)
		// 显式关闭会话，否则已分配的 IP/连接数配额不会回收，长期运行导致地址池枯竭
		params.cSess.SetLogoutCode(dbdata.UserLogoutTunErr)
		params.cSess.Close()
		conn.Close()
		return
	}
	go LinkCSTP(conn, bufRW, params.cSess)
}

// 校验门户会话并建立数据面连接，失败时已写错误响应
func (t *Tunnel) openSession(w http.ResponseWriter, r *http.Request) (*tunnelParams, bool) {
	cookie, err := r.Cookie("webvpn")
	if err != nil || cookie.Value == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return nil, false
	}

	sess := t.sessions.SToken2Sess(cookie.Value)
	if sess == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return nil, false
	}

	// 用户已过期：立即关闭会话并拒绝建隧道
	if sess.IsExpired() {
		base.Info("用户已过期，拒绝重建隧道:", sess.Username)
		t.sessions.CloseSess(sess.Token, dbdata.UserLogoutExpire)
		w.WriteHeader(http.StatusUnauthorized)
		return nil, false
	}

	// 开启link
	cSess := sess.NewConn()
	if cSess == nil {
		base.Warn("LinkTunnel: NewConn returned nil")
		w.WriteHeader(http.StatusUnauthorized)
		return nil, false
	}

	// 客户端信息
	cstpMtu := r.Header.Get("X-CSTP-MTU")
	cstpBaseMtu := r.Header.Get("X-CSTP-Base-MTU")
	masterSecret := r.Header.Get("X-DTLS-Master-Secret")
	localIp := r.Header.Get("X-Cstp-Local-Address-Ip4")
	// 出口ip
	exportIp4 := r.Header.Get("X-Cstp-Remote-Address-Ip4")
	mobile := r.Header.Get("X-Cstp-License")
	// mobile := cSess.Sess.Os == 4 || cSess.Sess.Os == 5

	// 设置 mtu
	if cSess.Mtu == 0 {
		cSess.SetMtu(cstpMtu)
	}

	cSess.MasterSecret = masterSecret
	cSess.RemoteAddr = r.RemoteAddr
	cSess.UserAgent = strings.ToLower(r.UserAgent())
	cSess.LocalIp = net.ParseIP(localIp)
	cstpKeepalive := base.GetCfg().CstpKeepalive
	cstpDpd := base.GetCfg().CstpDpd
	cSess.Client = "pc"
	if mobile == "mobile" {
		// 手机客户端
		cstpKeepalive = base.GetCfg().MobileKeepalive
		cstpDpd = base.GetCfg().MobileDpd
		cSess.Client = "mobile"
	}
	cSess.CstpDpd = cstpDpd

	dtlsPort := "443"
	if norm := base.FormatListenAddr(base.GetCfg().AdvertiseDTLSAddr); norm != "" {
		if _, port, err := net.SplitHostPort(norm); err == nil {
			dtlsPort = port
		}
	}

	base.Info(dbdata.Users.Label(sess.Username, cSess.Nickname), cSess.IpAddr, cSess.MacHw, cSess.Client, mobile)

	connInfo := "连接成功"
	if len(cSess.MacHw) > 0 {
		connInfo = fmt.Sprintf("连接成功 | MAC:%s", cSess.MacHw.String())
	}
	dbdata.UserActLogIns.Add(dbdata.UserActLog{
		Username:        cSess.Username,
		GroupName:       cSess.Group.Name,
		IpAddr:          cSess.IpAddr.String(),
		RemoteAddr:      r.RemoteAddr,
		Status:          dbdata.UserConnected,
		Info:            connInfo,
		DeviceType:      sess.DeviceType,
		PlatformVersion: sess.PlatformVersion,
		Os:              sess.Os,
	}, cSess.UserAgent)

	return &tunnelParams{
		sess:        sess,
		cSess:       cSess,
		baseMtu:     cstpBaseMtu,
		mobile:      mobile,
		exportIp4:   exportIp4,
		keepalive:   cstpKeepalive,
		dpd:         cstpDpd,
		dtlsPort:    dtlsPort,
		ciphersuite: getDTLS12Ciphersuite(r.Header.Get("X-Dtls12-Ciphersuite")),
	}, true
}

// 下发 CSTP/DTLS 协商响应头
func (t *Tunnel) sendConfig(w http.ResponseWriter, r *http.Request, params *tunnelParams) {
	sess := params.sess
	cSess := params.cSess

	base.Trace("params.ciphersuite", params.ciphersuite)
	t.setHeader(w, "X-CSTP-Version", "1")
	t.setHeader(w, "X-CSTP-Server-Name", fmt.Sprintf("%s %s", base.APP_NAME, base.APP_VER))
	t.setHeader(w, "X-CSTP-Protocol", "Copyright (c) 2004 Cisco Systems, Inc.")
	t.setHeader(w, "X-CSTP-Address", cSess.IpAddr.String())          // 分配的ip地址
	t.setHeader(w, "X-CSTP-Netmask", cSess.IpPool.Ipv4Mask.String()) // 子网掩码
	// OpenConnect 从 X-CSTP-Address 读 v6 地址、把 X-CSTP-Address-IP6 当 netmask；
	// AnyConnect 相反，故两种客户端需分别下发
	if cSess.IpAddr6 != nil {
		if strings.Contains(cSess.UserAgent, "openconnect") {
			t.addHeader(w, "X-CSTP-Address", cSess.IpAddr6.String())
			t.addHeader(w, "X-CSTP-Netmask", cSess.IpAddr6.String()+"/128")
		} else {
			t.setHeader(w, "X-CSTP-Address-IP6", cSess.IpAddr6.String())
			t.setHeader(w, "X-CSTP-Netmask-IP6", "128")
		}
	}
	t.setHeader(w, "X-CSTP-Hostname", t.hn) // 机器名称
	t.setHeader(w, "X-CSTP-Base-MTU", params.baseMtu)
	// 客户端dns的默认搜索域
	if base.GetCfg().DefaultDomain != "" {
		t.setHeader(w, "X-CSTP-Default-Domain", base.GetCfg().DefaultDomain)
	}

	// 压缩
	if cmpName, ok := cSess.SetPickCmp("cstp", r.Header.Get("X-Cstp-Accept-Encoding")); ok {
		t.setHeader(w, "X-CSTP-Content-Encoding", cmpName)
	}
	if base.GetCfg().ServerDTLS {
		if cmpName, ok := cSess.SetPickCmp("dtls", r.Header.Get("X-Dtls-Accept-Encoding")); ok {
			t.setHeader(w, "X-DTLS-Content-Encoding", cmpName)
		}
	}

	rp := cSess.Policy

	// 允许本地LAN访问vpn网络，必须放在路由的第一个
	if rp.AllowLan {
		t.setHeader(w, "X-CSTP-Split-Exclude", "0.0.0.0/255.255.255.255")
		// 双栈下同步排除本地 IPv6 局域网::/128
		if cSess.IpAddr6 != nil {
			t.addHeader(w, "X-CSTP-Split-Exclude-IP6", "::/128")
		}
	}
	// dns地址：IPv4 走 X-CSTP-DNS，IPv6 走 X-CSTP-DNS-IP6
	for _, v := range rp.ClientDns {
		ip := net.ParseIP(v.Val)
		if ip != nil && ip.To4() == nil {
			t.addHeader(w, "X-CSTP-DNS-IP6", v.Val)
		} else {
			t.addHeader(w, "X-CSTP-DNS", v.Val)
		}
	}
	// 分割dns
	for _, v := range cSess.Group.SplitDns {
		t.addHeader(w, "X-CSTP-Split-DNS", v.Val)
	}

	// 允许的路由
	for _, v := range rp.RouteInclude {
		if strings.ToLower(v.Val) == dbdata.ALL {
			continue
		}
		// IPv6 路由下发到 -IP6 头（CIDR 格式）；v4 维持原 IpMask 下发
		if _, ipNet, err := net.ParseCIDR(v.Val); err == nil {
			if ipNet.IP.To4() == nil {
				t.addHeader(w, "X-CSTP-Split-Include-IP6", ipNet.String())
				continue
			}
			if ones, bits := ipNet.Mask.Size(); ones == 0 && bits == 32 {
				continue
			}
		}
		t.addHeader(w, "X-CSTP-Split-Include", v.IpMask)
	}
	// 不允许的路由
	for _, v := range rp.RouteExclude {
		if _, ipNet, err := net.ParseCIDR(v.Val); err == nil && ipNet.IP.To4() == nil {
			t.addHeader(w, "X-CSTP-Split-Exclude-IP6", ipNet.String())
		} else {
			t.addHeader(w, "X-CSTP-Split-Exclude", v.IpMask)
		}
	}
	// 排除出口ip路由(出口ip不加密传输)
	if base.GetCfg().ExcludeExportIp {
		if params.exportIp4 != "" {
			t.addHeader(w, "X-CSTP-Split-Exclude", params.exportIp4+"/255.255.255.255")
		}
		// IPv6 出口地址：优先取客户端上报头 X-Cstp-Remote-Address-Ip6
		// 取不到则回退到实际 TCP 连接源地址（客户端经 IPv6 连入时为 v6）
		exportIp6 := r.Header.Get("X-Cstp-Remote-Address-Ip6")
		if exportIp6 == "" {
			if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
				if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
					exportIp6 = host
				}
			}
		}
		if exportIp6 != "" {
			t.addHeader(w, "X-CSTP-Split-Exclude-IP6", exportIp6+"/128")
		}
	}
	// 下发 FakeIP 段路由
	if rp.EnableFakeDNS {
		// 上游与 FakeIP 段须走隧道，否则客户端无法解析 FakeIP 对应域名
		if rp.FakeDNSUpstream != "" {
			// 上游可能是 v4 或 v6 地址，掩码须对应 /32 或 /128
			mask := "/255.255.255.255"
			if up := net.ParseIP(rp.FakeDNSUpstream); up != nil && up.To4() == nil {
				mask = "/128"
			}
			t.addHeader(w, "X-CSTP-Split-Include", rp.FakeDNSUpstream+mask)
		}
		_, ipNet, err := net.ParseCIDR(sessdata.DefaultFakeIPRange)
		if err == nil {
			mask := net.IP(ipNet.Mask)
			ipMask := fmt.Sprintf("%s/%s", ipNet.IP, mask)
			t.addHeader(w, "X-CSTP-Split-Include", ipMask)
		}
		// v6 FakeIP 段路由：仅「双栈开启 且 IPv6 优先开启」时下发
		if base.GetCfg().Ipv6CIDR != "" && rp.PreferIPv6 {
			if _, ipNet6, err6 := net.ParseCIDR(sessdata.DefaultFakeIPv6Range); err6 == nil {
				t.addHeader(w, "X-CSTP-Split-Include-IP6", ipNet6.String())
			}
		}
	}

	t.setHeader(w, "X-CSTP-Lease-Duration", cstpLeaseDurationSec) // ip地址租期
	t.setHeader(w, "X-CSTP-Session-Timeout", "none")
	t.setHeader(w, "X-CSTP-Session-Timeout-Alert-Interval", cstpSessionTimeoutAlertIntervalSec)
	t.setHeader(w, "X-CSTP-Session-Timeout-Remaining", "none")
	t.setHeader(w, "X-CSTP-Idle-Timeout", cstpIdleTimeoutSec)
	t.setHeader(w, "X-CSTP-Disconnected-Timeout", cstpDisconnectedTimeoutSec)
	t.setHeader(w, "X-CSTP-Keep", "true")
	tunnelAllDNS := "false"
	if rp.EnableFakeDNS {
		tunnelAllDNS = "true"
	}
	t.setHeader(w, "X-CSTP-Tunnel-All-DNS", tunnelAllDNS)

	t.setHeader(w, "X-CSTP-Rekey-Time", cstpRekeyTimeSec)
	t.setHeader(w, "X-CSTP-Rekey-Method", "new-tunnel")
	t.setHeader(w, "X-DTLS-Rekey-Time", dtlsRekeyTimeSec)
	t.setHeader(w, "X-DTLS-Rekey-Method", "new-tunnel")

	t.setHeader(w, "X-CSTP-DPD", fmt.Sprintf("%d", params.dpd))
	t.setHeader(w, "X-CSTP-Keepalive", fmt.Sprintf("%d", params.keepalive))
	t.setHeader(w, "X-CSTP-MSIE-Proxy-Lockdown", "true")
	t.setHeader(w, "X-CSTP-Smartcard-Removal-Disconnect", "true")
	t.setHeader(w, "X-CSTP-MTU", fmt.Sprintf("%d", cSess.Mtu))
	t.setHeader(w, "X-DTLS-MTU", fmt.Sprintf("%d", cSess.Mtu))
	if base.GetCfg().ServerDTLS {
		t.setHeader(w, "X-DTLS-Session-ID", sess.DtlsSid)
		t.setHeader(w, "X-DTLS-Port", params.dtlsPort)
		t.setHeader(w, "X-DTLS-DPD", fmt.Sprintf("%d", params.dpd))
		t.setHeader(w, "X-DTLS-Keepalive", fmt.Sprintf("%d", params.keepalive))
		t.setHeader(w, "X-DTLS12-CipherSuite", params.ciphersuite)
	}
	t.setHeader(w, "X-CSTP-License", "accept")
	t.setHeader(w, "X-CSTP-Routing-Filtering-Ignore", "false")
	t.setHeader(w, "X-CSTP-Quarantine", "false")
	t.setHeader(w, "X-CSTP-Disable-Always-On-VPN", "false")
	t.setHeader(w, "X-CSTP-Client-Bypass-Protocol", "true")
	t.setHeader(w, "X-CSTP-TCP-Keepalive", "false")
	if params.mobile != "mobile" {
		t.postAuthXML(cSess.Policy, w)
	}
}

func (t *Tunnel) startDataPlane(cSess *sessdata.ConnSession) error {
	switch base.GetCfg().LinkMode {
	case base.LinkModeTUN:
		return LinkTun(t.sessions, cSess)
	case base.LinkModeTAP:
		return t.tap.Link(cSess)
	case base.LinkModeMacvtap:
		return LinkMacvtap(t.tap, t.sessions, cSess)
	case base.LinkModeIpvtap:
		return LinkIpvtap(t.tap, t.sessions, cSess)
	}
	return nil
}

// 下发域名拆分隧道配置
func (t *Tunnel) postAuthXML(rp *dbdata.Policy, w http.ResponseWriter) error {
	if rp.DsExcludeDomains == "" && rp.DsIncludeDomains == "" {
		return nil
	}
	tmpl, err := template.New("post_auth_xml").Parse(ds_domains_xml)
	if err != nil {
		return err
	}
	var result bytes.Buffer
	err = tmpl.Execute(&result, rp)
	if err != nil {
		return err
	}
	var xmlAuth strings.Builder
	for v := range strings.SplitSeq(result.String(), "\n") {
		xmlAuth.WriteString(strings.TrimSpace(v))
	}
	t.setHeader(w, "X-CSTP-Post-Auth-XML", xmlAuth.String())
	return nil
}
