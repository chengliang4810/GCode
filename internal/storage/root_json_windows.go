package storage

import (
	"errors"
	"os"
	"syscall"
	"time"
)

func replaceJSONRoot(root *os.Root, source, target string) error {
	// Windows 读句柄可短暂禁止替换；重试期间保留旧文件，绝不先删后写。
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		err := root.Rename(source, target)
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
