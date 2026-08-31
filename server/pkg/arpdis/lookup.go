// Currently only Darwin and Linux support this.

package arpdis

import (
	"net"
	"os/exec"
	"strings"
)

func doLookup(ip net.IP) *Addr {
	err := doPing(ip.String())
	if err != nil {
		addr := &Addr{IP: net.IPv4(1, 2, 3, 4), Type: TypeUnreachable}
		copy(addr.IP, ip)
		return addr
	}

	return doArpShow(ip)
}

func doArpShow(ip net.IP) *Addr {
	cmd := exec.Command("ip", "n", "show", ip.String())
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	// os.Open("/proc/net/arp")
	// 192.168.1.2      0x1         0x2         e0:94:67:e2:42:5d     *        eth0
	// 192.168.1.2 dev eth0 lladdr 08:00:27:94:a5:a4 STALE
	outS := strings.ReplaceAll(string(out), "  ", " ")
	outS = strings.TrimSpace(outS)
	arpArr := strings.Split(outS, " ")
	if len(arpArr) != 6 {
		return nil
	}
	mac, err := net.ParseMAC(arpArr[4])
	if err != nil {
		return nil
	}

	addr := &Addr{IP: net.IPv4(1, 2, 3, 4), HardwareAddr: mac}
	copy(addr.IP, ip)
	return addr
}

// IP address       HW type     Flags       HW address            Mask     Device
// 172.23.24.12     0x1         0x2         00:e0:4c:73:5c:48     *        remlink0
// 172.23.24.1      0x1         0x2         3c:8c:40:a0:7a:2d     *        remlink0
// 172.23.24.13     0x1         0x2         00:1c:42:4d:33:46     *        remlink0
// 172.23.24.2      0x1         0x0         00:00:00:00:00:00     *        remlink0
// 172.23.24.14     0x1         0x0         00:00:00:00:00:00     *        remlink0
