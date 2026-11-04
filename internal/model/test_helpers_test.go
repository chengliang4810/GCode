package model

import (
	"context"
	"time"
)

func newTestClient() *Client {
	client := NewClient()
	retry := DefaultRetryPolicy()
	retry.Wait = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
	client.RetryPolicy = &retry
	return client
}
