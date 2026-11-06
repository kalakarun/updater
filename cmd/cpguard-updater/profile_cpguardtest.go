//go:build cpguardtest

package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"path/filepath"

	"cpguard/updater/internal/durable"
	"cpguard/updater/internal/installer"
	"cpguard/updater/internal/network"
	"cpguard/updater/internal/updater"
)

const buildProfile = "manual-test"
const stateDir = "/var/lib/cpguard-updater-manual-test/state"
const versionFile = "/var/lib/cpguard-updater-manual-test/setup/swversion"
const versionAnchor = "/var/lib"
const versionRelative = "cpguard-updater-manual-test/setup/swversion"
const sampleURL = "https://raw.githubusercontent.com/kalakarun/uptest/403e6a06a197e2a0ba1b4f24ba9724c3ef47f6c6/cpg-1.0.1.tar.gz"
const sampleSHA256 = "aa38e471520f2f2bc22f80cb23caf6dcd1dde6ec5d55003d8fc61165c887ba2f"
const sampleScript = `#!/bin/sh

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
timestamp=$(date '+%Y-%m-%d %H:%M:%S %z')
printf '%s - ran successfully - Sample message\n' "$timestamp" >> "$script_dir/run.log"`

type sampleArchives struct {
	*network.Client
}

func (source sampleArchives) ValidateURL(address string) error {
	if address != sampleURL {
		return errors.New("manual-test build only accepts the inspected GitHub sample archive URL")
	}
	return source.Client.ValidateURL(address)
}

func (source sampleArchives) Archive(address, destination string, length int64, digest []byte) error {
	if err := source.ValidateURL(address); err != nil {
		return err
	}
	if length != 284 || hex.EncodeToString(digest) != sampleSHA256 {
		return errors.New("manual-test build only accepts the inspected 284-byte sample and its exact SHA-256")
	}
	return source.Client.Archive(address, destination, length, digest)
}

func profileArchives(client *network.Client) updater.Archives {
	return sampleArchives{Client: client}
}

func profileInstaller(runner installer.Runner) func(context.Context, string) error {
	return func(ctx context.Context, directory string) error {
		script, err := durable.Read(filepath.Join(directory, "run.sh"), 1024)
		if err != nil {
			return err
		}
		if !bytes.Equal(script, []byte(sampleScript)) {
			return errors.New("manual-test build refuses an installer different from the inspected log-only run.sh")
		}
		return runner.Run(ctx, directory)
	}
}
