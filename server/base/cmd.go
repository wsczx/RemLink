package base

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"runtime"
	"strings"

	"github.com/skip2/go-qrcode"
	"github.com/spf13/cobra"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/xlzd/gotp"
)

var (
	// pass明文
	passwd string
	// 生成otp
	otp bool
	// 生成密钥
	secret bool
	// 显示版本信息
	rev bool
	// 输出debug信息
	debug bool
	// 重置管理员密码
	ResetAdminPassFlag bool
	// 强制禁用管理员两步验证
	DisableAdminOtpFlag bool
	// 切换 FakeDNS 可见性
	EnableFakeDNSFlag bool

	// Used for flags.
	runSrv bool

	rootCmd *cobra.Command
)

// Execute executes the root command.
func execute() {
	initCmd()

	err := rootCmd.Execute()
	if err != nil {
		fmt.Println(err)
		os.Exit(0)
	}

	if !runSrv {
		if debug {
			items := GetConfigMeta()
			fmtStr := "%-18v %-20v %v\n"
			fmt.Printf(fmtStr, "Name", "Value", "Info")
			for _, v := range items {
				fmt.Printf(fmtStr, v["name"], v["data"], v["usage"])
			}
		}
		os.Exit(0)
	}

	// 配置文件已移除，服务启动使用默认值、命令行/环境变量和数据库配置。
}

func initCmd() {
	rootCmd = &cobra.Command{
		Use:   "remlink",
		Short: "RemLink Secure Remote Access Gateway",
		Long:  `RemLink is a Enterprise-Grade Secure Remote Access Gateway`,
		Run: func(cmd *cobra.Command, args []string) {
			runSrv = true

			if rev {
				printVersion()
				os.Exit(0)
			}
		},
	}

	// 依据 ServerConfig 字段 + configMetas 注册命令行参数和默认值
	registerFlagsFromConfig()

	rootCmd.Flags().BoolVarP(&rev, "version", "v", false, "display version info")
	rootCmd.Flags().BoolVarP(&ResetAdminPassFlag, "reset-admin-password", "", false, "重置管理员密码为随机密码")
	rootCmd.Flags().BoolVarP(&DisableAdminOtpFlag, "disable-admin-otp", "", false, "强制禁用管理员两步验证（OTP密钥丢失时使用）")
	rootCmd.Flags().BoolVarP(&EnableFakeDNSFlag, "enable-fakedns", "", false, "开启 FakeDNS 功能在管理界面的可见性")
	rootCmd.Flags().MarkHidden("enable-fakedns")
	rootCmd.AddCommand(initToolCmd())
}

// 遍历 ServerConfig 字段，结合 configMetas 元数据注册 cobra flags
func registerFlagsFromConfig() {
	typ := reflect.TypeFor[ServerConfig]()
	for field := range typ.Fields() {
		name := field.Tag.Get("json")
		// 默认值/说明来自 config.go 的 configMetas
		meta := configMetas[name]
		usage := meta.usage

		switch field.Type.Kind() {
		case reflect.String:
			dv := meta.defaultVal
			// AdminPass 默认值由 CompleteConfig 随机生成，不暴露在命令行
			if dv == "defaultPwd" {
				dv = ""
			}
			rootCmd.Flags().StringP(name, "", dv, usage)
		case reflect.Int:
			var iDef int
			fmt.Sscan(meta.defaultVal, &iDef)
			rootCmd.Flags().IntP(name, "", iDef, usage)
		case reflect.Bool:
			rootCmd.Flags().BoolP(name, "", meta.defaultVal == "true", usage)
		}
	}
}

func initToolCmd() *cobra.Command {
	toolCmd := &cobra.Command{
		Use:   "tool",
		Short: "RemLink tool",
		Long:  `RemLink tool is a application`,
	}

	toolCmd.Flags().BoolVarP(&rev, "version", "v", false, "display version info")
	toolCmd.Flags().BoolVarP(&secret, "secret", "s", false, "generate a random jwt secret")
	toolCmd.Flags().StringVarP(&passwd, "passwd", "p", "", "convert the password plaintext")
	toolCmd.Flags().BoolVarP(&otp, "otp", "o", false, "generate a random otp secret")
	toolCmd.Flags().BoolVarP(&debug, "debug", "d", false, "list the config metadata")

	toolCmd.Run = func(cmd *cobra.Command, args []string) {
		runSrv = false

		switch {
		case rev:
			printVersion()
		case secret:
			s, _ := utils.RandSecret(40, 60)
			s = strings.Trim(s, "=")
			fmt.Printf("Secret: %s\n", s)
		case otp:
			s := gotp.RandomSecret(32)
			fmt.Printf("Otp: %s\n\n", s)
			qrstr := fmt.Sprintf("otpauth://totp/%s:%s?issuer=%s&secret=%s", "remlink_admin", "admin@remlink", "remlink_admin", s)
			qr, _ := qrcode.New(qrstr, qrcode.High)
			ss := qr.ToSmallString(false)
			io.WriteString(os.Stderr, ss)
		case passwd != "":
			pass, _ := utils.PasswordHash(passwd)
			fmt.Printf("Passwd: %s\n", pass)
		case debug:
			// 配置元数据打印见 execute()
		default:
			fmt.Println("Using [remlink tool -h] for help")
		}
	}

	return toolCmd
}

func printVersion() {
	fmt.Printf("%s v%s build on %s [%s, %s] date:%s commit_id(%s)\n",
		APP_NAME, APP_VER, runtime.Version(), runtime.GOOS, runtime.GOARCH, BuildDate, CommitId)
}

// 生成 LINK_<NAME> 形式的环境变量名
func envKeyOf(name string) string {
	return "LINK_" + strings.ToUpper(strings.ReplaceAll(name, ".", "_"))
}

// 返回字段的原始字符串值与是否被用户显式设置
// 取值优先级：flag(已变更) > 环境变量 > flag 默认值
func readConfigRaw(name string) (raw string, explicit bool) {
	if f := rootCmd.Flags().Lookup(name); f != nil {
		if f.Changed {
			return f.Value.String(), true
		}
		if v, ok := os.LookupEnv(envKeyOf(name)); ok {
			return v, true
		}
		return f.DefValue, false
	}
	if v, ok := os.LookupEnv(envKeyOf(name)); ok {
		return v, true
	}
	return "", false
}
