package mock

import (
	"errors"
	"path"
	"strings"

	"gcode/internal/contracts"
)

func (s *Server) fileMutationLocked(c contracts.Command) error {
	switch c.Kind {
	case "file.create":
		p := strings.ReplaceAll(strings.TrimSpace(c.Path), "\\", "/")
		if p == "" {
			p = c.Name
		}
		if p == "" || path.IsAbs(p) || p == ".." || strings.HasPrefix(p, "../") || strings.ContainsAny(p, ":\x00") {
			return errors.New("file path must be relative to the project")
		}
		p = path.Clean(p)
		if p == "." || strings.HasPrefix(p, "../") {
			return errors.New("invalid file path")
		}
		projectExists := false
		for _, pr := range s.state.Projects {
			if pr.ID == c.ProjectID {
				projectExists = true
				break
			}
		}
		if !projectExists {
			return ErrNotFound
		}
		parent := ""
		for _, f := range s.state.Files {
			if f.ProjectID != c.ProjectID {
				continue
			}
			if f.Path == p {
				return errors.New("a file with that name already exists")
			}
			if f.Path == path.Dir(p) {
				if f.Kind != "directory" {
					return errors.New("parent is not a directory")
				}
				parent = f.ID
			}
		}
		if path.Dir(p) != "." && parent == "" {
			return errors.New("parent directory does not exist")
		}
		kind := "file"
		if c.Enabled {
			kind = "directory"
		}
		language := strings.TrimPrefix(path.Ext(p), ".")
		if language == "md" {
			language = "markdown"
		}
		s.state.Files = append(s.state.Files, contracts.File{ID: s.idLocked("file"), ProjectID: c.ProjectID, Name: path.Base(p), Path: p, ParentID: parent, Kind: kind, Language: language, Content: c.Text, Version: 1, Modified: true})
		s.createFileChangeLocked(s.state.Files[len(s.state.Files)-1])
		return nil
	case "file.rename", "file.delete":
		index := -1
		for i, f := range s.state.Files {
			if f.ID == c.ID {
				index = i
				break
			}
		}
		if index < 0 {
			return ErrNotFound
		}
		target := s.state.Files[index]
		if target.ReadOnly {
			return ErrReadOnly
		}
		if c.Kind == "file.delete" {
			for _, f := range s.state.Files {
				if f.ProjectID == target.ProjectID && (f.ID == target.ID || strings.HasPrefix(f.Path, target.Path+"/")) && f.ReadOnly {
					return ErrReadOnly
				}
			}
			out := s.state.Files[:0]
			for _, f := range s.state.Files {
				if f.ProjectID == target.ProjectID && (f.ID == target.ID || strings.HasPrefix(f.Path, target.Path+"/")) {
					if f.ReadOnly {
						return ErrReadOnly
					}
					s.deleteFileChangeLocked(f)
					continue
				}
				out = append(out, f)
			}
			s.state.Files = out
			return nil
		}
		name := strings.TrimSpace(c.Name)
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\:\x00") {
			return errors.New("enter a file name without path separators")
		}
		newPath := path.Join(path.Dir(target.Path), name)
		if newPath == target.Path {
			return nil
		}
		for _, f := range s.state.Files {
			if f.ProjectID == target.ProjectID && f.Path == newPath && f.ID != target.ID {
				return errors.New("a file with that name already exists")
			}
			if f.ProjectID == target.ProjectID && strings.HasPrefix(f.Path, target.Path+"/") && f.ReadOnly {
				return ErrReadOnly
			}
		}
		for i := range s.state.Files {
			f := &s.state.Files[i]
			if f.ProjectID != target.ProjectID {
				continue
			}
			previousPath := f.Path
			if f.ID == target.ID {
				f.Name = name
				f.Path = newPath
				if f.Kind != "directory" {
					f.Language = strings.TrimPrefix(strings.ToLower(path.Ext(name)), ".")
					if f.Language == "md" {
						f.Language = "markdown"
					}
				}
				f.Version++
				f.Modified = true
			} else if strings.HasPrefix(f.Path, target.Path+"/") {
				f.Path = newPath + strings.TrimPrefix(f.Path, target.Path)
				f.Version++
				f.Modified = true
			}
			s.renameFileChangeLocked(f, previousPath)
		}
		return nil
	}
	return errors.New("unsupported file mutation")
}
