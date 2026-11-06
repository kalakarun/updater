//go:build !linux

package versionfile

import (
	"errors"
	"os"
)

type File struct{}

type Appender struct{}

func OpenAppend(anchor, relative string, allowedUID int) (*Appender, error) {
	return nil, errors.New("application result log integration requires Linux")
}

func (appender *Appender) Append(data []byte) error {
	return errors.New("application result log integration requires Linux")
}

func (appender *Appender) Close() error {
	return nil
}

func Open(anchor, relative string, allowedUID int) (*File, error) {
	return nil, errors.New("application version file integration requires Linux")
}

func (file *File) Read(name string, maximum int64) ([]byte, error) {
	return nil, errors.New("application version file integration requires Linux")
}

func (file *File) Stat(name string) (os.FileInfo, error) {
	return nil, errors.New("application version file integration requires Linux")
}

func (file *File) Write(name string, data []byte, mode os.FileMode, uid, gid int) error {
	return errors.New("application version file integration requires Linux")
}

func (file *File) Close() error {
	return nil
}
