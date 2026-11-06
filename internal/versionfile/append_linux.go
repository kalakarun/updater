package versionfile

import (
	"errors"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

type Appender struct {
	file  *File
	mutex sync.Mutex
}

func OpenAppend(anchor, relative string, allowedUID int) (*Appender, error) {
	return openAppend(anchor, relative, allowedUID, true)
}

func openAppend(anchor, relative string, allowedUID int, secureAnchor bool) (_ *Appender, failure error) {
	file, err := openParent(anchor, relative, allowedUID, secureAnchor)
	if err != nil {
		return nil, err
	}
	appender := &Appender{file: file}
	defer func() {
		if failure != nil {
			failure = errors.Join(failure, appender.Close())
		}
	}()
	opened, created, err := appender.open()
	if err != nil {
		return nil, err
	}
	if err := errors.Join(opened.Sync(), opened.Close()); err != nil {
		return nil, err
	}
	if created {
		if err := file.roots[len(file.roots)-1].Sync(); err != nil {
			return nil, err
		}
	}
	if err := file.checkDirectories(); err != nil {
		return nil, err
	}
	return appender, nil
}

func (appender *Appender) open() (*os.File, bool, error) {
	file := appender.file
	if err := file.validateName(file.name); err != nil {
		return nil, false, err
	}
	root := file.roots[len(file.roots)-1]
	for range 5 {
		before, err := lstatAt(root, file.leaf)
		created := errors.Is(err, os.ErrNotExist)
		if err != nil && !created {
			return nil, false, err
		}
		flags := os.O_WRONLY | os.O_APPEND | unix.O_NOFOLLOW | unix.O_NONBLOCK
		if created {
			flags |= os.O_CREATE | os.O_EXCL
		} else if err := file.checkInfo(before, false); err != nil {
			return nil, false, err
		}
		opened, err := openAt(root, file.leaf, flags, 0644)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) || (created && errors.Is(err, os.ErrExist)) {
				continue
			}
			return nil, false, err
		}
		info, err := opened.Stat()
		if err == nil {
			err = file.checkInfo(info, false)
		}
		if err != nil {
			return nil, false, errors.Join(err, opened.Close())
		}
		if !created && !os.SameFile(before, info) {
			if err := opened.Close(); err != nil {
				return nil, false, err
			}
			continue
		}
		current, err := lstatAt(root, file.leaf)
		if errors.Is(err, os.ErrNotExist) || (err == nil && !os.SameFile(info, current)) {
			if err := opened.Close(); err != nil {
				return nil, false, err
			}
			continue
		}
		if err == nil {
			err = file.checkInfo(current, false)
		}
		if err == nil {
			err = file.checkDirectories()
		}
		if err != nil {
			return nil, false, errors.Join(err, opened.Close())
		}
		return opened, created, nil
	}
	return nil, false, errors.New("result log changed repeatedly while opening")
}

func (appender *Appender) checkOpened(opened *os.File) error {
	file := appender.file
	if err := file.checkDirectories(); err != nil {
		return err
	}
	info, err := opened.Stat()
	if err != nil {
		return err
	}
	if err := file.checkInfo(info, false); err != nil {
		return err
	}
	current, err := lstatAt(file.roots[len(file.roots)-1], file.leaf)
	if err != nil {
		return err
	}
	if !os.SameFile(current, info) {
		return errors.New("result log was replaced while appending")
	}
	return file.checkInfo(current, false)
}

func (appender *Appender) Append(data []byte) (failure error) {
	if appender == nil || appender.file == nil {
		return errors.New("result log is closed")
	}
	if len(data) == 0 || len(data) > 4096 {
		return errors.New("result log entry must contain 1 to 4096 bytes")
	}
	appender.mutex.Lock()
	defer appender.mutex.Unlock()
	opened, created, err := appender.open()
	if err != nil {
		return err
	}
	defer func() { failure = errors.Join(failure, opened.Close()) }()
	if err := lockAppend(opened); err != nil {
		return err
	}
	defer func() { failure = errors.Join(failure, unix.Flock(int(opened.Fd()), unix.LOCK_UN)) }()
	if err := appender.checkOpened(opened); err != nil {
		return err
	}
	written, err := opened.Write(data)
	if err != nil {
		return err
	}
	if written != len(data) {
		return io.ErrShortWrite
	}
	if err := opened.Sync(); err != nil {
		return err
	}
	if created {
		if err := appender.file.roots[len(appender.file.roots)-1].Sync(); err != nil {
			return err
		}
	}
	return appender.checkOpened(opened)
}

func lockAppend(opened *os.File) error {
	for attempt := 0; attempt < 20; attempt++ {
		if attempt != 0 {
			time.Sleep(10 * time.Millisecond)
		}
		err := unix.Flock(int(opened.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) && !errors.Is(err, unix.EINTR) {
			return err
		}
	}
	return errors.New("result log is busy")
}

func (appender *Appender) Close() error {
	if appender == nil {
		return nil
	}
	appender.mutex.Lock()
	defer appender.mutex.Unlock()
	return appender.file.Close()
}
