package durable

import (
	"errors"
	"os"
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
	file, err := os.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}
func Ownership(info os.FileInfo) (int, int) {
	stat := info.Sys().(*syscall.Stat_t)
	return int(stat.Uid), int(stat.Gid)
}
func Protected(string, bool) error  { return errors.New("production updater requires Linux") }
func Lock(string) (*os.File, error) { return nil, errors.New("production updater requires Linux") }
