package durable

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

func openRegular(name string) (*os.File, error) {
	descriptor, err := syscall.Open(name, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	return os.NewFile(uintptr(descriptor), name), nil
}

func SyncDir(name string) error {
	directory, err := os.Open(name)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func Ownership(info os.FileInfo) (int, int) {
	stat := info.Sys().(*syscall.Stat_t)
	return int(stat.Uid), int(stat.Gid)
}

func Protected(name string, directory bool) error {
	name, err := filepath.Abs(name)
	if err != nil {
		return err
	}
	leaf := true
	for {
		info, err := os.Lstat(name)
		if err != nil {
			return err
		}
		uid, _ := Ownership(info)
		if uid != 0 || info.Mode()&(os.ModeSymlink|0022) != 0 || (leaf && directory && !info.IsDir()) || (leaf && !directory && !info.Mode().IsRegular()) || (!leaf && !info.IsDir()) {
			return errors.New("path must be root-owned, non-symlink, and not group/world writable: " + name)
		}
		parent := filepath.Dir(name)
		if parent == name {
			return nil
		}
		name = parent
		leaf = false
	}
}

func Lock(name string) (*os.File, error) {
	if err := Protected(filepath.Dir(name), true); err != nil {
		return nil, err
	}
	descriptor, err := syscall.Open(name, syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(descriptor), name)
	if err := Protected(name, false); err != nil {
		file.Close()
		return nil, err
	}
	if err := syscall.Flock(descriptor, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}
