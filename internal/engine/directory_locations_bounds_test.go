package engine

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
)

func TestDirectoryLocationsResultLimitAndOrder(t *testing.T) {
	for _, count := range []int{498, 499, 500} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			b := testDirectoryBrowser()
			b.timeout = time.Second
			roots := []string{"/home"} // The home root must not appear twice.
			for i := range count {
				roots = append(roots, fmt.Sprintf("/volume-%04d", i))
			}
			slices.Sort(roots)
			b.roots = func() []string { return roots }
			got, err := b.listDirectoryLocations(context.Background(), contracts.DirectoryRequest{DeviceID: "local"})
			if err != nil || len(got.Locations) != min(count+1, contracts.DirectoryResultLimit) || got.Truncated != (count+1 > contracts.DirectoryResultLimit) {
				t.Fatalf("location cap: count=%d, locations=%d, truncated=%v, err=%v", count, len(got.Locations), got.Truncated, err)
			}
			if got.Locations[0].Kind != contracts.LocationHome || got.Locations[0].Path != "/home" {
				t.Fatalf("home is not first: %+v", got.Locations[0])
			}
			if !slices.IsSortedFunc(got.Locations[1:], func(a, b contracts.DirectoryLocation) int { return strings.Compare(a.Path, b.Path) }) {
				t.Fatal("volume paths are not sorted")
			}
			for _, location := range got.Locations[1:] {
				if location.Path == "/home" || location.Kind != contracts.LocationVolume {
					t.Fatalf("invalid or duplicate location: %+v", location)
				}
			}
			waitBrowsePermit(t, b)
		})
	}
}
