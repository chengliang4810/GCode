package storage

import "os"

// Directory FlushFileBuffers is unavailable through os.Root on Windows.
// Namespace operations rely on the filesystem; no power-loss guarantee is made.
func syncDirectoryRoot(_ *os.Root) error { return nil }
