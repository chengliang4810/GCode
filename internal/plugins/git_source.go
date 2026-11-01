package plugins

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ExportGitSource fetches one fixed commit without checking out OS symlinks.
// The complete commit is exported as ordinary files and typed link metadata;
// final plugin installation decides which links actually need OS creation.
func ExportGitSource(ctx context.Context, source Source, target string) (tree SourceTree, commit string, resultErr error) {
	defer func() { resultErr = redactSourceError(resultErr, []string{source.URL}, nil) }()
	address, err := gitAddress(source)
	if err != nil {
		return SourceTree{}, "", err
	}
	ref := source.SHA
	if ref == "" {
		ref = source.Ref
	}
	if ref == "" {
		ref = "HEAD"
	}
	if strings.HasPrefix(ref, "-") || strings.ContainsAny(ref, "\r\n\x00") {
		return SourceTree{}, "", errors.New("invalid Git revision")
	}
	if source.SHA != "" && !fullSHA.MatchString(source.SHA) {
		return SourceTree{}, "", errors.New("sha must be a full 40-character commit hash")
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return SourceTree{}, "", err
	}
	target, err = pluginCopyDestination(target)
	if err != nil {
		return SourceTree{}, "", err
	}
	if filepath.IsAbs(address) && inside(address, target) {
		return SourceTree{}, "", errors.New("Git source export destination must not be inside its source repository")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return SourceTree{}, "", err
	}
	// Git for Windows cannot enter a directory beyond MAX_PATH during startup,
	// before core.longpaths can govern its repository file operations. Keep the
	// owned fetch repository independent of the final cache's directory depth.
	repository, err := os.MkdirTemp("", "gcode-plugin-source-")
	if err != nil {
		return SourceTree{}, "", err
	}
	defer os.RemoveAll(repository)
	if _, err := git(ctx, repository, "init"); err != nil {
		return SourceTree{}, "", err
	}
	if _, err := git(ctx, repository, "remote", "add", "origin", address); err != nil {
		return SourceTree{}, "", err
	}
	if _, err := git(ctx, repository, "fetch", "--depth=1", "--filter=blob:none", "--no-tags", "origin", ref); err != nil {
		return SourceTree{}, "", err
	}
	commit, err = git(ctx, repository, "rev-parse", "--verify", "FETCH_HEAD^{commit}")
	if err != nil {
		return SourceTree{}, "", err
	}
	if source.SHA != "" && !strings.EqualFold(commit, source.SHA) {
		return SourceTree{}, "", errors.New("Git commit does not match pinned SHA")
	}
	exportCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	command := gitCommand(
		exportCtx,
		"-C", repository, "archive", "--format=tar", commit,
	)
	var stderr archiveErrorOutput
	command.Stderr = &stderr
	stream, err := command.StdoutPipe()
	if err != nil {
		return SourceTree{}, "", err
	}
	if err := command.Start(); err != nil {
		_ = stream.Close()
		return SourceTree{}, "", err
	}
	tree, extractErr := extractGitSource(exportCtx, stream, target)
	if extractErr == nil {
		// tar.Reader stops at its end markers before Git's block padding. Drain
		// the bounded trailer so closing the pipe cannot SIGPIPE the exporter.
		const maxTrailer = int64(1 << 20)
		n, drainErr := io.Copy(io.Discard, io.LimitReader(stream, maxTrailer+1))
		extractErr = drainErr
		if n > maxTrailer {
			extractErr = errors.New("Git source archive exceeds its trailer limit")
		}
	}
	if extractErr != nil {
		cancel()
	}
	_ = stream.Close()
	waitErr := command.Wait()
	if extractErr != nil {
		return SourceTree{}, "", extractErr
	}
	if exportCtx.Err() != nil {
		return SourceTree{}, "", exportCtx.Err()
	}
	if waitErr != nil {
		return SourceTree{}, "", fmt.Errorf("Git source archive failed: %w: %s", waitErr, stderr.text)
	}
	return tree, commit, nil
}

type archiveErrorOutput struct{ text string }

func (b *archiveErrorOutput) Write(data []byte) (int, error) {
	if remaining := (1 << 20) - len(b.text); remaining > 0 {
		b.text += string(data[:min(len(data), remaining)])
	}
	return len(data), nil
}

func extractGitSource(ctx context.Context, input io.Reader, target string) (SourceTree, error) {
	target, err := filepath.Abs(target)
	if err != nil {
		return SourceTree{}, err
	}
	target, err = pluginCopyDestination(target)
	if err != nil {
		return SourceTree{}, err
	}
	if err := os.MkdirAll(target, 0700); err != nil {
		return SourceTree{}, err
	}
	root, err := os.OpenRoot(target)
	if err != nil {
		return SourceTree{}, err
	}
	defer root.Close()
	check, err := root.Open(".")
	if err != nil {
		return SourceTree{}, err
	}
	present, listErr := check.ReadDir(1)
	_ = check.Close()
	if listErr != nil && !errors.Is(listErr, io.EOF) {
		return SourceTree{}, listErr
	}
	if len(present) != 0 {
		return SourceTree{}, errors.New("Git source export destination must be empty")
	}
	result := SourceTree{Root: target, Links: map[string]string{}}
	seen := map[string]byte{}
	nodes := map[string]bool{}
	identity := func(name string) string {
		if runtime.GOOS == "windows" {
			return strings.ToLower(name)
		}
		return name
	}
	reader := tar.NewReader(input)
	var total int64
	entries, metadataBytes, nodeBytes := 0, 0, 0
	for {
		if err := ctx.Err(); err != nil {
			return SourceTree{}, err
		}
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return SourceTree{}, err
		}
		if header.Typeflag == tar.TypeXHeader || header.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		name := path.Clean(header.Name)
		if name == "." && header.Typeflag == tar.TypeDir {
			continue
		}
		if name != strings.TrimSuffix(header.Name, "/") || name == "." ||
			!filepath.IsLocal(filepath.FromSlash(name)) || strings.ContainsAny(name, "\\:\x00") {
			return SourceTree{}, fmt.Errorf("unsafe Git archive path: %q", header.Name)
		}
		parts := strings.Split(name, "/")
		if len(parts) > maxPluginDepth {
			return SourceTree{}, errors.New("Git source archive exceeds its depth limit")
		}
		for i, part := range parts {
			if strings.EqualFold(part, ".git") {
				return SourceTree{}, errors.New("Git source archive cannot contain .git")
			}
			if i < len(parts)-1 {
				parent := identity(strings.Join(parts[:i+1], "/"))
				if kind, exists := seen[parent]; exists && kind != tar.TypeDir {
					return SourceTree{}, errors.New("Git source archive has a non-directory parent")
				}
			}
			node := identity(strings.Join(parts[:i+1], "/"))
			if !nodes[node] {
				nodes[node] = true
				nodeBytes += len(node)
				if len(nodes) > maxPluginEntries || nodeBytes > 16<<20 {
					return SourceTree{}, errors.New("Git source archive exceeds its node/metadata limit")
				}
			}
		}
		key := identity(name)
		if _, duplicate := seen[key]; duplicate {
			return SourceTree{}, fmt.Errorf("duplicate Git archive path: %s", name)
		}
		seen[key] = header.Typeflag
		entries++
		metadataBytes += len(name) + len(header.Linkname)
		if entries > maxPluginEntries || metadataBytes > 16<<20 {
			return SourceTree{}, errors.New("Git source archive exceeds its entry/metadata limit")
		}
		relative := filepath.FromSlash(name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(relative, 0700); err != nil {
				return SourceTree{}, err
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || header.Size > maxPluginCopyBytes-total {
				return SourceTree{}, errors.New("Git source archive exceeds 512 MiB")
			}
			total += header.Size
			if err := root.MkdirAll(filepath.Dir(relative), 0700); err != nil {
				return SourceTree{}, err
			}
			file, err := root.OpenFile(relative, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(header.Mode)&0777)
			if err != nil {
				return SourceTree{}, err
			}
			_, copyErr := io.CopyN(file, reader, header.Size)
			closeErr := file.Close()
			if err := errors.Join(copyErr, closeErr); err != nil {
				return SourceTree{}, err
			}
		case tar.TypeSymlink:
			if header.Linkname == "" || strings.ContainsRune(header.Linkname, '\x00') {
				return SourceTree{}, errors.New("Git source archive has an invalid symbolic-link target")
			}
			if err := root.MkdirAll(filepath.Dir(relative), 0700); err != nil {
				return SourceTree{}, err
			}
			result.Links[name] = header.Linkname
		default:
			return SourceTree{}, errors.New("Git source archive contains a hard link or nonregular entry")
		}
	}
	verified, err := openSourceTree(result)
	if err != nil {
		return SourceTree{}, err
	}
	_ = verified.close()
	return result, nil
}

func readSourceDefinition(tree SourceTree, relative string) ([]byte, error) {
	source, err := openSourceTree(tree)
	if err != nil {
		return nil, err
	}
	defer source.close()
	if !filepath.IsLocal(filepath.FromSlash(relative)) || strings.ContainsAny(relative, "\\:") {
		return nil, errors.New("source definition path must remain inside its source tree")
	}
	resolved, err := resolveSourcePath(source, filepath.Join(source.root, filepath.FromSlash(relative)), false)
	if err != nil {
		return nil, err
	}
	path, err := relativeWithin(source.root, resolved)
	if err != nil {
		return nil, err
	}
	file, err := source.handle.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxDefinitionSize {
		return nil, errors.New("source definition is not a bounded regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxDefinitionSize+1))
	if len(data) > maxDefinitionSize {
		return nil, errors.New("source definition exceeds its size limit")
	}
	return data, err
}
