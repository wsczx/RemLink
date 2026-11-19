package dbdata_test

import (
	"os"
	"testing"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/auth/authsrv"
	"github.com/wsczx/remlink/dbdata"
)

func TestMain(m *testing.M) {
	auth.SetRegistry(auth.NewProviderRegistry())
	authsrv.RegisterProviders(auth.Registry)
	dbdata.SetBackups(dbdata.NewBackupManager())
	os.Exit(m.Run())
}
