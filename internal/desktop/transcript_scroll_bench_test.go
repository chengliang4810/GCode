package desktop

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"gcode/internal/accounting"
	"gcode/internal/assets"
	"gcode/internal/contracts"
	"gcode/internal/mock"
	"github.com/egoist/mygo/ui"
)

// Measures the production view and wheel input without touching a user profile.
// Tester measures CPU layout/build work; it does not measure native GPU presentation.
func BenchmarkTranscriptWheel(b *testing.B) {
	for _, scene := range []struct {
		count int
		rich  bool
	}{{20, false}, {200, false}, {200, true}} {
		count := scene.count
		b.Run(fmt.Sprintf("messages=%d/rich=%v", count, scene.rich), func(b *testing.B) {
			if err := assets.RegisterFonts(); err != nil {
				b.Fatal(err)
			}
			backend, err := mock.New(mock.Options{Scenario: "default", Instant: true})
			if err != nil {
				b.Fatal(err)
			}
			app, err := NewWithOptions(backend, Options{Now: time.Now})
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(app.Close)
			app.selectSession("chat-0")
			app.state.Settings.General.Language = "en"
			app.state.Settings.Appearance.ReduceMotion = true
			app.expanded["session-info-user"] = true
			app.expanded["session-info"] = false
			app.ensureComposerFooterRefs(false)
			messages := railFixtureMessages(count)
			if scene.rich {
				for i := range messages {
					if messages[i].Role != "assistant" {
						continue
					}
					for j := range messages[i].Blocks {
						messages[i].Blocks[j].Text = "### 滚动性能测量\n\n" + strings.Repeat("这是用于测试滚动布局的中文正文，包含 **强调内容**、`inline code` 和普通段落。", 8) + "\n\n```go\n" + strings.Repeat("func example() { println(123) }\n", 12) + "```\n"
					}
				}
			}
			app.state.Messages[app.sessionID] = messages
			app.state.Revision++
			publishMetricsFixture(app)
			var bounds ui.Rect
			tt := ui.NewTester(func(c *ui.Context) { app.View(c); bounds = app.messages.Bounds(c) }, 1320, 880)
			tt.Frame()
			app.messages.ScrollTo(len(app.transcriptCache.flat)/2, ui.Start)
			tt.Frame()
			b.ReportAllocs()
			n := 0
			for b.Loop() {
				delta := float32(-36)
				if n%40 >= 20 {
					delta = 36
				}
				tt.Wheel(bounds.X+bounds.W/2, bounds.Y+bounds.H/2, 0, delta)
				n++
			}
		})
	}
}

// RCODE_SCROLL_SESSION_FILE is a read-only session record. Only display data is
// decoded; the mock service never opens the user's runtime or provider config.
func BenchmarkStoredTranscriptWheel(b *testing.B) {
	path := os.Getenv("RCODE_SCROLL_SESSION_FILE")
	if path == "" {
		b.Skip("set RCODE_SCROLL_SESSION_FILE to a session record")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	var record struct {
		Messages      []contracts.Message      `json:"messages"`
		Runs          []contracts.Run          `json:"runs"`
		ModelRequests []contracts.ModelRequest `json:"modelRequests"`
	}
	if err = json.Unmarshal(raw, &record); err != nil {
		b.Fatal(err)
	}
	for i := range record.Messages {
		record.Messages[i].SessionID = "chat-0"
		record.Messages[i].Streaming = false
	}
	for i := range record.Runs {
		record.Runs[i].SessionID = "chat-0"
	}
	for i := range record.ModelRequests {
		record.ModelRequests[i].SessionID = "chat-0"
	}
	for _, scene := range []struct {
		position string
		scale    float32
	}{{"start", 1}, {"middle", 1}, {"end", 1}, {"middle", 2}, {"sweep", 2}, {"sweep-open", 2}} {
		position := scene.position
		b.Run(fmt.Sprintf("%s/scale=%g", position, scene.scale), func(b *testing.B) {
			if err := assets.RegisterFonts(); err != nil {
				b.Fatal(err)
			}
			backend, err := mock.New(mock.Options{Scenario: "default", Instant: true})
			if err != nil {
				b.Fatal(err)
			}
			app, err := NewWithOptions(backend, Options{Now: time.Now})
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(app.Close)
			app.selectSession("chat-0")
			app.ensureComposerFooterRefs(false)
			app.state.Settings.Appearance.ReduceMotion = true
			app.expanded["session-info-user"] = true
			app.expanded["session-info"] = false
			app.state.Messages["chat-0"] = record.Messages
			app.state.Runs = record.Runs
			app.state.ModelRequests = record.ModelRequests
			app.state.Revision++
			publishMetricsFixture(app)
			if position == "sweep-open" {
				for _, message := range transcriptEntries(record.Messages) {
					for _, row := range transcriptRows(message, false) {
						if len(row.Activity) > 0 {
							app.rememberActivityFold(row.ID, true)
						}
					}
				}
			}
			var bounds ui.Rect
			tt := ui.NewTester(func(c *ui.Context) { app.View(c); bounds = app.messages.Bounds(c) }, 1320, 880)
			tt.SetScale(scene.scale)
			tt.Frame()
			row := 0
			if position == "middle" {
				row = len(app.transcriptCache.flat) / 2
			}
			if position == "end" {
				row = len(app.transcriptCache.flat) - 1
			}
			app.messages.ScrollTo(row, ui.Start)
			tt.Frame()
			b.ReportMetric(float64(len(app.transcriptCache.flat)), "rows")
			b.ReportAllocs()
			n := 0
			timings := make([]float64, 0, 4096)
			direction := float32(120)
			for b.Loop() {
				delta := float32(36)
				if n%80 >= 40 || position == "end" {
					delta = -36
				}
				if position == "end" && n%80 >= 40 {
					delta = 36
				}
				if strings.HasPrefix(position, "sweep") {
					if app.messages.AtEnd() {
						direction = -120
					}
					first, _ := app.messages.Visible()
					if first == 0 && direction < 0 {
						direction = 120
					}
					delta = direction
				}
				started := time.Now()
				tt.Wheel(bounds.X+bounds.W/2, bounds.Y+bounds.H/2, 0, delta)
				timings = append(timings, float64(time.Since(started))/float64(time.Millisecond))
				n++
			}
			b.ReportMetric(float64(len(app.transcriptCache.flat)), "rows")
			slices.Sort(timings)
			b.ReportMetric(timings[(len(timings)-1)*95/100], "p95-ms")
			b.ReportMetric(timings[len(timings)-1], "max-ms")
		})
	}
}

func BenchmarkStoredConversationMetrics(b *testing.B) {
	path := os.Getenv("RCODE_SCROLL_SESSION_FILE")
	if path == "" {
		b.Skip("set RCODE_SCROLL_SESSION_FILE to a session record")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	var record struct {
		Session       contracts.Session        `json:"session"`
		Messages      []contracts.Message      `json:"messages"`
		Runs          []contracts.Run          `json:"runs"`
		ModelRequests []contracts.ModelRequest `json:"modelRequests"`
	}
	if err = json.Unmarshal(raw, &record); err != nil {
		b.Fatal(err)
	}
	a := &App{state: contracts.Snapshot{Revision: 1, Sessions: []contracts.Session{record.Session}, Messages: map[string][]contracts.Message{record.Session.ID: record.Messages}, Runs: record.Runs, ModelRequests: record.ModelRequests}}
	publishMetricsFixture(a)
	b.Run("recalculate", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			accounting.Own(record.Session.ID, "", record.Messages, record.ModelRequests, record.Runs)
		}
	})
	b.Run("saved-projection", func(b *testing.B) {
		a.metricsFor(record.Session.ID, "")
		b.ReportAllocs()
		for b.Loop() {
			a.metricsFor(record.Session.ID, "")
		}
	})
}
