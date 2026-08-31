package sessdata

import (
	"context"

	"golang.org/x/time/rate"
)

const maxBurst = 1500 // MTU

type LimitRater struct {
	limit *rate.Limiter
}

func NewLimitRater(bytesPerSec int) *LimitRater {
	b := max(min(bytesPerSec, maxBurst), 1)
	return &LimitRater{limit: rate.NewLimiter(rate.Limit(bytesPerSec), b)}
}

func (l *LimitRater) Wait(bt int) error {
	ctx := context.Background()
	burst := l.limit.Burst()
	for bt > 0 {
		n := min(bt, burst)
		if err := l.limit.WaitN(ctx, n); err != nil {
			return err
		}
		bt -= n
	}
	return nil
}
