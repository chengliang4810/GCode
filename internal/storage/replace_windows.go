package storage

import (
	"errors"
	"os"
	"syscall"
	"time"
)

const sharingViolation syscall.Errno = 32

func replaceFile(source, target string) error {
	// Windows readers opened without FILE_SHARE_DELETE can briefly prevent
	// replacement. Keep the old file intact and retry the atomic rename.
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		err := os.Rename(source, target)
		if !errors.Is(err, syscall.ERROR_ACCESS_DENIED) && !errors.Is(err, sharingViolation) {
			return err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return err
		}
		time.Sleep(min(10*time.Millisecond, remaining))
	}
}
