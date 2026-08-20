package sessdata

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wsczx/remlink/base"
)

// 验证 v4/v6 NAT 开关彼此独立。
func TestGroupNatSwitches(t *testing.T) {
	ast := assert.New(t)
	original := base.GetCfg()
	t.Cleanup(func() {
		base.UpdateCfg(func(c *base.ServerConfig) {
			*c = *original
		})
	})

	for _, tc := range []struct {
		name           string
		v4, v6         bool
		wantV4, wantV6 bool
	}{
		{name: "both enabled", v4: true, v6: true, wantV4: true, wantV6: true},
		{name: "only v6 enabled", v4: false, v6: true, wantV4: false, wantV6: true},
		{name: "only v4 enabled", v4: true, v6: false, wantV4: true, wantV6: false},
		{name: "both disabled", v4: false, v6: false, wantV4: false, wantV6: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base.UpdateCfg(func(c *base.ServerConfig) {
				c.GlobalNat = tc.v4
				c.GlobalNat6 = tc.v6
			})
			cfg := base.GetCfg()
			ast.Equal(tc.wantV4, cfg.GlobalNat)
			ast.Equal(tc.wantV6, cfg.GlobalNat6)
		})
	}
}
