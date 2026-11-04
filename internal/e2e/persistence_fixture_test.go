//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// Non-naming fixtures keep model request counts deterministic.
func openExecutionFixture(t *testing.T, options engine.Options) (*engine.Engine, error) {
	t.Helper()
	if options.Client == nil {
		options.Client = model.NewClient()
		retry := model.DefaultRetryPolicy()
		retry.Wait = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
		options.Client.RetryPolicy = &retry
	}
	core, err := engine.Open(options)
	if err != nil {
		return nil, err
	}
	snapshot, err := core.Snapshot(context.Background())
	if err == nil {
		settings := snapshot.Settings
		settings.General.ThreadNamingModel = "first-message"
		err = core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &settings})
	}
	if err != nil {
		_ = core.Close()
	}
	return core, err
}

type fixtureSessionRecord struct {
	Format        int                       `json:"format"`
	SessionID     string                    `json:"sessionId"`
	RootID        string                    `json:"rootId"`
	Session       contracts.Session         `json:"session"`
	Messages      []contracts.Message       `json:"messages"`
	Runs          []contracts.Run           `json:"runs"`
	ModelRequests []contracts.ModelRequest  `json:"modelRequests"`
	Queue         []contracts.QueuedMessage `json:"queue"`
	ContextUsage  contracts.ContextUsage    `json:"contextUsage"`
	Changes       []contracts.Change        `json:"changes"`
	Terminals     []contracts.Terminal      `json:"terminals"`
}

func readFixtureIndex(configDirectory string) (map[string]json.RawMessage, contracts.Snapshot, map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(configDirectory, "engine.json"))
	if err != nil {
		return nil, contracts.Snapshot{}, nil, err
	}
	var index map[string]json.RawMessage
	var snapshot contracts.Snapshot
	var records map[string]string
	if err = json.Unmarshal(data, &index); err == nil {
		err = json.Unmarshal(index["snapshot"], &snapshot)
	}
	if err == nil {
		err = json.Unmarshal(index["records"], &records)
	}
	return index, snapshot, records, err
}

func readFixtureSession(configDirectory string, session contracts.Session, record string) (string, []byte, fixtureSessionRecord, error) {
	var disk fixtureSessionRecord
	var location struct {
		Roots []storage.Root `json:"roots"`
	}
	data, err := os.ReadFile(filepath.Join(configDirectory, "location.json"))
	if err == nil {
		err = json.Unmarshal(data, &location)
	}
	if err != nil {
		return "", nil, disk, err
	}
	var directory string
	for _, root := range location.Roots {
		if root.ID == session.StorageRootID {
			directory = root.Directory
			break
		}
	}
	if directory == "" {
		return "", nil, disk, errors.New("fixture conversation root is not registered")
	}
	decoded, err := hex.DecodeString(record)
	if err != nil || len(decoded) != sha256.Size || filepath.Base(session.ID) != session.ID {
		return "", nil, disk, errors.New("fixture conversation has invalid storage identity")
	}
	directory = filepath.Join(directory, "sessions", session.ID)
	data, err = os.ReadFile(filepath.Join(directory, record+".json"))
	if err != nil {
		return "", nil, disk, err
	}
	checksum := sha256.Sum256(data)
	if hex.EncodeToString(checksum[:]) != record {
		return "", nil, disk, errors.New("fixture conversation checksum does not match its index")
	}
	if err := json.Unmarshal(data, &disk); err != nil {
		return "", nil, disk, err
	}
	if disk.SessionID != session.ID || disk.RootID != session.StorageRootID ||
		disk.Session.ID != session.ID || disk.Session.StorageRootID != session.StorageRootID {
		return "", nil, disk, errors.New("fixture conversation record does not match its index")
	}
	return directory, data, disk, nil
}

// Read committed records without opening a second host or bypassing its profile
// lock. A concurrent commit may retire a record; native await callers retry errors.
func readFixtureSnapshot(configDirectory string) (contracts.Snapshot, error) {
	_, snapshot, records, err := readFixtureIndex(configDirectory)
	if err != nil {
		return contracts.Snapshot{}, err
	}
	snapshot.Messages = make(map[string][]contracts.Message)
	snapshot.ContextUsage = make(map[string]contracts.ContextUsage)
	for i, session := range snapshot.Sessions {
		_, _, disk, err := readFixtureSession(configDirectory, session, records[session.ID])
		if err != nil {
			return contracts.Snapshot{}, err
		}
		snapshot.Sessions[i] = disk.Session
		snapshot.Messages[session.ID] = disk.Messages
		snapshot.ContextUsage[session.ID] = disk.ContextUsage
		snapshot.Runs = append(snapshot.Runs, disk.Runs...)
		snapshot.ModelRequests = append(snapshot.ModelRequests, disk.ModelRequests...)
		snapshot.Queue = append(snapshot.Queue, disk.Queue...)
		snapshot.Changes = append(snapshot.Changes, disk.Changes...)
		snapshot.Terminals = append(snapshot.Terminals, disk.Terminals...)
	}
	return snapshot, nil
}

// Display-only fixtures edit their stopped private host's current record.
// Unknown record/index fields, model history and context remain untouched.
func editFixtureConversation(t *testing.T, configDirectory, sessionID string, edit func(*contracts.Session, *[]contracts.Message)) {
	t.Helper()
	index, snapshot, records, err := readFixtureIndex(configDirectory)
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range snapshot.Sessions {
		if session.ID != sessionID {
			continue
		}
		directory, data, disk, err := readFixtureSession(configDirectory, session, records[sessionID])
		if err != nil {
			t.Fatal(err)
		}
		edit(&disk.Session, &disk.Messages)
		if disk.Session.ID != session.ID || disk.Session.StorageRootID != session.StorageRootID {
			t.Fatal("display fixture cannot change conversation storage identity")
		}
		var record map[string]json.RawMessage
		if err := json.Unmarshal(data, &record); err != nil {
			t.Fatal(err)
		}
		record["session"], err = json.Marshal(disk.Session)
		if err == nil {
			record["messages"], err = json.Marshal(disk.Messages)
		}
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		checksum := sha256.Sum256(encoded)
		name := hex.EncodeToString(checksum[:])
		if err := storage.WriteJSON(filepath.Join(directory, name+".json"), record); err != nil {
			t.Fatal(err)
		}
		records[sessionID] = name
		index["records"], err = json.Marshal(records)
		if err != nil {
			t.Fatal(err)
		}
		if err := storage.WriteJSON(filepath.Join(configDirectory, "engine.json"), index); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Fatalf("fixture conversation %q is absent", sessionID)
}

func TestFixtureConversationRecordRoundTrip(t *testing.T) {
	directory := t.TempDir()
	store, err := storage.Open(storage.Options{Directory: directory})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, engine.Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "session.create", Name: "Display fixture"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	session := snapshot.Sessions[0]
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	_, _, beforeRecords, err := readFixtureIndex(directory)
	if err != nil {
		t.Fatal(err)
	}
	_, beforeData, _, err := readFixtureSession(directory, session, beforeRecords[session.ID])
	if err != nil {
		t.Fatal(err)
	}
	editFixtureConversation(t, directory, session.ID, func(saved *contracts.Session, messages *[]contracts.Message) {
		saved.Goal = &contracts.SessionGoal{ID: "fixture-goal", Status: contracts.GoalPaused,
			Objective: "DISPLAY-ONLY GOAL OBJECTIVE", Reason: "DISPLAY-ONLY GOAL REASON"}
		*messages = []contracts.Message{{ID: "fixture-message", SessionID: saved.ID, Role: "assistant",
			Blocks: []contracts.Block{{ID: "fixture-todo", Kind: "todos",
				Todos: []contracts.Todo{{Text: "DISPLAY-ONLY TODO", Status: "pending"}}}}}}
	})
	_, indexed, afterRecords, err := readFixtureIndex(directory)
	if err != nil {
		t.Fatal(err)
	}
	if beforeRecords[session.ID] == afterRecords[session.ID] || indexed.Messages != nil || indexed.Sessions[0].Goal != nil {
		t.Fatal("fixture edited the small index instead of committing a new conversation record")
	}
	_, afterData, _, err := readFixtureSession(directory, session, afterRecords[session.ID])
	if err != nil {
		t.Fatal(err)
	}
	var before, after map[string]json.RawMessage
	if err := json.Unmarshal(beforeData, &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(afterData, &after); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"history", "context", "runs", "queue", "contextUsage", "changes", "terminals"} {
		if !bytes.Equal(before[name], after[name]) {
			t.Fatalf("display fixture changed unrelated record field %q", name)
		}
	}
	store, err = storage.Open(storage.Options{Directory: directory})
	if err != nil {
		t.Fatal(err)
	}
	core, err = openExecutionFixture(t, engine.Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	restored, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := restored.Sessions[0]
	if got.StorageError != "" || got.Goal == nil || got.Goal.Reason != "DISPLAY-ONLY GOAL REASON" ||
		len(restored.Messages[got.ID]) != 1 || restored.Messages[got.ID][0].Blocks[0].Todos[0].Text != "DISPLAY-ONLY TODO" {
		t.Fatalf("production core did not restore the display fixture record: %+v", got)
	}
}

func TestFixtureSnapshotReadsRegisteredHistoricalRoots(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "profile")
	store, err := storage.Open(storage.Options{Directory: profile})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, engine.Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "session.create", Name: "Historical"}); err != nil {
		t.Fatal(err)
	}
	before, _ := core.Snapshot(context.Background())
	old := before.Sessions[0]
	next := filepath.Join(t.TempDir(), "next")
	if err := store.RequestDirectory(next); err != nil {
		t.Fatal(err)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = storage.Open(storage.Options{Directory: profile})
	if err != nil {
		t.Fatal(err)
	}
	core, err = openExecutionFixture(t, engine.Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "session.create", Name: "Current"}); err != nil {
		t.Fatal(err)
	}
	current, _ := core.Snapshot(context.Background())
	fresh := current.Sessions[0]
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	for _, session := range []contracts.Session{old, fresh} {
		editFixtureConversation(t, profile, session.ID, func(saved *contracts.Session, messages *[]contracts.Message) {
			*messages = []contracts.Message{{ID: "fixture-" + saved.ID, SessionID: saved.ID, Role: "user",
				Blocks: []contracts.Block{{ID: "text-" + saved.ID, Kind: "text", Text: saved.Title}}}}
		})
	}
	restored, err := readFixtureSnapshot(profile)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.Messages[old.ID]) != 1 || restored.Messages[old.ID][0].Blocks[0].Text != "Historical" ||
		len(restored.Messages[fresh.ID]) != 1 || restored.Messages[fresh.ID][0].Blocks[0].Text != "Current" {
		t.Fatal("fixture reader did not reconstruct both registered roots")
	}
	_, _, records, err := readFixtureIndex(profile)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(next, "sessions", fresh.ID, records[fresh.ID]+".json")
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readFixtureSnapshot(profile); err == nil {
		t.Fatal("fixture reader silently accepted an uncommitted or corrupt record")
	}
}
