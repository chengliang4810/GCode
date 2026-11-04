package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"gcode/internal/model"
)

func TestInterruptedStepRecoveryIsBoundedAndCancelable(t *testing.T) {
	for _, cancelOnRetry := range []bool{false, true} {
		name := "retry limit"
		if cancelOnRetry {
			name = "cancel during recovery"
		}
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"unfinished\"}}]}\n\n")
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			recorded, retries := 0, 0
			provider := model.Provider{
				ID: "p", Name: "P", Protocol: model.Chat, BaseURL: server.URL,
				Models: []model.Model{{ID: "m"}},
			}
			gateway, err := newAgentTestClient().Bind(provider, provider.Models[0])
			if err != nil {
				t.Fatal(err)
			}
			err = Run(ctx, Options{
				Model:  gateway,
				Record: func(model.Message) error { recorded++; return nil },
				Emit: func(event Event) error {
					if event.Kind == "message.retry" {
						retries++
						if cancelOnRetry {
							cancel()
						}
					}
					return nil
				},
				Pending: func(bool) []model.Message { return nil },
			})
			if recorded != 0 {
				t.Fatal("incomplete response was recorded")
			}
			if cancelOnRetry {
				if !errors.Is(err, context.Canceled) || requests.Load() != 1 || retries != 1 {
					t.Fatalf("cancellation restarted the model: %v, requests=%d, retries=%d", err, requests.Load(), retries)
				}
			} else if !errors.Is(err, model.ErrStreamInterrupted) || requests.Load() != model.DefaultMaxRetries+1 || retries != model.DefaultMaxRetries {
				t.Fatalf("retry limit was not honored: %v, requests=%d, retries=%d", err, requests.Load(), retries)
			}
		})
	}
}

type testModel struct {
	stream func(model.Request) (model.Result, error)
}

func (m testModel) Stream(_ context.Context, request model.Request, _ func(model.Delta) error) (model.Result, error) {
	return m.stream(request)
}

func TestRunUsesModelInterfaceForToolRoundTrip(t *testing.T) {
	cwd := t.TempDir()
	steps := 0
	backend := testModel{stream: func(request model.Request) (model.Result, error) {
		steps++
		if len(request.Tools) != 9 || request.System != "task rules" {
			t.Fatal("unified request lost tools or instructions")
		}
		if steps == 1 {
			return model.Result{
				Message: model.Message{Role: "assistant", Calls: []model.Call{{
					ID: "call-1", Name: "Write", Arguments: `{"file_path":"proof.txt","content":"done"}`,
				}}},
				Stop: model.StopTools,
			}, nil
		}
		last := request.History[len(request.History)-1]
		if last.Role != "tool" || last.CallID != "call-1" || !strings.Contains(last.Text, "Wrote") {
			t.Fatalf("model did not receive the tool result: %+v", last)
		}
		return model.Result{Message: model.Message{Role: "assistant", Text: "finished"}, Stop: model.StopComplete}, nil
	}}
	recorded := []model.Message{}
	err := Run(context.Background(), Options{
		Model: backend, CWD: cwd, Tools: CoreTools(),
		Request: model.Request{System: "task rules"},
		Record: func(message model.Message) error {
			recorded = append(recorded, message)
			return nil
		},
		Emit:    func(Event) error { return nil },
		Pending: func(bool) []model.Message { return nil },
	})
	data, readErr := os.ReadFile(filepath.Join(cwd, "proof.txt"))
	if err != nil || readErr != nil || string(data) != "done" || steps != 2 || len(recorded) != 3 {
		t.Fatalf("interface tool loop failed: err=%v file=%s/%v steps=%d messages=%d",
			err, data, readErr, steps, len(recorded))
	}
}
