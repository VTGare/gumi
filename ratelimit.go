package gumi

import (
	"time"

	"github.com/zekroTJA/timedmap"
)

type RateLimiter struct {
	Cooldown time.Duration
	timedMap *timedmap.TimedMap
}

func NewRateLimiter(cooldown time.Duration) *RateLimiter {
	return &RateLimiter{
		Cooldown: cooldown,
		timedMap: timedmap.New(1 * time.Second),
	}
}

func (r *RateLimiter) Contains(key string) bool {
	return r.timedMap.Contains(key)
}

func (r *RateLimiter) Expires(key string) (time.Duration, error) {
	t, err := r.timedMap.GetExpires(key)
	if err != nil {
		return 0, err
	}

	return t.Sub(time.Now()), nil
}

func (r *RateLimiter) Set(key string) {
	r.timedMap.Set(key, true, r.Cooldown)
}
