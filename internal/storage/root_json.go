package storage

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// WriteJSONRoot atomically replaces one file directly within an already opened
// directory. The root keeps both staging and replacement within that directory.
func WriteJSONRoot(root *os.Root, name string, value any) error {
	if root == nil {
		return errors.New("JSON storage root is required")
	}
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name ||
		filepath.IsAbs(name) || filepath.VolumeName(name) != "" || strings.ContainsAny(name, `\/:`) {
		return errors.New("JSON storage name must be a single filename")
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	var file *os.File
	var temp string
	for {
		temp = ".write-" + strings.ToLower(rand.Text()) + ".tmp"
		file, err = root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return err
		}
		break
	}
	// 只删除根句柄内由本次写入创建的临时文件，不解析外部路径。
	defer root.Remove(temp)
	if _, err = file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err = errors.Join(file.Sync(), file.Close()); err != nil {
		return err
	}
	return replaceJSONRoot(root, temp, name)
}
