package versionfile

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"cpguard/updater/internal/durable"
)

func fixture(t *testing.T) (string, string, string) {
	t.Helper()
	anchor := temporaryDirectory(t)
	relative := filepath.FromSlash("cpguard/app/setup/swversion")
	name := filepath.Join(anchor, relative)
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte("58826"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(name, 0644); err != nil {
		t.Fatal(err)
	}
	return anchor, relative, name
}

func temporaryDirectory(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp("/tmp", "cpguard-versionfile-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(directory); err != nil {
			t.Error(err)
		}
	})
	return directory
}

func openFixture(t *testing.T, anchor, relative string, owner int) *File {
	t.Helper()
	file, err := openFile(anchor, relative, owner, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
	})
	return file
}

func TestAtomicWritePreservesOwnershipAndPermissions(t *testing.T) {
	anchor, relative, name := fixture(t)
	file := openFixture(t, anchor, relative, os.Geteuid())
	original, err := file.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	uid, gid := durable.Ownership(original)
	data, err := file.Read(name, 128)
	if err != nil || string(data) != "58826" {
		t.Fatalf("read = %q, %v", data, err)
	}
	if err := file.Write(name, []byte("58900"), original.Mode(), uid, gid); err != nil {
		t.Fatal(err)
	}
	updated, err := file.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	updatedUID, updatedGID := durable.Ownership(updated)
	if os.SameFile(original, updated) || updated.Mode().Perm() != 0644 || updatedUID != uid || updatedGID != gid {
		t.Fatalf("replacement did not preserve metadata: %v, uid=%d gid=%d", updated.Mode(), updatedUID, updatedGID)
	}
	data, err = file.Read(name, 128)
	if err != nil || string(data) != "58900" {
		t.Fatalf("read after write = %q, %v", data, err)
	}
	leftovers, err := filepath.Glob(filepath.Join(filepath.Dir(name), ".swversion-*"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("temporary files remain: %v, %v", leftovers, err)
	}
}

func TestRootUpdatesServiceOwnedVersion(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root to create a service-owned fixture")
	}
	anchor, relative, name := fixture(t)
	for _, suffix := range []string{"cpguard", "cpguard/app", "cpguard/app/setup", relative} {
		if err := os.Chown(filepath.Join(anchor, suffix), 999, 999); err != nil {
			t.Fatal(err)
		}
	}
	file := openFixture(t, anchor, relative, 999)
	if err := file.Write(name, []byte("58900"), 0644, 999, 999); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(name)
	if err != nil {
		t.Fatal(err)
	}
	uid, gid := durable.Ownership(info)
	if uid != 999 || gid != 999 || info.Mode().Perm() != 0644 {
		t.Fatalf("owner/mode changed: %d:%d %v", uid, gid, info.Mode())
	}
	data, err := file.Read(name, 128)
	if err != nil || string(data) != "58900" {
		t.Fatalf("root read service-owned version = %q, %v", data, err)
	}
}

func TestRejectUnsafePathsAndMetadata(t *testing.T) {
	for _, kind := range []string{"directory-mode", "leaf-mode", "special-mode", "wrong-owner", "directory-symlink", "leaf-symlink", "hardlink", "directory-leaf"} {
		t.Run(kind, func(t *testing.T) {
			anchor, relative, name := fixture(t)
			outside := filepath.Join(temporaryDirectory(t), "outside")
			if err := os.WriteFile(outside, []byte("untouched"), 0644); err != nil {
				t.Fatal(err)
			}
			owner := os.Geteuid()
			switch kind {
			case "directory-mode":
				if err := os.Chmod(filepath.Dir(name), 0775); err != nil {
					t.Fatal(err)
				}
			case "leaf-mode":
				if err := os.Chmod(name, 0664); err != nil {
					t.Fatal(err)
				}
			case "special-mode":
				if err := os.Chmod(name, 0644|os.ModeSetuid); err != nil {
					t.Fatal(err)
				}
			case "wrong-owner":
				if owner == 0 {
					if err := os.Chown(name, 999, 999); err != nil {
						t.Fatal(err)
					}
					owner = 998
				} else {
					owner++
				}
			case "directory-symlink":
				parent := filepath.Dir(name)
				if err := os.Rename(parent, parent+"-original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(parent+"-original", parent); err != nil {
					t.Fatal(err)
				}
			case "leaf-symlink":
				if err := os.Remove(name); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, name); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Remove(name); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(outside, name); err != nil {
					t.Fatal(err)
				}
			case "directory-leaf":
				if err := os.Remove(name); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(name, 0755); err != nil {
					t.Fatal(err)
				}
			}
			file, err := openFile(anchor, relative, owner, false)
			if err == nil {
				file.Close()
				t.Fatal("unsafe path was accepted")
			}
			data, err := os.ReadFile(outside)
			if err != nil || !bytes.Equal(data, []byte("untouched")) {
				t.Fatalf("outside file changed: %q, %v", data, err)
			}
		})
	}
}

func TestRejectChangedDirectoryAndLeaf(t *testing.T) {
	for _, kind := range []string{"parent-rename", "parent-mode", "leaf-symlink", "leaf-hardlink", "leaf-mode"} {
		t.Run(kind, func(t *testing.T) {
			anchor, relative, name := fixture(t)
			file := openFixture(t, anchor, relative, os.Geteuid())
			outside := filepath.Join(temporaryDirectory(t), "outside")
			if err := os.WriteFile(outside, []byte("untouched"), 0644); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "parent-rename":
				parent := filepath.Dir(name)
				if err := os.Rename(parent, parent+"-original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(parent, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(name, []byte("replacement"), 0644); err != nil {
					t.Fatal(err)
				}
			case "parent-mode":
				if err := os.Chmod(filepath.Dir(name), 0775); err != nil {
					t.Fatal(err)
				}
			case "leaf-symlink", "leaf-hardlink":
				if err := os.Remove(name); err != nil {
					t.Fatal(err)
				}
				var err error
				if kind == "leaf-symlink" {
					err = os.Symlink(outside, name)
				} else {
					err = os.Link(outside, name)
				}
				if err != nil {
					t.Fatal(err)
				}
			case "leaf-mode":
				if err := os.Chmod(name, 0664); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := file.Read(name, 128); err == nil {
				t.Fatal("read accepted a changed unsafe path")
			}
			if _, err := file.Stat(name); err == nil {
				t.Fatal("stat accepted a changed unsafe path")
			}
			if err := file.Write(name, []byte("58900"), 0644, os.Geteuid(), os.Getegid()); err == nil {
				t.Fatal("write accepted a changed unsafe path")
			}
			data, err := os.ReadFile(outside)
			if err != nil || string(data) != "untouched" {
				t.Fatalf("outside file changed: %q, %v", data, err)
			}
		})
	}
}

func TestFIFORejectedWithoutBlocking(t *testing.T) {
	anchor, relative, name := fixture(t)
	file := openFixture(t, anchor, relative, os.Geteuid())
	if err := os.Remove(name); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(name, 0600); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() {
		_, err := file.Read(name, 128)
		finished <- err
	}()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("FIFO accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("reading FIFO blocked")
	}
}

func TestRestoreMissingLeaf(t *testing.T) {
	anchor, relative, name := fixture(t)
	file := openFixture(t, anchor, relative, os.Geteuid())
	if err := os.Remove(name); err != nil {
		t.Fatal(err)
	}
	if err := file.Write(name, []byte("58826"), 0644, os.Geteuid(), os.Getegid()); err != nil {
		t.Fatal(err)
	}
	data, err := file.Read(name, 128)
	if err != nil || string(data) != "58826" {
		t.Fatalf("restored = %q, %v", data, err)
	}
}

func TestBoundedReadsAndInvalidArguments(t *testing.T) {
	anchor, relative, name := fixture(t)
	file := openFixture(t, anchor, relative, os.Geteuid())
	for _, limit := range []int64{-1, 0, 3, 1<<63 - 1} {
		if _, err := file.Read(name, limit); err == nil {
			t.Fatalf("read accepted invalid bound %d", limit)
		}
	}
	if _, err := file.Read(name+"-other", 128); err == nil {
		t.Fatal("read accepted another path")
	}
	if _, err := file.Stat(name + "-other"); err == nil {
		t.Fatal("stat accepted another path")
	}
	if err := file.Write(name+"-other", []byte("58900"), 0644, os.Geteuid(), os.Getegid()); err == nil {
		t.Fatal("write accepted another path")
	}
	for _, mode := range []os.FileMode{0664, 0644 | os.ModeSetuid, 0755 | os.ModeDir} {
		if err := file.Write(name, []byte("58900"), mode, os.Geteuid(), os.Getegid()); err == nil {
			t.Fatalf("write accepted unsafe mode %v", mode)
		}
	}
	if err := file.Write(name, []byte("58900"), 0644, os.Geteuid(), -1); err == nil {
		t.Fatal("write accepted an invalid group")
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Read(name, 128); err == nil {
		t.Fatal("read accepted a closed file")
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	for _, badRelative := range []string{".", "../swversion", "/swversion", "cpguard/../swversion", ""} {
		if opened, err := openFile(anchor, badRelative, os.Geteuid(), false); err == nil {
			opened.Close()
			t.Fatalf("accepted relative path %q", badRelative)
		}
	}
	if opened, err := Open(anchor, relative, os.Geteuid()); err == nil {
		opened.Close()
		t.Fatal("public Open accepted a root-unprotected temporary anchor")
	}
}

func TestCleanupDoesNotRemoveReplacedTemporaryFile(t *testing.T) {
	anchor := temporaryDirectory(t)
	root, err := os.Open(anchor)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	temporary, name, err := createTemporary(root)
	if err != nil {
		t.Fatal(err)
	}
	info, err := temporary.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := temporary.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(anchor, name), filepath.Join(anchor, "original")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(anchor, name), []byte("unrelated"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := removeTemporary(root, name, info); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(anchor, name))
	if err != nil || string(data) != "unrelated" {
		t.Fatalf("replacement removed: %q, %v", data, err)
	}
	if err := removeTemporary(root, "missing", info); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}
