package base

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

func RestartProcess() error {
	app, err := os.Executable()
	if err != nil {
		return err
	}
	closeDeviceFDs()
	return syscall.Exec(app, os.Args, os.Environ())
}

// 关闭设备 fd
func closeDeviceFDs() {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return
	}
	for _, e := range entries {
		fd, err := strconv.Atoi(e.Name())
		if err != nil || fd <= 2 {
			continue
		}
		target, err := os.Readlink("/proc/self/fd/" + e.Name())
		if err != nil {
			continue
		}
		if strings.HasPrefix(target, "/dev/net/tun") || strings.HasPrefix(target, "/dev/tap") {
			syscall.Close(fd)
		}
	}
}
