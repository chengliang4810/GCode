package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func mustOpen(t *testing.T, options Options) *Store {
	t.Helper()
	s, err := Open(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func TestDirectoryChangeDoesNotCopyAndConfigurationStaysFixed(t *testing.T) {
	home := t.TempDir()
	options := Options{Home: home}
	s := mustOpen(t, options)
	original, originalID := s.Directory(), s.ActiveRootID()
	if original != filepath.Join(home, ".gcode") || s.ConfigDirectory() != original {
		t.Fatal("unexpected default directories")
	}
	if err := s.Save("connection.json", map[string]string{"token": "kept"}); err != nil {
		t.Fatal(err)
	}
	history := filepath.Join(original, "sessions", "one.json")
	if err := WriteJSON(history, "complete history"); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, "custom")
	if err := s.RequestDirectory(target); err != nil {
		t.Fatal(err)
	}
	if s.Directory() != original || s.Pending() != target {
		t.Fatal("active root changed before restart")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = mustOpen(t, options)
	if s.Directory() != target || s.Pending() != "" || s.ConfigDirectory() != original {
		t.Fatal("pending activation changed the configuration entrance")
	}
	if s.ActiveRootID() == originalID {
		t.Fatal("new directory reused the original identity")
	}
	for _, name := range []string{"sessions", "connection.json"} {
		if _, err := os.Stat(filepath.Join(target, name)); !os.IsNotExist(err) {
			t.Fatalf("data was copied to new root: %s: %v", name, err)
		}
	}
	if _, err := os.Stat(history); err != nil {
		t.Fatal("source history was removed")
	}
	if err := s.Save("connection.json", map[string]string{"token": "updated"}); err != nil {
		t.Fatal(err)
	}
	var connection map[string]string
	data, err := os.ReadFile(filepath.Join(original, "connection.json"))
	if err != nil || json.Unmarshal(data, &connection) != nil || connection["token"] != "updated" {
		t.Fatalf("fixed configuration did not update: %s %v", data, err)
	}
	if directory, err := s.Resolve(originalID); err != nil || directory != original {
		t.Fatalf("historical root cannot be resolved: %s %v", directory, err)
	}
	if err := s.RequestDirectory(original); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s = mustOpen(t, options)
	if s.ActiveRootID() != originalID || s.Directory() != original || len(s.Roots()) != 2 {
		t.Fatal("registered identity was not retained on switch back")
	}
}

func TestRejectDirectoryCollisionAndCancelPending(t *testing.T) {
	home := t.TempDir()
	s := mustOpen(t, Options{Home: home})
	for _, path := range []string{"", "relative", home, filepath.Join(s.Directory(), "child")} {
		if err := s.RequestDirectory(path); err == nil {
			t.Fatalf("accepted unsafe directory %q", path)
		}
	}
	target := filepath.Join(home, "occupied")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "keep"), []byte("user file"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.RequestDirectory(target); err == nil {
		t.Fatal("accepted an occupied destination")
	}
	if _, err := os.Stat(filepath.Join(target, rootLockFile)); !os.IsNotExist(err) {
		t.Fatal("rejected unrelated directory was modified")
	}
	if err := s.RequestDirectory(filepath.Join(home, "new")); err != nil {
		t.Fatal(err)
	}
	if err := s.RequestDirectory(s.Directory()); err != nil || s.Pending() != "" {
		t.Fatal("could not cancel a pending change")
	}
}

func TestPendingUnavailableKeepsOriginalAndReportsFailure(t *testing.T) {
	home := t.TempDir()
	options := Options{Home: home}
	s := mustOpen(t, options)
	source := s.Directory()
	target := filepath.Join(home, "destination")
	if err := s.RequestDirectory(target); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	if err := os.Rename(target, filepath.Join(home, "offline")); err != nil {
		t.Fatal(err)
	}
	s = mustOpen(t, options)
	if s.Directory() != source || s.Pending() != target || s.ChangeError() == "" {
		t.Fatal("failed selection was silently activated or lost")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("offline root was silently recreated")
	}
	if err := s.RequestDirectory(source); err != nil || s.Pending() != "" || s.ChangeError() != "" {
		t.Fatal("could not cancel a failed pending change")
	}
}

func TestHistoricalRootOfflineAndIdentityMismatchStayVisible(t *testing.T) {
	home := t.TempDir()
	options := Options{Home: home, Directory: filepath.Join(home, "config")}
	s := mustOpen(t, options)
	firstData := filepath.Join(home, "data-a")
	if err := s.RequestDirectory(firstData); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s = mustOpen(t, options)
	firstID := s.ActiveRootID()
	if err := s.RequestDirectory(filepath.Join(home, "data-b")); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s = mustOpen(t, options)
	if err := os.Rename(firstData, firstData+"-offline"); err != nil {
		t.Fatal(err)
	}
	if root, ok := s.Root(firstID); !ok || root.Directory != firstData {
		t.Fatal("offline root registration disappeared")
	}
	if _, err := s.Resolve(firstID); err == nil {
		t.Fatal("offline root resolved")
	}
	if _, err := os.Stat(firstData); !os.IsNotExist(err) {
		t.Fatal("offline root was recreated")
	}
	if err := os.Rename(firstData+"-offline", firstData); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(filepath.Join(firstData, rootMarkerFile), rootMarker{Version: rootVersion, ID: "wrong"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(firstID); err == nil {
		t.Fatal("root marker mismatch was ignored")
	}
	if err := s.RequestDirectory(firstData); err == nil {
		t.Fatal("mismatched historical root was selected")
	}
	roots := s.Roots()
	roots[0].Directory = "changed"
	if s.Roots()[0].Directory == "changed" {
		t.Fatal("Roots exposed writable registration state")
	}
}

func TestActiveRootUnavailableDoesNotFallBackOrRecreate(t *testing.T) {
	for _, cause := range []string{"offline", "identity"} {
		t.Run(cause, func(t *testing.T) {
			home := t.TempDir()
			options := Options{Home: home}
			s := mustOpen(t, options)
			target := filepath.Join(home, "active")
			if err := s.RequestDirectory(target); err != nil {
				t.Fatal(err)
			}
			_ = s.Close()
			s = mustOpen(t, options)
			_ = s.Close()
			if cause == "offline" {
				if err := os.Rename(target, target+"-offline"); err != nil {
					t.Fatal(err)
				}
			} else if err := WriteJSON(filepath.Join(target, rootMarkerFile), rootMarker{Version: rootVersion, ID: "wrong"}); err != nil {
				t.Fatal(err)
			}
			if other, err := Open(options); err == nil {
				_ = other.Close()
				t.Fatal("unavailable active root silently fell back")
			}
			if cause == "offline" {
				if _, err := os.Stat(target); !os.IsNotExist(err) {
					t.Fatal("missing active root was recreated")
				}
			}
		})
	}
}

func TestPendingRootCanActivateWhilePreviousDataRootIsOffline(t *testing.T) {
	home := t.TempDir()
	options := Options{Home: home}
	s := mustOpen(t, options)
	oldPath := filepath.Join(home, "old-data")
	if err := s.RequestDirectory(oldPath); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s = mustOpen(t, options)
	oldID := s.ActiveRootID()
	newPath := filepath.Join(home, "new-data")
	if err := s.RequestDirectory(newPath); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	if err := os.Rename(oldPath, oldPath+"-offline"); err != nil {
		t.Fatal(err)
	}
	s = mustOpen(t, options)
	if s.Directory() != newPath || s.Pending() != "" || s.ChangeError() != "" {
		t.Fatal("offline historical root blocked activation")
	}
	if _, err := s.Resolve(oldID); err == nil {
		t.Fatal("offline historical root resolved")
	}
	if root, ok := s.Root(oldID); !ok || root.Directory != oldPath {
		t.Fatal("offline historical registration was lost")
	}
}

func TestUnavailablePendingAndActiveRootsFailWithoutCreatingEither(t *testing.T) {
	home := t.TempDir()
	options := Options{Home: home}
	s := mustOpen(t, options)
	active := filepath.Join(home, "active")
	pending := filepath.Join(home, "pending")
	if err := s.RequestDirectory(active); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s = mustOpen(t, options)
	if err := s.RequestDirectory(pending); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	for _, path := range []string{active, pending} {
		if err := os.Rename(path, path+"-offline"); err != nil {
			t.Fatal(err)
		}
	}
	if opened, err := Open(options); err == nil {
		_ = opened.Close()
		t.Fatal("neither selected root is available, but Open succeeded")
	}
	for _, path := range []string{active, pending} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("offline root was recreated")
		}
	}
}

func TestConfigurationAndResolvedRootLocksAreDistinct(t *testing.T) {
	home := t.TempDir()
	options := Options{Home: home}
	s := mustOpen(t, options)
	if other, err := Open(options); err == nil {
		_ = other.Close()
		t.Fatal("second configuration writer acquired lock")
	}
	if lock, err := lockAt(s.Directory(), rootLockFile); err == nil {
		_ = lock.Close()
		t.Fatal("active root is not locked")
	}
	if err := s.RequestDirectory(filepath.Join(home, "history")); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s = mustOpen(t, options)
	old := s.Roots()[0]
	// Open 只锁当前/待启用根，历史根在首次读取时才锁定。
	lock, err := lockAt(old.Directory, rootLockFile)
	if err != nil {
		t.Fatal("historical root was locked eagerly", err)
	}
	if _, err := s.Resolve(old.ID); err == nil {
		_ = lock.Close()
		t.Fatal("Resolve ignored another root writer")
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(old.ID); err != nil {
		t.Fatal(err)
	}
	if lock, err := lockAt(old.Directory, rootLockFile); err == nil {
		_ = lock.Close()
		t.Fatal("Resolve did not retain the root lock")
	}
	_ = s.Close()
	s = mustOpen(t, options)
	if _, err := s.Resolve(old.ID); err != nil {
		t.Fatal("Close did not release root locks", err)
	}
}

func TestExplicitProfileIsIndependentAndSaveRejectsReservedPaths(t *testing.T) {
	home := t.TempDir()
	profile := filepath.Join(home, "profile")
	options := Options{Home: home, Directory: profile}
	s := mustOpen(t, options)
	target := filepath.Join(home, "chosen")
	if err := s.RequestDirectory(target); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s = mustOpen(t, options)
	if s.Directory() != target || s.ConfigDirectory() != profile {
		t.Fatal("explicit profile discarded the saved directory choice")
	}
	if _, err := os.Stat(filepath.Join(home, ".gcode")); !os.IsNotExist(err) {
		t.Fatal("isolated profile changed the default entrance")
	}
	for _, name := range []string{"", ".", "../outside", `sub\file`, locationFile, rootMarkerFile, rootLockFile, configLockFile, "C:outside"} {
		if err := s.Save(name, "no"); err == nil {
			t.Fatalf("Save accepted reserved or nonlocal name %q", name)
		}
	}
	_ = s.Close()
	if _, err := s.Resolve(s.ActiveRootID()); err == nil {
		t.Fatal("closed store resolved a root")
	}
	if err := s.Save("connection.json", "no"); err == nil {
		t.Fatal("closed store wrote configuration")
	}
}

func TestOldLocationAndUnregisteredDataAreRejectedWithoutOverwrite(t *testing.T) {
	for _, kind := range []string{"location", "data"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			anchor := filepath.Join(home, ".gcode")
			name, value := "engine.json", `{"history":"keep"}`
			if kind == "location" {
				name, value = locationFile, `{"directory":"`+filepath.ToSlash(anchor)+`"}`
			}
			if err := os.MkdirAll(anchor, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(anchor, name)
			if err := os.WriteFile(path, []byte(value), 0600); err != nil {
				t.Fatal(err)
			}
			if s, err := Open(Options{Home: home}); err == nil {
				_ = s.Close()
				t.Fatal("old storage was silently accepted")
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != value {
				t.Fatal("old data was overwritten")
			}
		})
	}
}

func TestSymlinkStorageBoundariesAreRejected(t *testing.T) {
	home := t.TempDir()
	target, link := filepath.Join(home, "actual"), filepath.Join(home, "link")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink creation is not available: %v", err)
	}
	s := mustOpen(t, Options{Home: home})
	for _, path := range []string{link, filepath.Join(link, "nested")} {
		if err := s.RequestDirectory(path); err == nil {
			t.Fatalf("accepted symlink boundary %s", path)
		}
	}
	if other, err := Open(Options{Home: home, Directory: link}); err == nil {
		_ = other.Close()
		t.Fatal("configuration entrance followed a symlink")
	}
}

func TestRegistryAndMarkerRejectInvalidMetadata(t *testing.T) {
	for _, corrupt := range []string{"duplicate", "unknown-active", "unknown-pending", "relative", "overlap", "marker-version", "marker-missing"} {
		t.Run(corrupt, func(t *testing.T) {
			home := t.TempDir()
			options := Options{Home: home}
			s := mustOpen(t, options)
			anchor, rootID := s.ConfigDirectory(), s.ActiveRootID()
			loc := s.location
			_ = s.Close()
			switch corrupt {
			case "duplicate":
				loc.Roots = append(loc.Roots, loc.Roots[0])
			case "unknown-active":
				loc.ActiveRootID = "unknown"
			case "unknown-pending":
				loc.PendingRootID = "unknown"
			case "relative":
				loc.Roots[0].Directory = "relative"
			case "overlap":
				loc.Roots = append(loc.Roots, Root{ID: "nested", Directory: filepath.Join(anchor, "child")})
			case "marker-version":
				if err := WriteJSON(filepath.Join(anchor, rootMarkerFile), rootMarker{Version: 99, ID: rootID}); err != nil {
					t.Fatal(err)
				}
			case "marker-missing":
				if err := os.Remove(filepath.Join(anchor, rootMarkerFile)); err != nil {
					t.Fatal(err)
				}
			}
			if err := WriteJSON(filepath.Join(anchor, locationFile), loc); err != nil {
				t.Fatal(err)
			}
			if opened, err := Open(options); err == nil {
				_ = opened.Close()
				t.Fatalf("accepted invalid %s metadata", corrupt)
			}
			if corrupt == "marker-missing" {
				if _, err := os.Stat(filepath.Join(anchor, rootMarkerFile)); !os.IsNotExist(err) {
					t.Fatal("missing registered marker was recreated")
				}
			}
		})
	}
}
