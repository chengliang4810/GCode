package workbench

import (
	"bytes"
	"errors"
	"fmt"
	"gcode/internal/design"
	"io"
	"strings"
	"sync"

	"gcode/internal/contracts"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

type terminalView struct {
	meta       contracts.Terminal
	native     *terminal.Terminal
	conn       *streamConn
	err        string
	loading    bool
	appearance string
	resource   closeSlot
	body       ui.Handle
	focusBody  bool
	live       bool
}

// closeSlot owns a resource even before its asynchronous UI delivery runs.
// Closing a tab/window therefore closes a late loader result exactly once.
type closeSlot struct {
	mu     sync.Mutex
	closed bool
	value  io.Closer
}

func (s *closeSlot) install(value io.Closer) bool {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = value.Close()
		return false
	}
	s.value = value
	s.mu.Unlock()
	return true
}

func (s *closeSlot) close() {
	s.mu.Lock()
	s.closed = true
	value := s.value
	s.value = nil
	s.mu.Unlock()
	if value != nil {
		_ = value.Close()
	}
}

// streamConn is a bounded in-memory terminal transport. It never invokes an OS
// process. Typed bytes are echoed locally; complete input lines become backend
// commands. Closing unblocks Read and makes future writes fail with io.ErrClosedPipe.
type streamConn struct {
	mu            sync.Mutex
	ready         *sync.Cond
	output        bytes.Buffer
	input         []byte
	closed        bool
	lastCR        bool
	escape        string
	columns, rows int
	onLine        func(string)
	onResize      func(int, int)
	echoed        []string
}

func newStreamConn(onLine func(string), onResize func(int, int)) *streamConn {
	c := &streamConn{onLine: onLine, onResize: onResize}
	c.ready = sync.NewCond(&c.mu)
	return c
}

func (c *streamConn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for c.output.Len() == 0 && !c.closed {
		c.ready.Wait()
	}
	if c.output.Len() == 0 {
		return 0, io.EOF
	}
	return c.output.Read(p)
}

func (c *streamConn) Write(data []byte) (int, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return 0, io.ErrClosedPipe
	}
	if c.output.Len()+len(data) > 8*1024*1024 || len(c.input)+len(data) > 1024*1024 {
		c.mu.Unlock()
		return 0, errors.New("terminal input queue limit exceeded")
	}
	var lines []string
	for _, b := range data {
		if c.escape != "" {
			if c.escape == "\x1b" && b != '[' && b != 'O' && b != ']' {
				// Alt sends ESC plus an ordinary key. Drop the modifier prefix,
				// preserving this key and the text typed after it.
				c.escape = ""
			} else {
				c.escape += string(b)
				osc := strings.HasPrefix(c.escape, "\x1b]")
				if len(c.escape) > 2 && ((!osc && b >= 0x40 && b <= 0x7e) || (osc && (b == 7 || strings.HasSuffix(c.escape, "\x1b\\")))) {
					c.escape = ""
				}
				if len(c.escape) > 1024 {
					c.escape = ""
				}
				continue
			}
		}
		if b == 27 {
			c.escape = "\x1b"
			continue
		}
		if b == '\n' && c.lastCR {
			c.lastCR = false
			continue
		}
		c.lastCR = b == '\r'
		switch b {
		case '\r', '\n':
			line := string(c.input)
			c.input = nil
			c.echoed = append(c.echoed, line+"\r\n")
			lines = append(lines, line)
			c.output.WriteString("\r\n")
		case 127, 8:
			if len(c.input) > 0 {
				i := len(c.input) - 1
				for i > 0 && c.input[i]&0xc0 == 0x80 {
					i--
				}
				c.input = c.input[:i]
				c.output.WriteString("\b \b")
			}
		case 3:
			c.input = nil
			c.output.WriteString("^C\r\n")
			lines = append(lines, "")
			c.echoed = append(c.echoed, "\r\n")
		default:
			if b >= 32 || b == '\t' {
				c.input = append(c.input, b)
				c.output.WriteByte(b)
			}
		}
	}
	c.ready.Signal()
	c.mu.Unlock()
	for _, line := range lines {
		if c.onLine != nil {
			c.onLine(line)
		}
	}
	return len(data), nil
}

func (c *streamConn) push(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return io.ErrClosedPipe
	}
	if c.output.Len()+len(data) > 8*1024*1024 {
		return errors.New("terminal output queue exceeded 8 MiB")
	}
	c.output.Write(data)
	c.ready.Signal()
	return nil
}

func (c *streamConn) stripEcho(delta string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	for len(c.echoed) > 0 && strings.HasPrefix(delta, c.echoed[0]) {
		delta = strings.TrimPrefix(delta, c.echoed[0])
		c.echoed = c.echoed[1:]
	}
	return delta
}

func (c *streamConn) Resize(columns, rows int) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return io.ErrClosedPipe
	}
	if columns < 1 || rows < 1 {
		c.mu.Unlock()
		return fmt.Errorf("terminal dimensions must be positive")
	}
	if columns == c.columns && rows == c.rows {
		c.mu.Unlock()
		return nil
	}
	c.columns, c.rows = columns, rows
	c.mu.Unlock()
	if c.onResize != nil {
		c.onResize(columns, rows)
	}
	return nil
}

func (c *streamConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	c.ready.Broadcast()
	return nil
}

func (s *State) openTerminal(meta contracts.Terminal) {
	if s.closed.Load() {
		return
	}
	s.choose("terminal", meta.ID, meta.Title)
	if s.terminals[meta.ID] != nil {
		return
	}
	v := &terminalView{meta: meta, loading: s.win != nil, focusBody: true, live: s.snapshot.Runtime != nil}
	s.terminals[meta.ID] = v
	if s.win == nil {
		v.err = "A native window is required to display the terminal."
		return
	}
	post := func(command contracts.Command) {
		s.win.Update(func() {
			if !s.closed.Load() && s.terminals[meta.ID] == v {
				s.command(command)
			}
		})
	}
	if !v.live {
		v.conn = newStreamConn(func(line string) { post(contracts.Command{Kind: "terminal.input", ID: meta.ID, Text: line}) }, func(cols, rows int) {
			post(contracts.Command{Kind: "terminal.resize", ID: meta.ID, TerminalSize: &contracts.TerminalSize{Columns: cols, Rows: rows}})
		})
	}
	font := design.MonoFont(s.snapshot.Settings.Appearance.TerminalFont)
	fontSize := float32(s.snapshot.Settings.Appearance.TerminalFontSize)
	if font == "" {
		font = "Geist Mono"
	}
	if fontSize <= 0 {
		fontSize = 13
	}
	go func() {
		if s.closed.Load() {
			return
		}
		var conn io.ReadWriteCloser
		var err error
		if v.live {
			service, ok := s.backend.(contracts.TerminalService)
			if !ok {
				err = errors.New("terminal streaming is unavailable")
			} else {
				conn, err = service.OpenTerminal(s.ctx, meta.ID)
			}
		} else {
			conn = v.conn
		}
		if err != nil {
			s.win.Update(func() {
				if !s.closed.Load() && s.terminals[meta.ID] == v {
					v.loading = false
					v.err = err.Error()
				}
			})
			return
		}
		if s.closed.Load() {
			_ = conn.Close()
			return
		}
		options := terminal.Options{Conn: conn, Font: terminal.Font{Family: font, Size: fontSize}, NoBlink: true, NoClipboardWrite: true, Scrollback: 4 * 1024 * 1024}
		if v.live {
			options.OnClear = func(terminal.Clear) bool { post(contracts.Command{Kind: "terminal.clear", ID: meta.ID}); return true }
		}
		native, err := terminal.New(options)
		if native != nil && !v.resource.install(native) {
			return
		}
		if err != nil {
			_ = conn.Close()
		}
		s.win.Update(func() {
			if s.closed.Load() || s.terminals[meta.ID] != v {
				v.resource.close()
				return
			}
			v.loading = false
			if err != nil {
				v.err = err.Error()
				return
			}
			v.native = native
			s.configureTerminal(v)
			if !v.live {
				if err := v.conn.push([]byte(v.meta.Output)); err != nil {
					v.err = err.Error()
				}
			}
		})
	}()
}

func (s *State) configureTerminal(v *terminalView) {
	if v.native == nil {
		return
	}
	a := s.snapshot.Settings.Appearance
	p := s.palette()
	key := fmt.Sprintf("%s:%s:%s:%s:%g", p.ID, a.Mode, a.Accent, a.TerminalFont, a.TerminalFontSize)
	if v.appearance == key {
		return
	}
	v.appearance = key
	theme := &terminal.Theme{Foreground: p.TerminalForeground, Background: p.TerminalBackground, Selection: p.TerminalSelection, Cursor: p.Caret, Palette: p.ANSI}
	v.native.SetTheme(theme, theme)
	font := design.MonoFont(a.TerminalFont)
	if font == "" {
		font = "Geist Mono"
	}
	size := float32(a.TerminalFontSize)
	if size <= 0 {
		size = 13
	}
	v.native.SetFont(terminal.Font{Family: font, Size: size})
}

func (v *terminalView) update(meta contracts.Terminal) {
	previous := v.meta.Output
	v.meta = meta
	if v.live {
		return
	}
	if v.native == nil || previous == meta.Output {
		return
	}
	delta := meta.Output
	if strings.HasPrefix(meta.Output, previous) {
		delta = strings.TrimPrefix(meta.Output, previous)
		delta = v.conn.stripEcho(delta)
	} else {
		v.conn.mu.Lock()
		v.conn.echoed = nil
		v.conn.mu.Unlock()
		delta = "\x1b[2J\x1b[H" + delta
	}
	if err := v.conn.push([]byte(delta)); err != nil {
		v.err = err.Error()
	}
}

func (v *terminalView) close() {
	if v.conn != nil {
		_ = v.conn.Close()
	}
	v.resource.close()
	v.native = nil
}

func (s *State) terminalView(c *ui.Context, id string) {
	v := s.terminals[id]
	if v == nil {
		return
	}
	p := s.palette()
	if v.meta.StorageError != "" {
		ui.Text(c.Key("terminal.storage.error"), v.meta.StorageError).TextColor(p.Danger).Selectable()
	}
	if v.loading {
		ui.Column(c).Grow(1).FillWidth().Center().Children(func() { ui.Spinner(c) })
		return
	}
	if v.err != "" {
		ui.Column(c).Grow(1).FillWidth().Center().Gap(12).Padding(20).Children(func() {
			ui.Text(c, tr(c, "Unable to open terminal")).FontSize(design.TextSize(c, design.TextLarge))
			ui.Text(c, v.err).TextColor(p.Muted).MaxWidth(460)
		})
		return
	}
	if v.native != nil {
		body := terminal.View(c, v.native).Bind(&v.body).Grow(1).FillWidth()
		if v.focusBody {
			body.Focus()
			v.focusBody = false
		}
	}
}
