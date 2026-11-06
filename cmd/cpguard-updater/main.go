package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"cpguard/updater/internal/durable"
	"cpguard/updater/internal/installer"
	"cpguard/updater/internal/network"
	"cpguard/updater/internal/release"
	"cpguard/updater/internal/site"
	"cpguard/updater/internal/state"
	"cpguard/updater/internal/updater"
	_ "golang.org/x/crypto/x509roots/fallback"
)

const stateDir = "/var/lib/cpguard-updater"
const versionFile = "/opt/cpguard/app/setup/swversion"

type logWriter struct{ logger *slog.Logger }

func (writer logWriter) Write(data []byte) (int, error) {
	writer.logger.Info("installer_output", "output", string(data))
	return len(data), nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], logger); err != nil {
		logger.Error("update_failed", "error", err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, arguments []string, logger *slog.Logger) error {
	command := "apply"
	if len(arguments) > 1 {
		return errors.New("usage: cpguard-updater [apply|check|bootstrap|version]")
	}
	if len(arguments) == 1 {
		command = arguments[0]
	}
	if command == "version" {
		fmt.Println("cpguard-updater signed-json", runtime.Version(), runtime.GOOS, runtime.GOARCH)
		return nil
	}
	if command != "apply" && command != "check" && command != "bootstrap" {
		return errors.New("unknown command")
	}
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		return errors.New("updater requires Linux amd64 or arm64")
	}
	if os.Geteuid() != 0 {
		return errors.New("updater must run as root")
	}
	config, keyA, err := site.Load()
	if err != nil {
		return err
	}
	address, err := url.Parse(config.MetadataURL)
	if err != nil {
		return err
	}
	metadataClient, err := network.New(ctx, []string{address.Hostname()})
	if err != nil {
		return err
	}
	if err := metadataClient.ValidateURL(config.MetadataURL); err != nil {
		return err
	}
	archives, err := network.New(ctx, config.CDNHosts)
	if err != nil {
		return err
	}
	if err := durable.Protected(versionFile, false); err != nil {
		return err
	}
	if command == "bootstrap" {
		data, err := durable.Read(versionFile, 128)
		if err != nil {
			return err
		}
		if _, err := release.ParseInstalled(data); err != nil {
			return err
		}
		if err := durable.Protected(filepath.Dir(stateDir), true); err != nil {
			return err
		}
		if err := os.Mkdir(stateDir, 0700); err != nil {
			return errors.New("bootstrap requires new state; never delete existing state to bypass this error")
		}
		if err := durable.SyncDir(filepath.Dir(stateDir)); err != nil {
			return err
		}
	}
	if err := durable.Protected(stateDir, true); err != nil {
		return err
	}
	lock, err := durable.Lock(filepath.Join(stateDir, "lock"))
	if err != nil {
		return err
	}
	defer lock.Close()
	statePath := filepath.Join(stateDir, "state.json")
	if command == "bootstrap" {
		return state.Initialize(statePath)
	}
	for _, oldName := range []string{"metadata", "journal.json", "transaction", "status.json"} {
		if _, err := os.Lstat(filepath.Join(stateDir, oldName)); !errors.Is(err, os.ErrNotExist) {
			return errors.New("legacy TUF/transaction state found; explicit migration is required")
		}
	}
	if err := durable.Protected(statePath, false); err != nil {
		return err
	}
	service := updater.Service{KeyA: keyA, MetadataURL: config.MetadataURL, StateFile: statePath, VersionFile: versionFile, TempParent: stateDir, Metadata: metadataClient.DownloadFile, Archives: archives, Install: (installer.Runner{Log: logWriter{logger}}).Run}
	result, err := service.Run(ctx, command == "apply")
	if err != nil {
		return err
	}
	logger.Info("update_result", "outcome", result.Outcome, "installed", result.Installed, "available", result.Available, "sequence", result.Sequence)
	return json.NewEncoder(os.Stdout).Encode(result)
}
