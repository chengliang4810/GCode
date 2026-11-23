package engine

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
)

func TestDirectoryBrowseLimitsAndSorting(t *testing.T) {
	for _, tc := range []struct {
		name        string
		directories int
		files       int
		truncated   bool
	}{
		{name: "empty"},
		{name: "below result limit", directories: 499},
		{name: "exact result limit", directories: 500},
		{name: "above result limit", directories: 501, truncated: true},
		{name: "scan limit counts files", files: contracts.DirectoryScanLimit + 100, truncated: true},
		{name: "exact scan limit is conservative", files: contracts.DirectoryScanLimit, truncated: true},
		{name: "below scan limit", files: contracts.DirectoryScanLimit - 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := testDirectoryBrowser()
			b.timeout = 2 * time.Second
			var entries []fs.DirEntry
			for i := tc.directories; i > 0; i-- {
				name := fmt.Sprintf("dir-%04d", i)
				if i == 1 {
					name = ".hidden"
				}
				entries = append(entries, browseEntry{name: name, dir: true})
			}
			for i := range tc.files {
				entries = append(entries, browseEntry{name: fmt.Sprintf("file-%05d", i)})
			}
			read, closed := 0, false
			b.open = func(string) (directoryReader, error) {
				return browseTestReader{
					read: func(n int) ([]fs.DirEntry, error) {
						if n <= 0 || n > directoryReadBatch || n+read > contracts.DirectoryScanLimit {
							return nil, fmt.Errorf("unbounded ReadDir(%d), scanned %d", n, read)
						}
						count := min(n, len(entries))
						batch := entries[:count]
						entries = entries[count:]
						read += count
						if count == 0 {
							return batch, io.EOF
						}
						return batch, nil
					},
					close: func() error { closed = true; return nil },
				}, nil
			}
			got, err := b.listDirectories(context.Background(), contracts.DirectoryRequest{Path: "~"})
			if err != nil {
				t.Fatal(err)
			}
			waitBrowsePermit(t, b)
			if !closed || got.Truncated != tc.truncated || len(got.Entries) != min(tc.directories, contracts.DirectoryResultLimit) {
				t.Fatalf("entries=%d truncated=%v closed=%v", len(got.Entries), got.Truncated, closed)
			}
			if read > contracts.DirectoryScanLimit {
				t.Fatalf("scanned %d entries", read)
			}
			if got.Path != "/home" || got.Home != "/home" || got.DeviceID != "local" {
				t.Fatalf("path metadata: %+v", got)
			}
			if !slices.IsSortedFunc(got.Entries, func(a, b contracts.DirectoryEntry) int { return strings.Compare(a.Path, b.Path) }) {
				t.Fatal("entries not path sorted")
			}
			for _, entry := range got.Entries {
				if entry.Path != browseEntryPath(entry.Name) || entry.Hidden != strings.HasPrefix(entry.Name, ".") {
					t.Fatalf("invalid entry: %+v", entry)
				}
			}
		})
	}
}
