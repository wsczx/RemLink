package base

const (
	APP_NAME = "RemLink"
)

var (
	// app版本号
	APP_VER = "0.0.1"
	// 提交id
	CommitId  string
	BuildDate string
)

func SetVersion(ver, commit, buildDate string) {
	if ver != "" {
		APP_VER = ver
	}
	CommitId = commit
	BuildDate = buildDate
}
