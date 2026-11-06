package main

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestBuildProfilePaths(t *testing.T) {
	if runtime.GOOS == "linux" && filepath.Join(versionAnchor, versionRelative) != versionFile {
		t.Fatal("version-file anchor does not match the configured path")
	}
	switch buildProfile {
	case "production":
		if stateDir != "/var/lib/cpguard-updater" || versionFile != "/opt/cpguard/app/setup/swversion" {
			t.Fatal("production paths changed", stateDir, versionFile)
		}
	case "manual-test":
		if stateDir != "/var/lib/cpguard-updater-manual-test/state" || versionFile != "/var/lib/cpguard-updater-manual-test/setup/swversion" {
			t.Fatal("manual-test paths are not isolated", stateDir, versionFile)
		}
	default:
		t.Fatal("unknown build profile", buildProfile)
	}
}
