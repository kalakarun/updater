package archive

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"cpguard/updater/internal/capacity"
)

type Limits struct {
	FileBytes  int64
	TotalBytes int64
	Entries    int
	PathBytes  int
}

var DefaultLimits = Limits{FileBytes: 1073741824, TotalBytes: 4294967296, Entries: 100000, PathBytes: 4096}

func Extract(source io.Reader, destination string, limits Limits) error {
	return ExtractContext(context.Background(), source, destination, limits)
}

func ExtractContext(ctx context.Context, source io.Reader, destination string, limits Limits) (failure error) {
	if limits.FileBytes < 1 || limits.TotalBytes < 1 || limits.Entries < 1 || limits.PathBytes < 1 {
		return errors.New("invalid extraction limits")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Mkdir(destination, 0700); err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			failure = errors.Join(failure, os.RemoveAll(destination))
		}
	}()
	root, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer root.Close()
	compressed, err := gzip.NewReader(contextReader{ctx: ctx, source: source})
	if err != nil {
		return err
	}
	defer compressed.Close()
	decoded := contextReader{ctx: ctx, source: compressed}
	reader := tar.NewReader(decoded)
	seen := map[string]bool{}
	var total int64
	var entries int
	installer := false
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
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
		if copyErr == nil && written != header.Size {
			copyErr = io.ErrUnexpectedEOF
		}
		if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
			return fmt.Errorf("extract %q: %w", name, err)
		}
		if name == "run.sh" {
			installer = true
		}
	}
	if !installer {
		return errors.New("archive missing regular top-level run.sh")
	}
	tail, err := io.ReadAll(io.LimitReader(decoded, 1048577))
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
	if err := ctx.Err(); err != nil {
		return err
	}
	complete = true
	return nil
}

type contextReader struct {
	ctx    context.Context
	source io.Reader
}

func (reader contextReader) Read(data []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	read, err := reader.source.Read(data)
	if err == nil {
		err = reader.ctx.Err()
	}
	return read, err
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
