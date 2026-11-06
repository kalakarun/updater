package testpackage

import (
	"archive/tar"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cpguard/updater/internal/archive"
)

func TestSampleInstaller(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "source.tar")
	output := filepath.Join(directory, "package.tar.gz")
	if err := os.WriteFile(input, sample(t, true, &tar.Header{Name: "app/payload.txt", Typeflag: tar.TypeReg, Size: 5, Mode: 0644}), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(input, scriptPath(t), output); err != nil {
		t.Fatal(err)
	}
	runPackage(t, output)
}

func TestSuppliedSampleInstaller(t *testing.T) {
	archivePath := os.Getenv("CPGUARD_SAMPLE_ARCHIVE")
	if archivePath == "" {
		t.Skip("set CPGUARD_SAMPLE_ARCHIVE to validate the prepared local sample")
	}
	runPackage(t, archivePath)
}

func runPackage(t *testing.T, archivePath string) {
	t.Helper()
	directory, err := os.MkdirTemp("/tmp", "cpguard-copy-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(directory); err != nil {
			t.Error(err)
		}
	})
	packagePath := filepath.Join(directory, "package")
	file, err := os.Open(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	err = archive.Extract(file, packagePath, archive.DefaultLimits)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(directory, "cpguard-updater-test")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/bin/sh", "./run.sh")
	command.Dir = packagePath
	command.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "CPGUARD_TEST_ROOT=" + root}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("test installer failed: %v\n%s", err, output)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatal("unexpected test releases", entries, err)
	}
	releasePath := filepath.Join(root, entries[0].Name())
	marker, err := os.ReadFile(filepath.Join(releasePath, "installed.txt"))
	if err != nil || !strings.Contains(string(marker), "purpose=copy-only-updater-test") {
		t.Fatal("missing test marker", err)
	}
	sourceFiles := 0
	err = filepath.WalkDir(filepath.Join(packagePath, "app"), func(name string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		sourceFiles++
		relative, err := filepath.Rel(filepath.Join(packagePath, "app"), name)
		if err != nil {
			return err
		}
		expected, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		actual, err := os.ReadFile(filepath.Join(releasePath, "app", relative))
		if err != nil {
			return err
		}
		if string(actual) != string(expected) {
			t.Fatalf("copied payload differs: %s", relative)
		}
		return nil
	})
	if err != nil || sourceFiles == 0 {
		t.Fatal("sample copy verification failed", sourceFiles, err)
	}
	if _, err := os.Stat(filepath.Join(packagePath, "swversion")); !os.IsNotExist(err) {
		t.Fatal("sample created a version file")
	}
	t.Logf("verified %d copied app files without executing application code", sourceFiles)
}
