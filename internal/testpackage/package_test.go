package testpackage

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"cpguard/updater/internal/archive"
)

func sample(t *testing.T, compressed bool, headers ...*tar.Header) []byte {
	t.Helper()
	var output bytes.Buffer
	var raw bytes.Buffer
	writer := tar.NewWriter(&raw)
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
	if !compressed {
		return raw.Bytes()
	}
	zip := gzip.NewWriter(&output)
	if _, err := zip.Write(raw.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := zip.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func scriptPath(t *testing.T) string {
	t.Helper()
	name, err := filepath.Abs("../../examples/test-package/run.sh")
	if err != nil {
		t.Fatal(err)
	}
	return name
}

func TestBuildPlainAndCompressed(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		directory := t.TempDir()
		input := filepath.Join(directory, "app.tar")
		output := filepath.Join(directory, "update.tar.gz")
		data := sample(t, compressed, &tar.Header{Name: "./app/", Typeflag: tar.TypeDir, Mode: 0755}, &tar.Header{Name: "./app/payload.txt", Typeflag: tar.TypeReg, Mode: 0644, Size: 5})
		if err := os.WriteFile(input, data, 0600); err != nil {
			t.Fatal(err)
		}
		summary, err := Build(input, scriptPath(t), output)
		if err != nil || summary.Files != 1 || summary.Directories != 1 || summary.PayloadBytes != 5 || summary.PackageBytes < 1 {
			t.Fatal(summary, err)
		}
		file, err := os.Open(output)
		if err != nil {
			t.Fatal(err)
		}
		err = archive.Extract(file, filepath.Join(directory, "extracted"), archive.DefaultLimits)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
		copied, err := os.ReadFile(filepath.Join(directory, "extracted", "app", "payload.txt"))
		if err != nil || string(copied) != "aaaaa" {
			t.Fatal(string(copied), err)
		}
		if _, err := Build(input, scriptPath(t), output); err == nil {
			t.Fatal("overwrote output")
		}
		after, err := os.ReadFile(input)
		if err != nil || !bytes.Equal(after, data) {
			t.Fatal("source archive modified")
		}
	}
}

func TestRejectUnsafeSample(t *testing.T) {
	cases := map[string][]*tar.Header{
		"traversal":        {{Name: "app/../outside", Typeflag: tar.TypeReg}},
		"outside":          {{Name: "other.txt", Typeflag: tar.TypeReg}},
		"link":             {{Name: "app/link", Typeflag: tar.TypeSymlink, Linkname: "/etc"}},
		"setuid":           {{Name: "app/binary", Typeflag: tar.TypeReg, Mode: 04755}},
		"app-file":         {{Name: "app", Typeflag: tar.TypeReg}},
		"duplicate":        {{Name: "app/file", Typeflag: tar.TypeReg}, {Name: "app/file", Typeflag: tar.TypeReg}},
		"conflict":         {{Name: "app/file", Typeflag: tar.TypeReg}, {Name: "app/file/child", Typeflag: tar.TypeReg}},
		"reverse-conflict": {{Name: "app/file/child", Typeflag: tar.TypeReg}, {Name: "app/file", Typeflag: tar.TypeReg}},
		"empty":            {{Name: "app", Typeflag: tar.TypeDir}},
	}
	for name, headers := range cases {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			input := filepath.Join(directory, "source.tar")
			output := filepath.Join(directory, "package.tar.gz")
			if err := os.WriteFile(input, sample(t, false, headers...), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Build(input, scriptPath(t), output); err == nil {
				t.Fatal("accepted unsafe input")
			}
			if _, err := os.Lstat(output); !os.IsNotExist(err) {
				t.Fatal("partial output retained")
			}
		})
	}
}
