package durable

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

func Write(name string, data []byte, mode os.FileMode, uid, gid int) error {
	temporary, err := os.CreateTemp(filepath.Dir(name), ".write-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	defer temporary.Close()
	if uid >= 0 {
		if err := temporary.Chown(uid, gid); err != nil {
			return err
		}
	}
	if err := temporary.Chmod(mode.Perm()); err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, name); err != nil {
		return err
	}
	return SyncDir(filepath.Dir(name))
}

func Read(name string, limit int64) ([]byte, error) {
	info, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("expected bounded regular file")
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("file exceeds limit")
	}
	return data, nil
}

func Remove(name string) error {
	if err := os.Remove(name); err != nil {
		return err
	}
	return SyncDir(filepath.Dir(name))
}
