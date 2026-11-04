package model

import (
	"context"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const DefaultMaxRetries = 10

// RetryStatus is request-scoped progress, not a new assistant message.
// Attempt counts retries: the initial request is attempt zero.
type RetryStatus struct {
	Attempt    int    `json:"attempt"`
	MaxRetries int    `json:"maxRetries"`
	Reason     string `json:"reason"`
	HTTPStatus int    `json:"httpStatus,omitempty"`
}

// RetryPolicy owns the single budget shared by HTTP, transport and stream
// failures. Wait is injectable for deterministic tests; nil uses a cancelable timer.
type RetryPolicy struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	Jitter     bool
	Wait       func(context.Context, time.Duration) error
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxRetries: DefaultMaxRetries, BaseDelay: 2 * time.Second, MaxDelay: time.Minute, Jitter: true}
}

func (p RetryPolicy) delay(retry int, after *time.Duration) time.Duration {
	uncapped := float64(p.BaseDelay) * math.Pow(2, float64(max(0, retry-1)))
	capped := time.Duration(min(uncapped, float64(p.MaxDelay)))
	if after != nil && *after >= 0 && (*after <= 5*time.Minute || float64(*after) < uncapped) {
		delay := *after
		if p.Jitter {
			delay += time.Duration(rand.Float64() * float64(min(delay/5, 5*time.Second)))
		}
		return delay
	}
	if p.Jitter {
		return time.Duration(float64(capped) * (.5 + rand.Float64()*.5))
	}
	return capped
}

func (p RetryPolicy) wait(ctx context.Context, delay time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.Wait != nil {
		return p.Wait(ctx, delay)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func parseRetryAfter(value string, now time.Time) *time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.ParseInt(value, 10, 32); err == nil && seconds >= 0 {
		delay := time.Duration(seconds) * time.Second
		return &delay
	}
	if date, err := http.ParseTime(value); err == nil {
		delay := max(time.Duration(0), date.Sub(now))
		return &delay
	}
	return nil
}
