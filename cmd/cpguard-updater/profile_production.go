//go:build !cpguardtest

package main

import (
	"context"

	"cpguard/updater/internal/installer"
	"cpguard/updater/internal/network"
	"cpguard/updater/internal/updater"
)

const buildProfile = "production"
const stateDir = "/var/lib/cpguard-updater"
const versionFile = "/opt/cpguard/app/setup/swversion"
const versionAnchor = "/opt"
const versionRelative = "cpguard/app/setup/swversion"

func profileArchives(client *network.Client) updater.Archives {
	return client
}

func profileInstaller(runner installer.Runner) func(context.Context, string) error {
	return runner.Run
}
