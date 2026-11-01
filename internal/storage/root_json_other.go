//go:build !windows

package storage

import "os"

func replaceJSONRoot(root *os.Root, source, target string) error {
	return root.Rename(source, target)
}
