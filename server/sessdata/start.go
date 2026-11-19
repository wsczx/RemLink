package sessdata

import (
	"context"
	"fmt"
)

func Start(ctx context.Context, m *SessionManager) error {
	if err := m.pools.Init(); err != nil {
		return fmt.Errorf("IP池初始化失败: %w", err)
	}
	m.checkSession(ctx)
	m.saveStatsInfo(ctx)
	return nil
}
