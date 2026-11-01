package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"gcode/internal/contracts"
	"gcode/internal/storage"
	"gcode/internal/terminalhost"
)

const (
	terminalOutputFormat            = 1
	terminalOutputIncompleteMessage = "Some terminal output could not be saved; the displayed history may be incomplete."
	// 控制字符的 JSON 转义会放大正文；限制损坏文件的读取量，不截断合法历史。
	maxTerminalRecordBytes = 8*terminalhost.HistoryLimit + 1<<20
)

type terminalOutputDisk struct {
	Format     int    `json:"format"`
	TerminalID string `json:"terminalId"`
	RootID     string `json:"rootId"`
	Output     string `json:"output"`
}

// persistStandaloneTerminals stages immutable output and changes only the index
// projection. The caller commits its references after engine.json succeeds.
func (e *Engine) persistStandaloneTerminals(snapshot *contracts.Snapshot) error {
	if snapshot == nil {
		return errors.New("terminal index snapshot is required")
	}
	snapshot.Terminals = slices.Clone(snapshot.Terminals)
	for i := range snapshot.Terminals {
		item := &snapshot.Terminals[i]
		if item.SessionID != "" {
			continue
		}
		var ref string
		var err error
		restoring := item.Output == "" && item.OutputRef != ""
		if restoring {
			// 离线载入时的空投影不能作为真实空历史覆盖已提交引用。
			var output string
			output, err = e.readTerminalOutput(*item)
			ref = item.OutputRef
			if err == nil {
				e.setTerminalStorage(item.ID, output, "")
			}
		} else {
			ref, err = e.writeTerminalOutput(*item)
		}
		if err != nil {
			// 独立终端的离线根只影响该历史，其他根的会话和配置仍可提交。
			// 单纯读取离线旧引用失败不代表曾丢失输出，不能永久标成不完整。
			if !restoring {
				item.OutputIncomplete = true
			}
			e.setTerminalSaveState(item.ID, item.OutputIncomplete, err)
		} else {
			// 重启载入的旧引用仍然缺少未保存的尾部；只有真正的新输出成功
			// 形成不同引用，才能证明当前运行态的正文已经完整写入。
			item.OutputIncomplete = item.OutputIncomplete && ref == item.OutputRef
			item.OutputRef = ref
			e.setTerminalSaveState(item.ID, item.OutputIncomplete, nil)
		}
		item.Output, item.StorageError = "", ""
		if item.OutputIncomplete {
			item.StorageError = terminalOutputIncompleteMessage
		}
	}
	return nil
}

// commitStandaloneTerminals runs only after the fixed index was replaced.
// Until then the previous references and their immutable files remain valid.
func (e *Engine) commitStandaloneTerminals(snapshot *contracts.Snapshot) {
	if snapshot == nil {
		return
	}
	for _, saved := range snapshot.Terminals {
		if saved.SessionID != "" {
			continue
		}
		for i := range e.state.Terminals {
			current := &e.state.Terminals[i]
			if current.ID != saved.ID || current.SessionID != "" || current.StorageRootID != saved.StorageRootID {
				continue
			}
			old := current.OutputRef
			current.OutputRef = saved.OutputRef
			current.OutputIncomplete = saved.OutputIncomplete
			if old != "" && old != saved.OutputRef && !e.terminalRefInUse(saved.StorageRootID, old) {
				e.removeTerminalOutput(saved.StorageRootID, old)
			}
			break
		}
	}
}

// loadStandaloneTerminals reconstructs display history without starting shells.
// An unavailable root remains visible as a per-terminal error.
func (e *Engine) loadStandaloneTerminals() error {
	for i := range e.state.Terminals {
		item := &e.state.Terminals[i]
		if item.SessionID != "" {
			continue
		}
		output, err := e.readTerminalOutput(*item)
		if err != nil {
			item.Output, item.StorageError = "", err.Error()
			continue
		}
		item.Output, item.StorageError = output, ""
		if item.OutputIncomplete {
			item.StorageError = terminalOutputIncompleteMessage
		}
	}
	return nil
}

func (e *Engine) setTerminalStorage(id, output, message string) {
	for i := range e.state.Terminals {
		if e.state.Terminals[i].ID == id {
			e.state.Terminals[i].Output = output
			e.state.Terminals[i].StorageError = message
			return
		}
	}
}

func (e *Engine) setTerminalSaveState(id string, incomplete bool, err error) {
	for i := range e.state.Terminals {
		if e.state.Terminals[i].ID == id {
			e.state.Terminals[i].OutputIncomplete = incomplete
			e.state.Terminals[i].StorageError = ""
			if incomplete {
				e.state.Terminals[i].StorageError = terminalOutputIncompleteMessage
			}
			if err != nil {
				e.state.Terminals[i].StorageError = err.Error()
			}
			return
		}
	}
}

func (e *Engine) writeTerminalOutput(item contracts.Terminal) (string, error) {
	if item.StorageRootID == "" {
		return "", errors.New("terminal storage root is missing")
	}
	if item.OutputRef != "" && !validRecordHash(item.OutputRef) {
		return "", errors.New("terminal output reference is invalid")
	}
	if _, err := e.storage.Resolve(item.StorageRootID); err != nil {
		return "", err
	}
	if item.Output == "" && item.OutputRef == "" {
		return "", nil
	}
	disk := terminalOutputDisk{
		Format: terminalOutputFormat, TerminalID: item.ID, RootID: item.StorageRootID, Output: item.Output,
	}
	data, err := json.MarshalIndent(disk, "", "  ")
	if err != nil {
		return "", err
	}
	if len(data) > maxTerminalRecordBytes {
		return "", errors.New("terminal output record exceeds its size limit")
	}
	hash := sha256.Sum256(data)
	ref := hex.EncodeToString(hash[:])
	if item.OutputRef == ref {
		if _, err := e.readTerminalOutput(item); err != nil {
			return "", err
		}
		return ref, nil
	}
	folder, err := e.openTerminalOutputRoot(item.StorageRootID, item.OutputRef == "")
	if err != nil {
		return "", err
	}
	defer folder.Close()
	// 已存在的同名记录必须具有相同原始字节，不能悄悄替换损坏数据。
	existing, err := readTerminalRecordBytes(folder, ref)
	if err == nil {
		existingHash := sha256.Sum256(existing)
		if existingHash != hash {
			return "", errors.New("terminal output record checksum does not match its name")
		}
		return ref, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := storage.WriteJSONRoot(folder, ref+".json", disk); err != nil {
		return "", err
	}
	return ref, nil
}

func (e *Engine) readTerminalOutput(item contracts.Terminal) (string, error) {
	if item.StorageRootID == "" {
		return "", errors.New("terminal storage root is missing")
	}
	if _, err := e.storage.Resolve(item.StorageRootID); err != nil {
		return "", err
	}
	if item.OutputRef == "" {
		if item.Output != "" {
			return "", errors.New("terminal output is missing its immutable reference")
		}
		return "", nil
	}
	if !validRecordHash(item.OutputRef) {
		return "", errors.New("terminal output reference is invalid")
	}
	folder, err := e.openTerminalOutputRoot(item.StorageRootID, false)
	if err != nil {
		return "", err
	}
	defer folder.Close()
	data, err := readTerminalRecordBytes(folder, item.OutputRef)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != item.OutputRef {
		return "", errors.New("terminal output record checksum does not match its index")
	}
	var disk terminalOutputDisk
	if err := json.Unmarshal(data, &disk); err != nil {
		return "", err
	}
	if disk.Format != terminalOutputFormat || disk.TerminalID != item.ID || disk.RootID != item.StorageRootID {
		return "", errors.New("terminal output storage identity does not match its index")
	}
	return disk.Output, nil
}

func (e *Engine) openTerminalOutputRoot(rootID string, create bool) (*os.Root, error) {
	directory, err := e.storage.Resolve(rootID)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if create {
		if err := root.Mkdir("terminals", 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
	}
	return openRealDirectory(root, "terminals")
}

func readTerminalRecordBytes(folder *os.Root, ref string) ([]byte, error) {
	if !validRecordHash(ref) {
		return nil, errors.New("terminal output reference is invalid")
	}
	file, err := folder.Open(ref + ".json")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("terminal output record must be a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(maxTerminalRecordBytes)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxTerminalRecordBytes {
		return nil, fmt.Errorf("terminal output record exceeds %d bytes", maxTerminalRecordBytes)
	}
	return data, nil
}

func (e *Engine) terminalRefInUse(rootID, ref string) bool {
	return slices.ContainsFunc(e.state.Terminals, func(item contracts.Terminal) bool {
		return item.StorageRootID == rootID && item.OutputRef == ref
	})
}

func (e *Engine) removeTerminalOutput(rootID, ref string) {
	if !validRecordHash(ref) {
		return
	}
	folder, err := e.openTerminalOutputRoot(rootID, false)
	if err != nil {
		return
	}
	defer folder.Close()
	// 只清理已提交索引不再引用的一个应用记录，不递归清理整个运行根。
	_ = folder.Remove(ref + ".json")
}
