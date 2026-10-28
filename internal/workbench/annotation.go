package workbench

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	"gcode/internal/contracts"

	"github.com/egoist/mygo/ui"
)

// This is the unmodified public-page picker from Zeron v0.2.128. It runs only
// after the user enables annotation in an external browser body, never as UI.
//
//go:embed annotate.js
var annotationScript string

const annotationLimit = 24

type pageEvaluator interface {
	EvalContext(context.Context, string) (any, error)
}
type annotationSession struct {
	cancel context.CancelFunc
	nonce  uint32
	page   *scriptQueue
}
type annotationPick struct {
	contracts.Annotation
	Marker uint32 `json:"marker"`
}
type annotationPoll struct {
	State string           `json:"state"`
	Picks []annotationPick `json:"picks"`
}

func decodeAnnotationPoll(value any) (annotationPoll, error) {
	var poll annotationPoll
	text, ok := value.(string)
	if !ok || len(text) > 1024*1024 {
		return poll, fmt.Errorf("unreadable annotation response")
	}
	if err := json.Unmarshal([]byte(text), &poll); err != nil {
		return poll, err
	}
	if poll.State == "" {
		return poll, fmt.Errorf("annotation response has no state")
	}
	if len(poll.Picks) > annotationLimit {
		poll.Picks = poll.Picks[:annotationLimit]
	}
	for i := range poll.Picks {
		poll.Picks[i].Annotation = poll.Picks[i].Bounded()
	}
	return poll, nil
}

func (b *browser) stopAnnotation() {
	if b.annotation == nil {
		return
	}
	session := b.annotation
	b.annotation = nil
	session.cancel()
	// A stopped old session must not erase a newer picker on the same page.
	_ = session.page.post(fmt.Sprintf("(() => { const a=window.__zeronAnnotator; if(a && a.take(%d)!==null) a.stop(); return 'stopped'; })()", session.nonce), "")
}

func (s *State) toggleAnnotation(b *browser) {
	if b.annotation != nil {
		b.stopAnnotation()
		return
	}
	if b.view == nil || b.scripts == nil || b.data.Error != "" || b.data.Loading || s.OnAnnotation == nil || s.closed.Load() {
		return
	}
	var bytes [4]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		s.Error = err.Error()
		return
	}
	nonce := binary.LittleEndian.Uint32(bytes[:])
	if nonce == 0 {
		nonce = 1
	}
	ctx, cancel := context.WithCancel(s.ctx)
	session := &annotationSession{cancel: cancel, nonce: nonce, page: b.scripts}
	b.annotation = session
	p := s.palette()
	css := func(c ui.Color) string { return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A) }
	palette, _ := json.Marshal(map[string]any{"accent": css(p.Accent), "surface": css(p.Dialog), "border": css(p.Border), "text": css(p.Text), "muted": css(p.Muted), "faint": css(p.Faint), "chip": css(sourceInk(p.Dark, .09)), "dark": p.Dark, "font": s.snapshot.Settings.Appearance.Font})
	go s.pollAnnotations(ctx, b, session, string(palette))
}

func (s *State) pollAnnotations(ctx context.Context, b *browser, session *annotationSession, palette string) {
	evaluate := func(script string) (any, error) {
		callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		return session.page.EvalContext(callCtx, script)
	}
	_, err := evaluate(annotationScript + fmt.Sprintf("\n;window.__zeronAnnotator.start(%d,%s);return 'started';", session.nonce, palette))
	if err != nil {
		s.finishAnnotation(b, session)
		return
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	picks, misses := 0, 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		value, err := evaluate(fmt.Sprintf("(() => { const a=window.__zeronAnnotator; return a ? a.take(%d) : '{\"state\":\"gone\"}'; })()", session.nonce))
		poll, parseErr := decodeAnnotationPoll(value)
		if err != nil || parseErr != nil {
			misses++
			if misses >= 20 {
				s.finishAnnotation(b, session)
				return
			}
			continue
		}
		misses = 0
		room := annotationLimit - picks
		if len(poll.Picks) > room {
			poll.Picks = poll.Picks[:room]
		}
		picks += len(poll.Picks)
		finished := poll.State != "active" || picks >= annotationLimit
		s.win.Update(func() {
			if s.closed.Load() || b.annotation != session || s.browsers[b.data.ID] != b {
				return
			}
			for _, pick := range poll.Picks {
				s.nextAnnotation++
				annotation := pick.Annotation
				annotation.Index = s.nextAnnotation
				if s.OnAnnotation != nil {
					s.OnAnnotation(annotation)
				}
				marker, number := pick.Marker, annotation.Index
				_ = session.page.post(fmt.Sprintf("window.__zeronAnnotator?.label(%d,%d)", marker, number), "")
			}
			if finished {
				b.stopAnnotation()
			}
		})
		if finished {
			return
		}
	}
}

func (s *State) finishAnnotation(b *browser, session *annotationSession) {
	if s.win == nil {
		return
	}
	s.win.Update(func() {
		if !s.closed.Load() && b.annotation == session {
			b.stopAnnotation()
		}
	})
}

// KeepAnnotations removes page markers whose chips were removed from the draft.
func (s *State) KeepAnnotations(indices []uint32) {
	s.keepAnnotations(indices, "keep")
}

// ReserveAnnotationIndices prevents a restored draft from reusing chip IDs.
func (s *State) ReserveAnnotationIndices(indices []uint32) {
	for _, index := range indices {
		if index > s.nextAnnotation {
			s.nextAnnotation = index
		}
	}
}

func (s *State) keepAnnotations(indices []uint32, coalesce string) {
	if indices == nil {
		indices = []uint32{}
	}
	data, _ := json.Marshal(indices)
	for _, b := range s.browsers {
		if b.scripts != nil {
			_ = b.scripts.post("window.__zeronAnnotator?.keep("+string(data)+")", coalesce)
		}
	}
}

func (s *State) FocusAnnotation(index uint32) {
	for _, b := range s.browsers {
		if b.scripts != nil {
			_ = b.scripts.post(fmt.Sprintf("window.__zeronAnnotator?.focus(%d)", index), "focus")
		}
	}
}

// ResetAnnotations is called when a draft is sent or discarded.
func (s *State) ResetAnnotations() {
	s.nextAnnotation = 0
	// Reset is a barrier: a later keep must not replace it, because numbering
	// begins at one again and old markers could otherwise share those numbers.
	s.keepAnnotations(nil, "")
	for _, b := range s.browsers {
		b.stopAnnotation()
	}
}

// StopAnnotations suspends pickers when the workspace/browser is hidden, while
// retaining captured draft chips and their page markers.
func (s *State) StopAnnotations() {
	for _, b := range s.browsers {
		b.stopAnnotation()
	}
}
