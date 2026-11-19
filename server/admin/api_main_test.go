package admin

import (
	"os"
	"testing"

	"github.com/wsczx/remlink/auth"
	"github.com/wsczx/remlink/auth/authsrv"
)

func TestMain(m *testing.M) {
	auth.SetRegistry(auth.NewProviderRegistry())
	authsrv.RegisterProviders(auth.Registry)
	os.Exit(m.Run())
}
