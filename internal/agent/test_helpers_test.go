package agent

import (
	"context"
	"time"

	"gcode/internal/model"
)

func newAgentTestClient() *model.Client {
	client := model.NewClient()
	retry := model.DefaultRetryPolicy()
	retry.Wait = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
	client.RetryPolicy = &retry
	return client
}
