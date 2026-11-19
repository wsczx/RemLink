package dbdata

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/denisenkom/go-mssqldb"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/utils"
	"xorm.io/xorm"
	"xorm.io/xorm/core"
	"xorm.io/xorm/dialects"
)

var (
	db     atomic.Pointer[DB]
	initMu sync.Mutex
)

func GetDB() *DB {
	return db.Load()
}

// 注入持久化
func SetDB(s *DB) {
	db.Store(s)
}

func TableModels() []any {
	return []any{
		&User{},
		&Setting{},
		&Group{},
		&IpMap{},
		&AccessAudit{},
		&AdminOpLog{},
		&Policy{},
		&StatsNetwork{},
		&StatsCpu{},
		&StatsMem{},
		&StatsOnline{},
		&UserActLog{},
		&PasswordReset{},
		&ClientCertData{},
		&Provider{},
		&WebVpnApp{},
		&WebVpnAudit{},
		&WebVpnRevoke{},
	}
}

func initDb() error {
	var err error
	var engine *xorm.Engine
	dbType := base.GetCfg().DbType
	dbSource := base.GetCfg().DbSource

	if dbType == "sqlite3" {
		if dbSource != ":memory:" && !strings.HasPrefix(dbSource, "file:") {
			dbSource = "file:" + dbSource
		}
		if !strings.Contains(dbSource, "_busy_timeout=") {
			sep := "?"
			if strings.Contains(dbSource, "?") {
				sep = "&"
			}
			dbSource += sep + "_busy_timeout=5000"
		}

		sqlDB, dErr := newRetrySqliteDB(dbSource)
		if dErr != nil {
			return dErr
		}
		dialect, dErr := dialects.OpenDialect(dbType, dbSource)
		if dErr != nil {
			return dErr
		}
		engine, err = xorm.NewEngineWithDialectAndDB(dbType, dbSource, dialect, core.FromDB(sqlDB))
	} else {
		engine, err = xorm.NewEngine(dbType, dbSource)
	}
	if err != nil {
		return err
	}

	engine.DatabaseTZ = time.Local
	engine.TZLocation = time.Local

	if base.GetCfg().ShowSQL {
		engine.ShowSQL(true)
	}

	SetDB(NewDB(engine))

	if err = engine.Sync2(TableModels()...); err != nil {
		return err
	}
	return nil
}

func initData() error {
	initMu.Lock()
	defer initMu.Unlock()

	install := &SettingInstall{}
	err := Settings.Get(install)

	if err == nil && install.Installed {
		return nil
	}
	if err != nil && !utils.CheckErrNotFound(err) {
		return err
	}
	return addInitData()
}

func addInitData() error {
	sess := GetDB().Engine().NewSession()
	defer sess.Close()
	if err := sess.Begin(); err != nil {
		return err
	}
	defer sess.Rollback()

	cfg, err := addInitDataTx(sess)
	if err != nil {
		return err
	}
	if err := sess.Commit(); err != nil {
		return err
	}
	base.LoadPersistedConfig(cfg)
	return nil
}

func addInitDataTx(sess *xorm.Session) (base.ServerConfig, error) {
	cfg := *base.GetCfg()
	base.CompleteConfig(&cfg)

	for _, v := range defaultSettings(cfg) {
		if err := Settings.SessAddIfAbsent(sess, v); err != nil {
			return cfg, err
		}
	}
	if err := seedPoliciesGroupsTx(sess); err != nil {
		return cfg, err
	}
	if err := Settings.SessUpsert(sess, &SettingInstall{Installed: true}); err != nil {
		return cfg, err
	}
	return cfg, nil
}
