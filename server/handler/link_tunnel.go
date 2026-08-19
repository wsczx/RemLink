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
	"github.com/wsczx/remlink/sessdata"
)

var (
	hn string
)

func init() {
	// 获取主机名称
	hn, _ = os.Hostname()
}

func HttpSetHeader(w http.ResponseWriter, key string, value string) {
	w.Header()[key] = []string{value}
}

func HttpAddHeader(w http.ResponseWriter, key string, value string) {
	w.Header()[key] = append(w.Header()[key], value)
}

func LinkTunnel(w http.ResponseWriter, r *http.Request) {
	if base.GetLogLevel() == base.LogLevelTrace {
		hd, _ := httputil.DumpRequest(r, true)
		base.Trace("LinkTunnel: ", string(hd))
	}

	// 判断session-token的值
	cookie, err := r.Cookie("webvpn")
	if err != nil || cookie.Value == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	sess := sessdata.SToken2Sess(cookie.Value)
	if sess == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// 开启link
	cSess := sess.NewConn()
	if cSess == nil {
		base.Warn("LinkTunnel: NewConn returned nil")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// 客户端信息
	cstpMtu := r.Header.Get("X-CSTP-MTU")
	cstpBaseMtu := r.Header.Get("X-CSTP-Base-MTU")
	masterSecret := r.Header.Get("X-DTLS-Master-Secret")
	localIp := r.Header.Get("X-Cstp-Local-Address-Ip4")
	// 出口ip
	exportIp4 := r.Header.Get("X-Cstp-Remote-Address-Ip4")
	mobile := r.Header.Get("X-Cstp-License")

	//设置 mtu
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

	base.Info(dbdata.UserLabel(sess.Username, cSess.Nickname), cSess.IpAddr, cSess.MacHw, cSess.Client, mobile)

	// 连接建立成功日志
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
	}, cSess.UserAgent)

	// 检测密码套件
	dtlsCiphersuite := checkDtls12Ciphersuite(r.Header.Get("X-Dtls12-Ciphersuite"))
	base.Trace("dtlsCiphersuite", dtlsCiphersuite)

	// 返回客户端数据
	HttpSetHeader(w, "Server", fmt.Sprintf("%s %s", base.APP_NAME, base.APP_VER))
	HttpSetHeader(w, "X-CSTP-Version", "1")
	HttpSetHeader(w, "X-CSTP-Server-Name", fmt.Sprintf("%s %s", base.APP_NAME, base.APP_VER))
	HttpSetHeader(w, "X-CSTP-Protocol", "Copyright (c) 2004 Cisco Systems, Inc.")
	HttpSetHeader(w, "X-CSTP-Address", cSess.IpAddr.String())          // 分配的ip地址
	HttpSetHeader(w, "X-CSTP-Netmask", cSess.IpPool.Ipv4Mask.String()) // 子网掩码
	// IPv6 双栈：仅当分配了 v6 地址时下发
	//   - AnyConnect 从 X-CSTP-Address-IP6 读取 v6 地址；
	//   - OpenConnect 从 X-CSTP-Address（含冒号的值）读取 v6 地址，并把
	//     X-CSTP-Address-IP6 当成 netmask（netmask6）处理
	if cSess.IpAddr6 != nil {
		if strings.Contains(cSess.UserAgent, "openconnect") {
			HttpAddHeader(w, "X-CSTP-Address", cSess.IpAddr6.String())
			HttpAddHeader(w, "X-CSTP-Netmask", cSess.IpAddr6.String()+"/128")
		} else {
			HttpSetHeader(w, "X-CSTP-Address-IP6", cSess.IpAddr6.String())
			HttpSetHeader(w, "X-CSTP-Netmask-IP6", "128")
		}
	}
	HttpSetHeader(w, "X-CSTP-Hostname", hn) // 机器名称
	HttpSetHeader(w, "X-CSTP-Base-MTU", cstpBaseMtu)
	// 客户端dns的默认搜索域
	if base.GetCfg().DefaultDomain != "" {
		HttpSetHeader(w, "X-CSTP-Default-Domain", base.GetCfg().DefaultDomain)
	}

	// 压缩
	if cmpName, ok := cSess.SetPickCmp("cstp", r.Header.Get("X-Cstp-Accept-Encoding")); ok {
		HttpSetHeader(w, "X-CSTP-Content-Encoding", cmpName)
	}
	if base.GetCfg().ServerDTLS {
		if cmpName, ok := cSess.SetPickCmp("dtls", r.Header.Get("X-Dtls-Accept-Encoding")); ok {
			HttpSetHeader(w, "X-DTLS-Content-Encoding", cmpName)
		}
	}

	rp := cSess.Policy

	// 允许本地LAN访问vpn网络，必须放在路由的第一个
	if rp.AllowLan {
		HttpSetHeader(w, "X-CSTP-Split-Exclude", "0.0.0.0/255.255.255.255")
		// 双栈下同步排除本地 IPv6 局域网::/128
		if cSess.IpAddr6 != nil {
			HttpAddHeader(w, "X-CSTP-Split-Exclude-IP6", "::/128")
		}
	}
	// dns地址：IPv4 走 X-CSTP-DNS，IPv6 走 X-CSTP-DNS-IP6
	for _, v := range rp.ClientDns {
		ip := net.ParseIP(v.Val)
		if ip != nil && ip.To4() == nil {
			HttpAddHeader(w, "X-CSTP-DNS-IP6", v.Val)
		} else {
			HttpAddHeader(w, "X-CSTP-DNS", v.Val)
		}
	}
	// 分割dns
	for _, v := range cSess.Group.SplitDns {
		HttpAddHeader(w, "X-CSTP-Split-DNS", v.Val)
	}

	// 允许的路由
	for _, v := range rp.RouteInclude {
		if strings.ToLower(v.Val) == dbdata.ALL {
			continue
		}
		// IPv6 路由下发到 -IP6 头（CIDR 格式）；v4 维持原 IpMask 下发
		if _, ipNet, err := net.ParseCIDR(v.Val); err == nil && ipNet.IP.To4() == nil {
			HttpAddHeader(w, "X-CSTP-Split-Include-IP6", ipNet.String())
		} else {
			HttpAddHeader(w, "X-CSTP-Split-Include", v.IpMask)
		}
	}
	// 不允许的路由
	for _, v := range rp.RouteExclude {
		if _, ipNet, err := net.ParseCIDR(v.Val); err == nil && ipNet.IP.To4() == nil {
			HttpAddHeader(w, "X-CSTP-Split-Exclude-IP6", ipNet.String())
		} else {
			HttpAddHeader(w, "X-CSTP-Split-Exclude", v.IpMask)
		}
	}
	// 排除出口ip路由(出口ip不加密传输)
	if base.GetCfg().ExcludeExportIp {
		if exportIp4 != "" {
			HttpAddHeader(w, "X-CSTP-Split-Exclude", exportIp4+"/255.255.255.255")
		}
		// IPv6 出口地址：优先取客户端上报头 X-Cstp-Remote-Address-Ip6，
		// 取不到则回退到实际 TCP 连接源地址（客户端经 IPv6 连入时为 v6）。
		exportIp6 := r.Header.Get("X-Cstp-Remote-Address-Ip6")
		if exportIp6 == "" {
			if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
				if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
					exportIp6 = host
				}
			}
		}
		if exportIp6 != "" {
			HttpAddHeader(w, "X-CSTP-Split-Exclude-IP6", exportIp6+"/128")
		}
	}
	// 下发 FakeIP 段路由
	if rp.EnableFakeDNS {
		// 确保 FakeDNS 上游 DNS 服务器可达
		if rp.FakeDNSUpstream != "" {
			// 上游可能是 v4 或 v6 地址，掩码须对应 /32 或 /128
			mask := "/255.255.255.255"
			if up := net.ParseIP(rp.FakeDNSUpstream); up != nil && up.To4() == nil {
				mask = "/128"
			}
			HttpAddHeader(w, "X-CSTP-Split-Include", rp.FakeDNSUpstream+mask)
		}
		_, ipNet, err := net.ParseCIDR(sessdata.DefaultFakeIPRange)
		if err == nil {
			mask := net.IP(ipNet.Mask)
			ipMask := fmt.Sprintf("%s/%s", ipNet.IP, mask)
			HttpAddHeader(w, "X-CSTP-Split-Include", ipMask)
		}
		// v6 FakeIP 段路由：仅「双栈开启 且 IPv6 优先开启」时下发
		if base.GetCfg().Ipv6CIDR != "" && rp.PreferIPv6 {
			if _, ipNet6, err6 := net.ParseCIDR(sessdata.DefaultFakeIPv6Range); err6 == nil {
				HttpAddHeader(w, "X-CSTP-Split-Include-IP6", ipNet6.String())
			}
		}
	}

	HttpSetHeader(w, "X-CSTP-Lease-Duration", "1209600") // ip地址租期
	HttpSetHeader(w, "X-CSTP-Session-Timeout", "none")
	HttpSetHeader(w, "X-CSTP-Session-Timeout-Alert-Interval", "60")
	HttpSetHeader(w, "X-CSTP-Session-Timeout-Remaining", "none")
	HttpSetHeader(w, "X-CSTP-Idle-Timeout", "18000")
	HttpSetHeader(w, "X-CSTP-Disconnected-Timeout", "18000")
	HttpSetHeader(w, "X-CSTP-Keep", "true")
	tunnelAllDNS := "false"
	if rp.EnableFakeDNS {
		tunnelAllDNS = "true"
	}
	HttpSetHeader(w, "X-CSTP-Tunnel-All-DNS", tunnelAllDNS)

	HttpSetHeader(w, "X-CSTP-Rekey-Time", "86400") // 172800
	HttpSetHeader(w, "X-CSTP-Rekey-Method", "new-tunnel")
	HttpSetHeader(w, "X-DTLS-Rekey-Time", "86400")
	HttpSetHeader(w, "X-DTLS-Rekey-Method", "new-tunnel")

	HttpSetHeader(w, "X-CSTP-DPD", fmt.Sprintf("%d", cstpDpd))
	HttpSetHeader(w, "X-CSTP-Keepalive", fmt.Sprintf("%d", cstpKeepalive))
	// HttpSetHeader(w, "X-CSTP-Banner", banner.Banner)
	HttpSetHeader(w, "X-CSTP-MSIE-Proxy-Lockdown", "true")
	HttpSetHeader(w, "X-CSTP-Smartcard-Removal-Disconnect", "true")

	HttpSetHeader(w, "X-CSTP-MTU", fmt.Sprintf("%d", cSess.Mtu)) // 1399
	HttpSetHeader(w, "X-DTLS-MTU", fmt.Sprintf("%d", cSess.Mtu))

	if base.GetCfg().ServerDTLS {
		HttpSetHeader(w, "X-DTLS-Session-ID", sess.DtlsSid)
		HttpSetHeader(w, "X-DTLS-Port", dtlsPort)
		HttpSetHeader(w, "X-DTLS-DPD", fmt.Sprintf("%d", cstpDpd))
		HttpSetHeader(w, "X-DTLS-Keepalive", fmt.Sprintf("%d", cstpKeepalive))
		HttpSetHeader(w, "X-DTLS12-CipherSuite", dtlsCiphersuite)
	}

	HttpSetHeader(w, "X-CSTP-License", "accept")
	HttpSetHeader(w, "X-CSTP-Routing-Filtering-Ignore", "false")
	HttpSetHeader(w, "X-CSTP-Quarantine", "false")
	HttpSetHeader(w, "X-CSTP-Disable-Always-On-VPN", "false")
	HttpSetHeader(w, "X-CSTP-Client-Bypass-Protocol", "true")
	HttpSetHeader(w, "X-CSTP-TCP-Keepalive", "false")
	// 设置域名拆分隧道（移动端不支持）
	if mobile != "mobile" {
		SetPostAuthXml(cSess.Policy, w)
	}

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

	// 开始数据处理
	switch base.GetCfg().LinkMode {
	case base.LinkModeTUN:
		err = LinkTun(cSess)
	case base.LinkModeTAP:
		err = LinkTap(cSess)
	case base.LinkModeMacvtap:
		err = LinkMacvtap(cSess)
	case base.LinkModeIpvtap:
		err = LinkIpvtap(cSess)
	}
	if err != nil {
		base.Error(err)
		// 显式关闭会话，否则已分配的 IP/连接数配额不会回收，长期运行导致地址池枯竭
		cSess.SetLogoutCode(dbdata.UserLogoutTunErr)
		cSess.Close()
		conn.Close()
		return
	}
	go LinkCstp(conn, bufRW, cSess)
}

// 设置域名拆分隧道
func SetPostAuthXml(rp *dbdata.Policy, w http.ResponseWriter) error {
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
	HttpSetHeader(w, "X-CSTP-Post-Auth-XML", xmlAuth.String())
	return nil
}
