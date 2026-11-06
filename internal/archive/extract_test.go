package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T, headers ...*tar.Header) []byte {
	t.Helper()
	var buffer bytes.Buffer
	compressed := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(compressed)
	for _, header := range headers {
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA {
			if _, err := writer.Write(bytes.Repeat([]byte("a"), int(header.Size))); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestExtraction(t *testing.T) {
	data := fixture(t, &tar.Header{Name: "run.sh", Mode: 0755, Typeflag: tar.TypeReg, Size: 1}, &tar.Header{Name: "bin/app", Mode: 0755, Typeflag: tar.TypeReg, Size: 10})
	destination := filepath.Join(t.TempDir(), "package")
	if err := Extract(bytes.NewReader(data), destination, DefaultLimits); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(destination, "run.sh"))
	if err != nil || info.Size() != 1 {
		t.Fatal(info, err)
	}
}

func TestUnsafeArchives(t *testing.T) {
	cases := map[string][]*tar.Header{
		"parent":            {{Name: "../outside", Typeflag: tar.TypeReg, Mode: 0600}},
		"nested-parent":     {{Name: "a/../outside", Typeflag: tar.TypeReg, Mode: 0600}},
		"absolute":          {{Name: "/outside", Typeflag: tar.TypeReg, Mode: 0600}},
		"windows":           {{Name: `C:\outside`, Typeflag: tar.TypeReg, Mode: 0600}},
		"symlink":           {{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/etc"}},
		"hardlink":          {{Name: "link", Typeflag: tar.TypeLink, Linkname: "run.sh"}},
		"device":            {{Name: "dev", Typeflag: tar.TypeChar}},
		"fifo":              {{Name: "fifo", Typeflag: tar.TypeFifo}},
		"setuid":            {{Name: "app", Typeflag: tar.TypeReg, Mode: 04755}},
		"duplicate":         {{Name: "run.sh", Typeflag: tar.TypeReg, Mode: 0600}, {Name: "run.sh", Typeflag: tar.TypeReg, Mode: 0600}},
		"conflict":          {{Name: "app", Typeflag: tar.TypeReg, Mode: 0600}, {Name: "app/bin", Typeflag: tar.TypeReg, Mode: 0600}},
		"missing-installer": {{Name: "app", Typeflag: tar.TypeReg, Mode: 0600}},
	}
	for name, headers := range cases {
		t.Run(name, func(t *testing.T) {
			destination := filepath.Join(t.TempDir(), "package")
			if err := Extract(bytes.NewReader(fixture(t, headers...)), destination, DefaultLimits); err == nil {
				t.Fatal("accepted unsafe archive")
			}
			if _, err := os.Lstat(destination); !os.IsNotExist(err) {
				t.Fatal("failed extraction retained disposable files")
			}
		})
	}
}

func TestExtractionLimitsAndTruncation(t *testing.T) {
	data := fixture(t, &tar.Header{Name: "run.sh", Mode: 0700, Typeflag: tar.TypeReg, Size: 20})
	for _, limits := range []Limits{{FileBytes: 10, TotalBytes: 100, Entries: 2, PathBytes: 20}, {FileBytes: 100, TotalBytes: 10, Entries: 2, PathBytes: 20}, {FileBytes: 100, TotalBytes: 100, Entries: 2, PathBytes: 2}} {
		if err := Extract(bytes.NewReader(data), filepath.Join(t.TempDir(), "package"), limits); err == nil {
			t.Fatal("limit ignored")
		}
	}
	if err := Extract(bytes.NewReader(data[:len(data)-5]), filepath.Join(t.TempDir(), "package"), DefaultLimits); err == nil {
		t.Fatal("accepted truncated gzip")
	}
	data = fixture(t, &tar.Header{Name: "run.sh", Mode: 0600, Typeflag: tar.TypeReg}, &tar.Header{Name: "other", Mode: 0600, Typeflag: tar.TypeReg})
	if err := Extract(bytes.NewReader(data), filepath.Join(t.TempDir(), "package"), Limits{FileBytes: 100, TotalBytes: 100, Entries: 1, PathBytes: 100}); err == nil {
		t.Fatal("entry limit ignored")
	}
}
