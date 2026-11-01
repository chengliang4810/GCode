package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gcode/internal/model"
)

// This model has no HTTP client, channel configuration or protocol identity.
// The real Agent and filesystem tools still perform the entire execution loop.
type toolRoundTripModel struct {
	t    *testing.T
	step int
}

func (m *toolRoundTripModel) Stream(_ context.Context, r model.Request, _ func(model.Delta) error) (model.Result, error) {
	m.step++
	if len(r.Tools) != len(CoreTools()) || r.System != "uniform instructions" {
		m.t.Fatal("agent did not pass the uniform tool definitions/system message")
	}
	switch m.step {
	case 1:
		return model.Result{
			Message: model.Message{Role: "assistant", Calls: []model.Call{{
				ID: "write-1", Name: "Write", Arguments: `{"file_path":"proof.txt","content":"written by agent"}`,
			}}},
			Stop: model.StopTools,
		}, nil
	case 2:
		last := r.History[len(r.History)-1]
		if last.Role != "tool" || last.CallID != "write-1" || last.IsError {
			m.t.Fatalf("tool result was not carried back to the model: %+v", last)
		}
		return model.Result{
			Message: model.Message{Role: "assistant", Text: "complete"},
			Stop:    model.StopComplete,
		}, nil
	default:
		m.t.Fatal("unexpected extra model step")
		return model.Result{}, nil
	}
}

func TestAgentUsesModelInterfaceForRealToolRoundTrip(t *testing.T) {
	workspace := t.TempDir()
	bound := &toolRoundTripModel{t: t}
	recorded := []model.Message{}
	err := Run(context.Background(), Options{
		Model: bound, CWD: workspace, Tools: CoreTools(),
		Request: model.Request{
			System:  "uniform instructions",
			History: []model.Message{{Role: "user", Text: "write proof"}},
		},
		Record: func(message model.Message) error {
			recorded = append(recorded, message)
			return nil
		},
		Emit:    func(Event) error { return nil },
		Pending: func(bool) []model.Message { return nil },
	})
	data, readErr := os.ReadFile(filepath.Join(workspace, "proof.txt"))
	if err != nil || readErr != nil || string(data) != "written by agent" ||
		bound.step != 2 || len(recorded) != 3 || recorded[2].Text != "complete" {
		t.Fatalf("model interface tool loop failed: err=%v read=%v data=%s history=%+v", err, readErr, data, recorded)
	}
}
