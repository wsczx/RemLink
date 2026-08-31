package utils

import (
	"os"
	"path/filepath"
	"sort"
)

// 获取物理网卡名称列表，过滤掉虚拟网卡（docker、veth、bridge、tun、tap、lo 等）
func GetPhysicalInterfaces() []string {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return nil
	}
	// 虚拟网卡前缀/名称黑名单
	virtualPrefixes := []string{"docker", "veth", "br-", "virbr", "tun", "tap", "lvtap", "cali", "flannel", "cilium", "kube-", "cni-", "vxlan", "wg", "ip6tnl", "sit"}
	virtualExact := map[string]bool{"lo": true}

	var ifaces []string
	for _, entry := range entries {
		name := entry.Name()
		if virtualExact[name] {
			continue
		}
		skip := false
		for _, prefix := range virtualPrefixes {
			if len(name) >= len(prefix) && name[:len(prefix)] == prefix {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		// 物理网卡在 sysfs 中有 device 子目录（指向 PCI/USB 设备）
		devicePath := filepath.Join("/sys/class/net", name, "device")
		if _, err := os.Stat(devicePath); err == nil {
			ifaces = append(ifaces, name)
		}
	}

	sort.Strings(ifaces)
	return ifaces
}
