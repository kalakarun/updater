package durable

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

func Write(name string, data []byte, mode os.FileMode, uid, gid int) error {
	return writeAtomic(name, data, mode, uid, gid, true)
}

func WriteNew(name string, data []byte, mode os.FileMode) error {
	return writeAtomic(name, data, mode, -1, -1, false)
}

func writeAtomic(name string, data []byte, mode os.FileMode, uid, gid int, replace bool) (failure error) {
	temporary, err := os.CreateTemp(filepath.Dir(name), ".write-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			failure = errors.Join(failure, temporary.Close())
		}
		if err := os.Remove(temporaryName); err != nil && !errors.Is(err, os.ErrNotExist) {
			failure = errors.Join(failure, err)
		}
	}()
	written, err := temporary.Write(data)
	if err != nil {
		return err
	}
	if written != len(data) {
		return io.ErrShortWrite
	}
	if uid >= 0 {
		if err := temporary.Chown(uid, gid); err != nil {
			return err
		}
	}
	if err := temporary.Chmod(mode.Perm()); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	err = temporary.Close()
	closed = true
	if err != nil {
		return err
	}
	if replace {
		if err := os.Rename(temporaryName, name); err != nil {
			return err
		}
	} else {
		if err := os.Link(temporaryName, name); err != nil {
			return err
		}
		return errors.Join(os.Remove(temporaryName), SyncDir(filepath.Dir(name)))
	}
	return SyncDir(filepath.Dir(name))
}

func OpenRegular(name string) (*os.File, error) {
	before, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, errors.New("expected regular file")
	}
	file, err := openRegular(name)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err == nil && (!info.Mode().IsRegular() || !os.SameFile(before, info)) {
		err = errors.New("file changed while opening")
	}
	if err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return file, nil
}

func Read(name string, limit int64) (data []byte, failure error) {
	if limit < 1 || limit == int64(^uint64(0)>>1) {
		return nil, errors.New("invalid file size limit")
	}
	file, err := OpenRegular(name)
	if err != nil {
		return nil, err
	}
	defer func() { failure = errors.Join(failure, file.Close()) }()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		return nil, errors.New("file exceeds limit")
	}
	data, err = io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("file exceeds limit")
	}
	current, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	held, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !current.Mode().IsRegular() || !os.SameFile(info, current) ||
		info.Size() != int64(len(data)) || held.Size() != info.Size() || current.Size() != info.Size() ||
		!held.ModTime().Equal(info.ModTime()) || !current.ModTime().Equal(info.ModTime()) {
		return nil, errors.New("file changed while reading")
	}
	return data, nil
}

func Remove(name string) error {
	if err := os.Remove(name); err != nil {
		return err
	}
	return SyncDir(filepath.Dir(name))
}
