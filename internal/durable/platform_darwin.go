package durable

import (
	"errors"
	"os"
	"syscall"
)

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
