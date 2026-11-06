package archive

import (
	"archive/tar"
	"compress/gzip"
	"cpguard/updater/internal/capacity"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type Limits struct {
	FileBytes  int64
	TotalBytes int64
	Entries    int
	PathBytes  int
}

var DefaultLimits = Limits{FileBytes: 1073741824, TotalBytes: 4294967296, Entries: 100000, PathBytes: 4096}

func Extract(source io.Reader, destination string, limits Limits) error {
	if limits.FileBytes < 1 || limits.TotalBytes < 1 || limits.Entries < 1 || limits.PathBytes < 1 {
		return errors.New("invalid extraction limits")
	}
	if err := os.Mkdir(destination, 0700); err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(destination)
		}
	}()
	root, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer root.Close()
	compressed, err := gzip.NewReader(source)
	if err != nil {
		return err
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	seen := map[string]bool{}
	var total int64
	var entries int
	installer := false
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		entries++
		name := strings.TrimSuffix(header.Name, "/")
		if entries > limits.Entries || len(name) > limits.PathBytes || !SafeName(name) || seen[name] {
			return fmt.Errorf("unsafe, duplicate, or excessive archive path %q", name)
		}
		seen[name] = true
		if header.Mode&07000 != 0 || header.Size < 0 || header.Size > limits.FileBytes || header.Size > limits.TotalBytes-total {
			return errors.New("unsafe mode or archive size limit exceeded")
		}
		total += header.Size
		if header.Typeflag != tar.TypeDir && header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return errors.New("unsupported archive entry type")
		}
		if err := root.MkdirAll(filepath.FromSlash(path.Dir(name)), 0700); err != nil {
			return err
		}
		if header.Typeflag == tar.TypeDir {
			if header.Size != 0 {
				return errors.New("directory contains data")
			}
			if err := root.MkdirAll(filepath.FromSlash(name), 0700); err != nil {
				return err
			}
			continue
		}
		mode := os.FileMode(0600)
		if header.Mode&0111 != 0 {
			mode = 0700
		}
		if err := capacity.Require(destination, uint64(header.Size)+67108864); err != nil {
			return err
		}
		file, err := root.OpenFile(filepath.FromSlash(name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err != nil {
			return err
		}
		written, copyErr := io.CopyN(file, reader, header.Size)
		syncErr := file.Sync()
		closeErr := file.Close()
		if copyErr != nil || written != header.Size {
			return errors.New("truncated archive entry")
		}
		if syncErr != nil {
			return syncErr
		}
		if closeErr != nil {
			return closeErr
		}
		if name == "run.sh" {
			installer = true
		}
	}
	if !installer {
		return errors.New("archive missing regular top-level run.sh")
	}
	tail, err := io.ReadAll(io.LimitReader(compressed, 1048577))
	if err != nil {
		return err
	}
	if len(tail) > 1048576 {
		return errors.New("excessive archive trailer")
	}
	for _, padding := range tail {
		if padding != 0 {
			return errors.New("nonzero data after tar terminator")
		}
	}
	complete = true
	return nil
}

func SafeName(name string) bool {
	if name == "" || name == "." || strings.ContainsAny(name, "\\:\x00") || strings.HasPrefix(name, "/") || path.Clean(name) != name {
		return false
	}
	for _, component := range strings.Split(name, "/") {
		if component == ".." || component == "." || component == "" {
			return false
		}
	}
	return true
}
