package testpackage

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"cpguard/updater/internal/archive"
	"cpguard/updater/internal/durable"
	"cpguard/updater/internal/release"
)

type Summary struct {
	Files        int   `json:"files"`
	Directories  int   `json:"directories"`
	PayloadBytes int64 `json:"payload_bytes"`
	PackageBytes int64 `json:"package_bytes"`
}

type boundedWriter struct {
	target    io.Writer
	remaining int64
}

func (writer *boundedWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > writer.remaining {
		return 0, errors.New("test package exceeds compressed size limit")
	}
	written, err := writer.target.Write(data)
	writer.remaining -= int64(written)
	return written, err
}

func Build(inputPath, scriptPath, outputPath string) (summary Summary, failure error) {
	script, err := durable.Read(scriptPath, 1048576)
	if err != nil {
		return summary, err
	}
	if len(script) == 0 || strings.Contains(string(script), "\r") {
		return summary, errors.New("run.sh must be a nonempty LF-only shell script")
	}
	input, err := os.Open(inputPath)
	if err != nil {
		return summary, err
	}
	defer input.Close()
	buffer := bufio.NewReader(input)
	magic, err := buffer.Peek(2)
	if err != nil {
		return summary, err
	}
	var source io.Reader = buffer
	if magic[0] == 0x1f && magic[1] == 0x8b {
		compressed, err := gzip.NewReader(buffer)
		if err != nil {
			return summary, err
		}
		defer compressed.Close()
		source = compressed
	}
	output, err := os.OpenFile(outputPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return summary, err
	}
	defer func() {
		closeErr := output.Close()
		if failure == nil {
			failure = closeErr
		}
		if failure != nil {
			_ = os.Remove(outputPath)
		}
	}()
	compressed := gzip.NewWriter(&boundedWriter{target: output, remaining: release.MaxArchiveBytes})
	writer := tar.NewWriter(compressed)
	if err := writer.WriteHeader(&tar.Header{Name: "run.sh", Mode: 0700, Typeflag: tar.TypeReg, Size: int64(len(script))}); err != nil {
		return summary, err
	}
	if _, err := writer.Write(script); err != nil {
		return summary, err
	}
	reader := tar.NewReader(source)
	seen := map[string]bool{}
	kinds := map[string]byte{}
	count := 1
	var total int64 = int64(len(script))
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return summary, err
		}
		name := strings.TrimSuffix(header.Name, "/")
		for strings.HasPrefix(name, "./") {
			name = strings.TrimPrefix(name, "./")
		}
		if name == "" || name == "." {
			if header.Typeflag != tar.TypeDir {
				return summary, errors.New("invalid root entry")
			}
			continue
		}
		count++
		if !archive.SafeName(name) || (name != "app" && !strings.HasPrefix(name, "app/")) || seen[name] || len(name) > archive.DefaultLimits.PathBytes || count > archive.DefaultLimits.Entries {
			return summary, fmt.Errorf("unsafe or unexpected sample archive path %q; expected only app/", name)
		}
		seen[name] = true
		if header.Typeflag != tar.TypeDir && header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return summary, errors.New("sample archive contains links or unsupported entries")
		}
		if name == "app" && header.Typeflag != tar.TypeDir {
			return summary, errors.New("app must be a directory")
		}
		if header.Mode&07000 != 0 || header.Size < 0 || header.Size > archive.DefaultLimits.FileBytes || header.Size > archive.DefaultLimits.TotalBytes-total {
			return summary, errors.New("unsafe mode or sample size limit exceeded")
		}
		if kinds[name] == tar.TypeDir && header.Typeflag != tar.TypeDir {
			return summary, errors.New("file/directory conflict in sample")
		}
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if kind, exists := kinds[parent]; exists && kind != tar.TypeDir {
				return summary, errors.New("file used as a directory in sample")
			}
			kinds[parent] = tar.TypeDir
		}
		kinds[name] = header.Typeflag
		mode := int64(0600)
		if header.Typeflag == tar.TypeDir || header.Mode&0111 != 0 {
			mode = 0700
		}
		if header.Typeflag == tar.TypeDir && header.Size != 0 {
			return summary, errors.New("directory has data")
		}
		if err := writer.WriteHeader(&tar.Header{Name: name, Typeflag: header.Typeflag, Size: header.Size, Mode: mode}); err != nil {
			return summary, err
		}
		if _, err := io.CopyN(writer, reader, header.Size); err != nil {
			return summary, err
		}
		total += header.Size
		if header.Typeflag == tar.TypeDir {
			summary.Directories++
		} else {
			summary.Files++
			summary.PayloadBytes += header.Size
		}
	}
	if summary.Files == 0 {
		return summary, errors.New("sample contains no app files")
	}
	trailer, err := io.ReadAll(io.LimitReader(source, 1048577))
	if err != nil {
		return summary, err
	}
	if len(trailer) > 1048576 {
		return summary, errors.New("excessive sample trailer")
	}
	for _, padding := range trailer {
		if padding != 0 {
			return summary, errors.New("unexpected content after sample archive")
		}
	}
	if err := writer.Close(); err != nil {
		return summary, err
	}
	if err := compressed.Close(); err != nil {
		return summary, err
	}
	if err := output.Sync(); err != nil {
		return summary, err
	}
	info, err := output.Stat()
	if err != nil {
		return summary, err
	}
	summary.PackageBytes = info.Size()
	return summary, nil
}
