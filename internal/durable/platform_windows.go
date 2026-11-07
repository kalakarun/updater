package durable

import (
	"errors"
	"os"
)

func openRegular(name string) (*os.File, error) { return os.Open(name) }

func SyncDir(string) error             { return nil }
func Ownership(os.FileInfo) (int, int) { return -1, -1 }
func Protected(string, bool) error {
	return errors.New("production filesystem validation requires Linux")
}
func Lock(string) (*os.File, error) { return nil, errors.New("production locking requires Linux") }
