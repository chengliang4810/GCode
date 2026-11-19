//go:build !unix && !windows

package engine

import (
	"errors"
	"os"
)

func openFileSaveSource(_ *os.Root, _ string) (*os.File, error) {
	return nil, errors.New("safe editor saves are not supported on this platform")
}

func checkFileSaveLinks(_ *os.File, _ os.FileInfo) error {
	return errors.New("cannot determine file hard-link count on this platform")
}
