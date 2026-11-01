package contracts

import (
	"context"
	"io"
)

// TerminalStream attaches a view to a host-owned process. Closing a stream
// detaches that view; terminal.close explicitly ends the process.
type TerminalStream interface {
	io.ReadWriteCloser
	Resize(columns, rows int) error
}

type TerminalService interface {
	OpenTerminal(context.Context, string) (TerminalStream, error)
}

// TerminalInputService writes directly to the host without attaching a view.
type TerminalInputService interface {
	WriteTerminal(context.Context, string, []byte) (int, error)
}
