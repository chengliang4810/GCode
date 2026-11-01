package plugins

import (
	"errors"
	"os"
	"runtime"
	"syscall"
	"time"
)

// renameWithRetry moves a freshly written tree or registry file into place.
// Windows can transiently deny the rename while antivirus or the system still
// holds a handle on the just-closed source or target; a short bounded retry
// keeps that race from failing a real install without hiding other errors.
func renameWithRetry(oldname, newname string) error {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
		}
		err = renameFunc(oldname, newname)
		if err == nil || !transientRenameError(err) {
			return err
		}
	}
	return err
}

var renameFunc = os.Rename

// transientRenameError reports whether Windows refused the rename only while
// another component still held a handle on the just-written source or target;
// that condition clears on its own. Other platforms and persistent failures
// such as real ACL problems return false so the first attempt result stands.
func transientRenameError(err error) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	switch errno {
	case 5, 32, 33: // ERROR_ACCESS_DENIED, ERROR_SHARING_VIOLATION, ERROR_LOCK_VIOLATION
		return true
	default:
		return false
	}
}
