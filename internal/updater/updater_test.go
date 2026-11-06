package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cpguard/updater/internal/durable"
	"cpguard/updater/internal/manifest"
	"cpguard/updater/internal/state"
)

type download struct {
	data  []byte
	calls int
}

func (source *download) ValidateURL(address string) error {
	if err := manifest.HTTPS(address); err != nil {
		return err
	}
	parsed, _ := url.Parse(address)
	if parsed.Hostname() != "cdn.test" {
		return errors.New("archive hostname is not allowed")
	}
	return nil
}

func (source *download) Archive(address, destination string, length int64, digest []byte) error {
	source.calls++
	if err := source.ValidateURL(address); err != nil {
		return err
	}
	actual := sha256.Sum256(source.data)
	if int64(len(source.data)) != length || !bytes.Equal(actual[:], digest) {
		return errors.New("archive verification failed")
	}
	return os.WriteFile(destination, source.data, 0600)
}

func packageBytes(t *testing.T, unsafe bool) []byte {
	t.Helper()
	var buffer bytes.Buffer
	compressed := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(compressed)
	script := []byte("#!/bin/sh\nexit 0\n")
	if err := writer.WriteHeader(&tar.Header{Name: "run.sh", Mode: 0700, Size: int64(len(script)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(script); err != nil {
		t.Fatal(err)
	}
	if unsafe {
		if err := writer.WriteHeader(&tar.Header{Name: "link", Linkname: "/etc", Typeflag: tar.TypeSymlink}); err != nil {
			t.Fatal(err)
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

type fixture struct {
	service  Service
	source   *download
	payload  manifest.Payload
	private  ed25519.PrivateKey
	metadata []byte
	installs int
	now      time.Time
}

func setup(t *testing.T) *fixture {
	t.Helper()
	directory := t.TempDir()
	publicA, privateA, _ := ed25519.GenerateKey(rand.Reader)
	item := &fixture{source: &download{data: packageBytes(t, false)}, private: privateA, now: time.Now().UTC().Truncate(time.Second)}
	item.payload = manifest.Payload{Product: "cpguard", Version: "5.89.00", Sequence: 1042, IssuedAt: item.now.Format(manifest.TimeLayout), ExpiresAt: item.now.Add(time.Hour).Format(manifest.TimeLayout), File: manifest.File{URL: "https://cdn.test/release.tar.gz"}}
	item.service = Service{KeyA: publicA, MetadataURL: "https://metadata.test/update_check.json", StateFile: filepath.Join(directory, "state.json"), VersionFile: filepath.Join(directory, "swversion"), TempParent: directory, Archives: item.source, Now: func() time.Time { return item.now }}
	item.service.Metadata = func(string, int64) ([]byte, error) { return item.metadata, nil }
	item.service.Install = func(_ context.Context, packagePath string) error {
		item.installs++
		if _, err := os.Stat(filepath.Join(packagePath, "run.sh")); err != nil {
			return err
		}
		return nil
	}
	if err := state.Initialize(item.service.StateFile); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(item.service.VersionFile, []byte("58826\n"), 0640); err != nil {
		t.Fatal(err)
	}
	item.sign(t)
	return item
}

func (item *fixture) sign(t *testing.T) {
	t.Helper()
	digest := sha256.Sum256(item.source.data)
	item.payload.File.Size = int64(len(item.source.data))
	item.payload.File.SHA256 = hex.EncodeToString(digest[:])
	item.resign(t)
}

func (item *fixture) resign(t *testing.T) {
	t.Helper()
	var err error
	item.metadata, err = manifest.Sign(item.payload, item.private, item.now)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSuccessfulFlowAndNoUpdate(t *testing.T) {
	item := setup(t)
	result, err := item.service.Run(context.Background(), true)
	if err != nil || result.Outcome != "installed" || item.installs != 1 {
		t.Fatal(result, item.installs, err)
	}
	data, _ := os.ReadFile(item.service.VersionFile)
	if string(data) != "58900\n" {
		t.Fatal(string(data))
	}
	result, err = item.service.Run(context.Background(), true)
	if err != nil || result.Outcome != "up_to_date" || item.installs != 1 || item.source.calls != 1 {
		t.Fatal(result, item.installs, err)
	}
}

func TestChecksBlockInstaller(t *testing.T) {
	for _, scenario := range []string{"signature", "expired", "sequence", "hash", "size", "unsafe-archive", "missing-state", "corrupt-state", "missing-version", "invalid-version", "host"} {
		t.Run(scenario, func(t *testing.T) {
			item := setup(t)
			switch scenario {
			case "signature":
				item.metadata = bytes.Replace(item.metadata, []byte(`"signature": "`), []byte(`"signature": "!`), 1)
			case "expired":
				item.now = item.now.Add(2 * time.Hour)
			case "sequence":
				verified, err := manifest.Verify(item.metadata, item.service.KeyA, item.now, item.source.ValidateURL)
				if err != nil {
					t.Fatal(err)
				}
				verified.Payload.Sequence++
				if err := state.Accept(item.service.StateFile, verified); err != nil {
					t.Fatal(err)
				}
			case "hash":
				item.payload.File.SHA256 = strings.Repeat("b", 64)
				item.resign(t)
			case "size":
				item.payload.File.Size++
				item.resign(t)
			case "unsafe-archive":
				item.source.data = packageBytes(t, true)
				item.sign(t)
			case "missing-state":
				if err := os.Remove(item.service.StateFile); err != nil {
					t.Fatal(err)
				}
			case "corrupt-state":
				if err := os.WriteFile(item.service.StateFile, []byte("{}"), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing-version":
				if err := os.Remove(item.service.VersionFile); err != nil {
					t.Fatal(err)
				}
			case "invalid-version":
				if err := os.WriteFile(item.service.VersionFile, []byte("invalid"), 0600); err != nil {
					t.Fatal(err)
				}
			case "host":
				item.payload.File.URL = "https://other.test/release.tar.gz"
				item.resign(t)
			}
			if _, err := item.service.Run(context.Background(), true); err == nil {
				t.Fatal("accepted failing check")
			}
			if item.installs != 0 {
				t.Fatal("installer ran before validation")
			}
			if scenario != "missing-version" && scenario != "invalid-version" {
				data, _ := os.ReadFile(item.service.VersionFile)
				if string(data) != "58826\n" {
					t.Fatal("version changed", string(data))
				}
			}
			entries, _ := os.ReadDir(item.service.TempParent)
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), "update-") {
					t.Fatal("temporary files retained")
				}
			}
		})
	}
}

func TestFailedInstallerAndRetry(t *testing.T) {
	item := setup(t)
	item.service.Install = func(context.Context, string) error { item.installs++; return errors.New("script failed") }
	if _, err := item.service.Run(context.Background(), true); err == nil {
		t.Fatal("ignored script failure")
	}
	data, _ := os.ReadFile(item.service.VersionFile)
	if string(data) != "58826\n" {
		t.Fatal("failed install changed version")
	}
	item.service.Install = func(context.Context, string) error { item.installs++; return nil }
	result, err := item.service.Run(context.Background(), true)
	if err != nil || result.Outcome != "installed" || item.installs != 2 {
		t.Fatal(result, err)
	}
}

func TestCheckAndOlderVersionDoNotDownload(t *testing.T) {
	for _, install := range []bool{false, true} {
		item := setup(t)
		if install {
			item.payload.Version = "5.88.25"
			item.resign(t)
		}
		if _, err := item.service.Run(context.Background(), install); err != nil {
			t.Fatal(err)
		}
		if item.installs != 0 || item.source.calls != 0 {
			t.Fatal("unexpected download or installation")
		}
	}
}

func TestRestoreVersionAfterWriteFailure(t *testing.T) {
	item := setup(t)
	calls := 0
	item.service.WriteVersion = func(name string, data []byte, mode os.FileMode, uid, gid int) error {
		calls++
		if err := durable.Write(name, data, mode, uid, gid); err != nil {
			return err
		}
		if calls == 1 {
			return errors.New("injected error after rename")
		}
		return nil
	}
	if _, err := item.service.Run(context.Background(), true); err == nil {
		t.Fatal("ignored version persistence failure")
	}
	data, _ := os.ReadFile(item.service.VersionFile)
	if string(data) != "58826\n" {
		t.Fatal("original version not restored")
	}
}

func TestInstallerMustNotWriteVersion(t *testing.T) {
	item := setup(t)
	item.service.Install = func(context.Context, string) error {
		return os.WriteFile(item.service.VersionFile, []byte("58900\n"), 0600)
	}
	if _, err := item.service.Run(context.Background(), true); err == nil {
		t.Fatal("accepted installer version write")
	}
	data, _ := os.ReadFile(item.service.VersionFile)
	if string(data) != "58826\n" {
		t.Fatal("original version not restored")
	}
}
