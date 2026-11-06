package versionfile

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"cpguard/updater/internal/durable"
	"golang.org/x/sys/unix"
)

type File struct {
	anchor       string
	name         string
	leaf         string
	allowedUID   int
	secureAnchor bool
	roots        []*os.File
	children     []string
	identities   []os.FileInfo
}

func Open(anchor, relative string, allowedUID int) (*File, error) {
	return openFile(anchor, relative, allowedUID, true)
}

func openFile(anchor, relative string, allowedUID int, secureAnchor bool) (_ *File, failure error) {
	if !filepath.IsAbs(anchor) || filepath.Clean(anchor) != anchor || !filepath.IsLocal(relative) || filepath.Clean(relative) != relative || relative == "." || allowedUID < 0 {
		return nil, errors.New("invalid anchored version path or allowed owner")
	}
	if secureAnchor {
		if err := durable.Protected(anchor, true); err != nil {
			return nil, err
		}
	}
	file := &File{anchor: anchor, name: filepath.Join(anchor, relative), allowedUID: allowedUID, secureAnchor: secureAnchor}
	defer func() {
		if failure != nil {
			failure = errors.Join(failure, file.Close())
		}
	}()
	anchorInfo, err := os.Lstat(anchor)
	if err != nil {
		return nil, err
	}
	if err := file.checkInfo(anchorInfo, true); err != nil {
		return nil, err
	}
	root, err := os.OpenFile(anchor, os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	file.roots = append(file.roots, root)
	file.identities = append(file.identities, anchorInfo)
	parts := strings.Split(relative, string(filepath.Separator))
	file.leaf = parts[len(parts)-1]
	for _, child := range parts[:len(parts)-1] {
		parent := file.roots[len(file.roots)-1]
		info, err := lstatAt(parent, child)
		if err != nil {
			return nil, err
		}
		if err := file.checkInfo(info, true); err != nil {
			return nil, err
		}
		childRoot, err := openAt(parent, child, os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if err != nil {
			return nil, err
		}
		file.roots = append(file.roots, childRoot)
		file.children = append(file.children, child)
		file.identities = append(file.identities, info)
		if err := file.checkDirectories(); err != nil {
			return nil, err
		}
	}
	if _, err := file.Stat(file.name); err != nil {
		return nil, err
	}
	return file, nil
}

func (file *File) checkInfo(info os.FileInfo, directory bool) error {
	uid, _ := durable.Ownership(info)
	if (uid != 0 && uid != file.allowedUID) || info.Mode()&(0022|os.ModeSymlink|os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || (directory && !info.IsDir()) || (!directory && !info.Mode().IsRegular()) {
		return fmt.Errorf("version path must be an expected non-symlink type owned by root or UID %d, without group/world write or special modes: %s", file.allowedUID, info.Name())
	}
	if !directory && info.Sys().(*syscall.Stat_t).Nlink != 1 {
		return errors.New("version file must not have hard links")
	}
	return nil
}

func (file *File) checkDirectories() error {
	if len(file.roots) == 0 {
		return errors.New("version file is closed")
	}
	if file.secureAnchor {
		if err := durable.Protected(file.anchor, true); err != nil {
			return err
		}
	}
	for index, root := range file.roots {
		var current os.FileInfo
		var err error
		if index == 0 {
			current, err = os.Lstat(file.anchor)
		} else {
			current, err = lstatAt(file.roots[index-1], file.children[index-1])
		}
		if err != nil {
			return err
		}
		held, err := root.Stat()
		if err != nil {
			return err
		}
		if !os.SameFile(current, file.identities[index]) || !os.SameFile(current, held) {
			return errors.New("version directory was replaced or moved")
		}
		if err := file.checkInfo(current, true); err != nil {
			return err
		}
		if err := file.checkInfo(held, true); err != nil {
			return err
		}
	}
	return nil
}

func (file *File) validateName(name string) error {
	if file == nil || name != file.name {
		return errors.New("unexpected version file path")
	}
	return file.checkDirectories()
}

func (file *File) openRead(name string) (*os.File, os.FileInfo, error) {
	if err := file.validateName(name); err != nil {
		return nil, nil, err
	}
	root := file.roots[len(file.roots)-1]
	before, err := lstatAt(root, file.leaf)
	if err != nil {
		return nil, nil, err
	}
	if err := file.checkInfo(before, false); err != nil {
		return nil, nil, err
	}
	opened, err := openAt(root, file.leaf, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, err
	}
	info, err := opened.Stat()
	if err == nil {
		err = file.checkInfo(info, false)
	}
	if err == nil && !os.SameFile(before, info) {
		err = errors.New("version file was replaced while opening")
	}
	if err == nil {
		current, statErr := lstatAt(root, file.leaf)
		if statErr != nil {
			err = statErr
		} else if !os.SameFile(info, current) {
			err = errors.New("version file was replaced while opening")
		}
	}
	if err != nil {
		return nil, nil, errors.Join(err, opened.Close())
	}
	return opened, info, nil
}

func (file *File) Read(name string, maximum int64) ([]byte, error) {
	if maximum < 1 || maximum == int64(^uint64(0)>>1) {
		return nil, errors.New("invalid version file size limit")
	}
	opened, info, err := file.openRead(name)
	if err != nil {
		return nil, err
	}
	defer opened.Close()
	if info.Size() > maximum {
		return nil, errors.New("version file exceeds size limit")
	}
	data, err := io.ReadAll(io.LimitReader(opened, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, errors.New("version file exceeds size limit")
	}
	if err := file.checkDirectories(); err != nil {
		return nil, err
	}
	current, err := lstatAt(file.roots[len(file.roots)-1], file.leaf)
	if err != nil {
		return nil, err
	}
	if !os.SameFile(info, current) {
		return nil, errors.New("version file was replaced while reading")
	}
	if err := file.checkInfo(current, false); err != nil {
		return nil, err
	}
	return data, nil
}

func (file *File) Stat(name string) (os.FileInfo, error) {
	opened, info, err := file.openRead(name)
	if err != nil {
		return nil, err
	}
	if err := errors.Join(opened.Close(), file.checkDirectories()); err != nil {
		return nil, err
	}
	return info, nil
}

func (file *File) Write(name string, data []byte, mode os.FileMode, uid, gid int) (failure error) {
	if err := file.validateName(name); err != nil {
		return err
	}
	if (uid != 0 && uid != file.allowedUID) || gid < 0 || mode&(0022|os.ModeType|os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return errors.New("unsafe version file ownership or permissions")
	}
	root := file.roots[len(file.roots)-1]
	original, err := lstatAt(root, file.leaf)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		if err := file.checkInfo(original, false); err != nil {
			return err
		}
	}
	temporary, temporaryName, err := createTemporary(root)
	if err != nil {
		return err
	}
	temporaryInfo, err := temporary.Stat()
	if err != nil {
		return errors.Join(err, temporary.Close())
	}
	defer func() {
		failure = errors.Join(failure, temporary.Close(), removeTemporary(root, temporaryName, temporaryInfo))
	}()
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Chown(uid, gid); err != nil {
		return err
	}
	if err := temporary.Chmod(mode.Perm()); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := file.checkDirectories(); err != nil {
		return err
	}
	current, err := lstatAt(root, file.leaf)
	if original == nil {
		if !errors.Is(err, os.ErrNotExist) {
			return errors.Join(errors.New("version file appeared during write"), err)
		}
	} else if err != nil {
		return err
	} else if !os.SameFile(original, current) {
		return errors.New("version file changed during write")
	} else if err := file.checkInfo(current, false); err != nil {
		return err
	}
	current, err = lstatAt(root, temporaryName)
	if err != nil {
		return err
	}
	if !os.SameFile(temporaryInfo, current) {
		return errors.New("temporary version file was replaced")
	}
	if err := file.checkInfo(current, false); err != nil {
		return err
	}
	if err := unix.Renameat(int(root.Fd()), temporaryName, int(root.Fd()), file.leaf); err != nil {
		return err
	}
	current, err = lstatAt(root, file.leaf)
	if err != nil {
		return err
	}
	if !os.SameFile(temporaryInfo, current) {
		return errors.New("new version file was replaced")
	}
	if err := file.checkInfo(current, false); err != nil {
		return err
	}
	return errors.Join(root.Sync(), file.checkDirectories())
}

func createTemporary(root *os.File) (*os.File, string, error) {
	for range 10 {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return nil, "", err
		}
		name := ".swversion-" + hex.EncodeToString(random[:])
		temporary, err := openAt(root, name, os.O_CREATE|os.O_EXCL|os.O_WRONLY|unix.O_NOFOLLOW, 0600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return temporary, name, err
	}
	return nil, "", errors.New("cannot allocate a unique temporary version file")
}

func removeTemporary(root *os.File, name string, expected os.FileInfo) error {
	info, err := lstatAt(root, name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !os.SameFile(info, expected) {
		return nil
	}
	return unix.Unlinkat(int(root.Fd()), name, 0)
}

func openAt(directory *os.File, name string, flags int, mode os.FileMode) (*os.File, error) {
	descriptor, err := unix.Openat(int(directory.Fd()), name, flags|unix.O_CLOEXEC, uint32(mode.Perm()))
	if err != nil {
		return nil, &os.PathError{Op: "openat", Path: name, Err: err}
	}
	return os.NewFile(uintptr(descriptor), name), nil
}

func lstatAt(directory *os.File, name string) (os.FileInfo, error) {
	opened, err := openAt(directory, name, unix.O_PATH|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	info, err := opened.Stat()
	return info, errors.Join(err, opened.Close())
}

func (file *File) Close() error {
	if file == nil {
		return nil
	}
	var failure error
	for index := len(file.roots) - 1; index >= 0; index-- {
		failure = errors.Join(failure, file.roots[index].Close())
	}
	file.roots = nil
	return failure
}
