package updater

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"

	"cpguard/updater/internal/archive"
	"cpguard/updater/internal/capacity"
	"cpguard/updater/internal/durable"
	"cpguard/updater/internal/manifest"
	"cpguard/updater/internal/release"
	"cpguard/updater/internal/state"
)

type Archives interface {
	ValidateURL(string) error
	Archive(string, string, int64, []byte) error
}

type Service struct {
	KeyA         ed25519.PublicKey
	MetadataURL  string
	StateFile    string
	VersionFile  string
	TempParent   string
	Metadata     func(string, int64) ([]byte, error)
	Archives     Archives
	Install      func(context.Context, string) error
	Now          func() time.Time
	ReadVersion  func(string, int64) ([]byte, error)
	StatVersion  func(string) (os.FileInfo, error)
	WriteVersion func(string, []byte, os.FileMode, int, int) error
}

type Result struct {
	Outcome   string `json:"outcome"`
	Installed string `json:"installed"`
	Available string `json:"available"`
	Sequence  uint64 `json:"sequence"`
}

func (service Service) Run(ctx context.Context, install bool) (result Result, failure error) {
	if service.Metadata == nil || service.Archives == nil || (install && service.Install == nil) {
		return result, errors.New("updater integration is incomplete")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	data, err := service.Metadata(service.MetadataURL, manifest.MaxBytes)
	if err != nil {
		return result, err
	}
	now := time.Now()
	if service.Now != nil {
		now = service.Now()
	}
	verified, err := manifest.Verify(data, service.KeyA, now, service.Archives.ValidateURL)
	if err != nil {
		return result, err
	}
	if err := state.Accept(service.StateFile, verified); err != nil {
		return result, err
	}
	read := service.ReadVersion
	if read == nil {
		read = durable.Read
	}
	stat := service.StatVersion
	if stat == nil {
		stat = os.Lstat
	}
	old, err := read(service.VersionFile, 128)
	if err != nil {
		return result, err
	}
	installed, err := release.ParseInstalled(old)
	if err != nil {
		return result, err
	}
	candidate, err := release.ParseVersion(verified.Payload.Version)
	if err != nil {
		return result, err
	}
	result = Result{Outcome: "available", Installed: installed.String(), Available: candidate.String(), Sequence: verified.Payload.Sequence}
	if candidate <= installed {
		result.Outcome = "up_to_date"
		return result, nil
	}
	if !install {
		return result, nil
	}
	info, err := stat(service.VersionFile)
	if err != nil {
		return result, err
	}
	uid, gid := durable.Ownership(info)
	write := service.WriteVersion
	if write == nil {
		write = durable.Write
	}
	unchanged := func() (bool, error) {
		current, err := read(service.VersionFile, 128)
		if err != nil {
			return false, err
		}
		currentInfo, err := stat(service.VersionFile)
		if err != nil {
			return false, err
		}
		currentUID, currentGID := durable.Ownership(currentInfo)
		return bytes.Equal(current, old) && os.SameFile(info, currentInfo) &&
			currentUID == uid && currentGID == gid && currentInfo.Mode().Perm() == info.Mode().Perm(), nil
	}
	restore := func() error {
		current, err := read(service.VersionFile, 128)
		if err == nil && bytes.Equal(current, old) {
			currentInfo, statErr := stat(service.VersionFile)
			if statErr == nil {
				currentUID, currentGID := durable.Ownership(currentInfo)
				if currentUID == uid && currentGID == gid && currentInfo.Mode().Perm() == info.Mode().Perm() {
					return nil
				}
			}
		}
		return write(service.VersionFile, old, info.Mode(), uid, gid)
	}
	temporary, err := os.MkdirTemp(service.TempParent, "update-")
	if err != nil {
		return result, err
	}
	defer func() { failure = errors.Join(failure, os.RemoveAll(temporary)) }()
	if err := capacity.Require(temporary, uint64(verified.Payload.File.Size)+67108864); err != nil {
		return result, err
	}
	archivePath := filepath.Join(temporary, "release.tar.gz")
	digest, err := hex.DecodeString(verified.Payload.File.SHA256)
	if err != nil {
		return result, err
	}
	if err := service.Archives.Archive(verified.Payload.File.URL, archivePath, verified.Payload.File.Size, digest); err != nil {
		return result, err
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return result, err
	}
	packagePath := filepath.Join(temporary, "package")
	extractErr := archive.ExtractContext(ctx, file, packagePath, archive.DefaultLimits)
	closeErr := file.Close()
	if extractErr != nil || closeErr != nil {
		return result, errors.Join(extractErr, closeErr)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if matches, err := unchanged(); err != nil || !matches {
		return result, errors.Join(errors.New("swversion changed before installation"), err)
	}
	if err := service.Install(ctx, packagePath); err != nil {
		return result, errors.Join(err, restore())
	}
	if matches, err := unchanged(); err != nil || !matches {
		return result, errors.Join(errors.New("run.sh must leave swversion to the updater"), err, restore())
	}
	if err := write(service.VersionFile, candidate.Installed(old), info.Mode(), uid, gid); err != nil {
		return result, errors.Join(err, restore())
	}
	result.Installed = candidate.String()
	result.Outcome = "installed"
	return result, nil
}
