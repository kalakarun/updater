package testpackage

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"cpguard/updater/internal/durable"
	"cpguard/updater/internal/manifest"
	"cpguard/updater/internal/publisher"
	"cpguard/updater/internal/release"
	"cpguard/updater/internal/state"
	"cpguard/updater/internal/updater"
)

const offlineArchiveURL = "https://offline.invalid/cpguard-dummy-test.tar.gz"

type localArchive struct {
	path   string
	tamper string
	calls  int
}

func (source *localArchive) ValidateURL(address string) error {
	if address != offlineArchiveURL {
		return errors.New("offline test requires the non-downloadable offline.invalid archive URL")
	}
	return manifest.HTTPS(address)
}

func (source *localArchive) Archive(address, destination string, length int64, digest []byte) (failure error) {
	source.calls++
	if err := source.ValidateURL(address); err != nil {
		return err
	}
	if length <= 0 || length > release.MaxArchiveBytes || len(digest) != sha256.Size {
		return errors.New("invalid authenticated archive size or digest")
	}
	input, err := os.Open(source.path)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("offline archive must be a regular file")
	}
	var reader io.Reader = input
	if source.tamper == "hash" {
		var first [1]byte
		if _, err := io.ReadFull(input, first[:]); err != nil {
			return err
		}
		first[0] ^= 0xff
		reader = io.MultiReader(bytes.NewReader(first[:]), input)
	}
	if source.tamper == "size" {
		reader = io.LimitReader(reader, length-1)
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() {
		failure = errors.Join(failure, output.Close())
		if failure != nil {
			_ = os.Remove(destination)
		}
	}()
	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(output, hasher), io.LimitReader(reader, length+1))
	if err != nil {
		return err
	}
	if written != length || !bytes.Equal(hasher.Sum(nil), digest) {
		return errors.New("offline archive size or SHA-256 mismatch")
	}
	return output.Sync()
}

func TestOfflineSignedFlowFixture(t *testing.T) {
	directory := t.TempDir()
	archivePath := os.Getenv("CPGUARD_SAMPLE_ARCHIVE")
	if archivePath == "" {
		input := filepath.Join(directory, "source.tar")
		archivePath = filepath.Join(directory, "dummy.tar.gz")
		if err := os.WriteFile(input, sample(t, true, &tar.Header{Name: "app/payload.txt", Typeflag: tar.TypeReg, Size: 5, Mode: 0644}), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Build(input, scriptPath(t), archivePath); err != nil {
			t.Fatal(err)
		}
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	payload, err := publisher.Make(archivePath, offlineArchiveURL, "5.89.00", 1, now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := manifest.Sign(payload, private, now)
	if err != nil {
		t.Fatal(err)
	}
	metadataPath := filepath.Join(directory, "update_check.json")
	publicPath := filepath.Join(directory, "key-a.public.txt")
	if err := os.WriteFile(metadataPath, metadata, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(publicPath, []byte(base64.StdEncoding.EncodeToString(public)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CPGUARD_SAMPLE_ARCHIVE", archivePath)
	t.Setenv("CPGUARD_SAMPLE_METADATA", metadataPath)
	t.Setenv("CPGUARD_SAMPLE_PUBLIC_KEY", publicPath)
	testOfflineSignedInputs(t)
}

func TestOfflineSignedSampleFlow(t *testing.T) {
	testOfflineSignedInputs(t)
}

func testOfflineSignedInputs(t *testing.T) {
	t.Helper()
	archivePath := os.Getenv("CPGUARD_SAMPLE_ARCHIVE")
	metadataPath := os.Getenv("CPGUARD_SAMPLE_METADATA")
	publicPath := os.Getenv("CPGUARD_SAMPLE_PUBLIC_KEY")
	if metadataPath == "" && publicPath == "" {
		t.Skip("set CPGUARD_SAMPLE_ARCHIVE, CPGUARD_SAMPLE_METADATA and CPGUARD_SAMPLE_PUBLIC_KEY")
	}
	if archivePath == "" || metadataPath == "" || publicPath == "" {
		t.Fatal("all three offline test input paths are required")
	}
	metadata, err := durable.Read(metadataPath, manifest.MaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := durable.Read(publicPath, 1024)
	if err != nil {
		t.Fatal(err)
	}
	public, err := base64.StdEncoding.Strict().DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil || len(public) != ed25519.PublicKeySize {
		t.Fatal("public key must contain one base64-encoded Ed25519 public key")
	}
	exerciseOfflineFlow(t, archivePath, metadata, public)
}

func exerciseOfflineFlow(t *testing.T, archivePath string, metadata []byte, public ed25519.PublicKey) {
	t.Helper()
	expectedScript, err := durable.Read(scriptPath(t), 1048576)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"success", "signature", "hash", "size"} {
		t.Run(scenario, func(t *testing.T) {
			directory, err := os.MkdirTemp("/tmp", "cpguard-offline-test-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.RemoveAll(directory); err != nil {
					t.Error(err)
				}
			})
			statePath := filepath.Join(directory, "state.json")
			versionPath := filepath.Join(directory, "swversion")
			if err := state.Initialize(statePath); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(versionPath, []byte("58826"), 0600); err != nil {
				t.Fatal(err)
			}
			data := metadata
			if scenario == "signature" {
				var envelope manifest.Envelope
				if err := json.Unmarshal(data, &envelope); err != nil {
					t.Fatal(err)
				}
				envelope.Signature = base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
				data, err = json.Marshal(envelope)
				if err != nil {
					t.Fatal(err)
				}
			}
			source := &localArchive{path: archivePath, tamper: scenario}
			installs := 0
			root := filepath.Join(directory, "cpguard-updater-test")
			service := updater.Service{
				KeyA: public, StateFile: statePath, VersionFile: versionPath, TempParent: directory, Archives: source,
				Metadata: func(string, int64) ([]byte, error) { return data, nil },
				Install: func(ctx context.Context, packagePath string) error {
					installs++
					script, err := durable.Read(filepath.Join(packagePath, "run.sh"), 1048576)
					if err != nil {
						return err
					}
					if !bytes.Equal(script, expectedScript) {
						return errors.New("offline test only executes the repository's copy-only dummy run.sh")
					}
					command := exec.CommandContext(ctx, "/bin/sh", "./run.sh")
					command.Dir = packagePath
					command.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C", "CPGUARD_TEST_ROOT=" + root}
					command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
					command.Cancel = func() error { return syscall.Kill(-command.Process.Pid, syscall.SIGKILL) }
					command.WaitDelay = 5 * time.Second
					output, runErr := command.CombinedOutput()
					if command.Process != nil {
						_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
					}
					if runErr != nil {
						t.Logf("dummy installer output: %s", output)
					}
					return runErr
				},
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			if scenario == "success" {
				result, err := service.Run(ctx, false)
				if err != nil || result.Outcome != "available" || source.calls != 0 || installs != 0 {
					t.Fatal("offline check failed", result, err)
				}
			}
			result, runErr := service.Run(ctx, true)
			version, err := os.ReadFile(versionPath)
			if err != nil {
				t.Fatal(err)
			}
			if scenario != "success" {
				if runErr == nil || installs != 0 || string(version) != "58826" {
					t.Fatal("invalid input was not rejected before installation", result, installs, string(version), runErr)
				}
				t.Logf("rejected %s; installer not run and test version unchanged: %v", scenario, runErr)
			} else {
				if runErr != nil || result.Outcome != "installed" || installs != 1 || source.calls != 1 || string(version) != "58900" {
					t.Fatal("offline installation failed", result, installs, string(version), runErr)
				}
				entries, err := os.ReadDir(root)
				if err != nil || len(entries) != 1 {
					t.Fatal("missing dummy release", entries, err)
				}
				marker, err := os.ReadFile(filepath.Join(root, entries[0].Name(), "installed.txt"))
				if err != nil || !strings.Contains(string(marker), "purpose=copy-only-updater-test") {
					t.Fatal("missing dummy installer marker", err)
				}
				result, err = service.Run(ctx, true)
				if err != nil || result.Outcome != "up_to_date" || installs != 1 || source.calls != 1 {
					t.Fatal("same release installed again", result, err)
				}
				t.Log("offline check -> installed (58826 -> 58900) -> up_to_date; real cPGuard paths were not used")
			}
			entries, err := os.ReadDir(directory)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), "update-") {
					t.Fatal("updater temporary files were not cleaned up")
				}
			}
		})
	}
}
