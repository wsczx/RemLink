// RemLink 是一个企业级远程办公vpn软件，可以支持多人同时在线使用

//go:build !windows

package main

import (
	"context"
	"embed"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/wsczx/remlink/app"
	"github.com/wsczx/remlink/base"
)

//go:embed ui
var uiData embed.FS

// 程序版本
var (
	appVer    string
	commitId  string
	buildDate string
)

func main() {
	a := app.New(uiData)
	a.Init(appVer, commitId, buildDate)

	if handled, msg, err := a.RunMaintenance(); handled {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprint(os.Stderr, msg)
		os.Exit(0)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		signalWatch(cancel)
	}()

	if err := a.Run(ctx); err != nil {
		base.Error("启动错误:", err)
		os.Exit(1)
	}
}

func signalWatch(cancel context.CancelFunc) {
	base.Info("Server pid: ", os.Getpid())

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGALRM, syscall.SIGUSR2)
	defer signal.Stop(sigs)
	for {
		sig := <-sigs
		base.Info("Get signal:", sig)
		switch sig {
		case syscall.SIGUSR2:
			base.Info("Reload")
		default:
			base.Info("Stop")
			cancel()
			return
		}
	}
}
