package capacity

import (
	"errors"
	"syscall"
)

func Require(directory string, bytes uint64) error {
	var filesystem syscall.Statfs_t
	if err := syscall.Statfs(directory, &filesystem); err != nil {
		return err
	}
	if filesystem.Bsize == 0 || uint64(filesystem.Bavail) < (bytes+uint64(filesystem.Bsize)-1)/uint64(filesystem.Bsize) {
		return errors.New("insufficient available filesystem capacity")
	}
	return nil
}
