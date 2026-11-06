package testpackage

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cpguard/updater/internal/durable"
	"cpguard/updater/internal/installer"
	"cpguard/updater/internal/manifest"
	"cpguard/updater/internal/network"
	"cpguard/updater/internal/release"
	"cpguard/updater/internal/site"
	"cpguard/updater/internal/state"
	"cpguard/updater/internal/updater"
	_ "golang.org/x/crypto/x509roots/fallback"
)

const githubArchiveURL = "https://raw.githubusercontent.com/kalakarun/uptest/403e6a06a197e2a0ba1b4f24ba9724c3ef47f6c6/cpg-1.0.1.tar.gz"
const githubArchiveSHA256 = "aa38e471520f2f2bc22f80cb23caf6dcd1dde6ec5d55003d8fc61165c887ba2f"
const githubArchiveSize int64 = 284
const githubInitialVersion = "10000"
const githubSampleScript = `#!/bin/sh

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
timestamp=$(date '+%Y-%m-%d %H:%M:%S %z')
printf '%s - ran successfully - Sample message\n' "$timestamp" >> "$script_dir/run.log"`

func validateGitHubSample(payload manifest.Payload) error {
	if payload.File.URL != githubArchiveURL || payload.File.SHA256 != githubArchiveSHA256 || payload.File.Size != githubArchiveSize {
		return errors.New("GitHub test only accepts the inspected, commit-pinned 284-byte sample archive")
	}
	version, err := release.ParseVersion(payload.Version)
	if err != nil {
		return err
	}
	initial, err := release.ParseInstalled([]byte(githubInitialVersion))
	if err != nil {
		return err
	}
	if version <= initial {
		return errors.New("GitHub sample version must be newer than the isolated test version 1.0.0")
	}
	return nil
}

func runGitHubSample(ctx context.Context, directory string) ([]byte, error) {
	script, err := durable.Read(filepath.Join(directory, "run.sh"), 1024)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(script, []byte(githubSampleScript)) {
		return nil, errors.New("GitHub test refuses an installer different from the inspected sample run.sh")
	}
	if err := (installer.Runner{Timeout: time.Minute}).Run(ctx, directory); err != nil {
		return nil, err
	}
	output, err := durable.Read(filepath.Join(directory, "run.log"), 4096)
	if err != nil {
		return nil, err
	}
	if !strings.Contains(string(output), " - ran successfully - Sample message\n") {
		return nil, errors.New("sample installer did not produce the expected run.log")
	}
	return output, nil
}

func githubTestDirectory(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp("/tmp", "cpguard-github-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(directory); err != nil {
			t.Error(err)
		}
	})
	return directory
}

func TestGitHubSamplePolicy(t *testing.T) {
	for _, scenario := range []string{"valid", "url", "hash", "size", "old-version", "invalid-version"} {
		t.Run(scenario, func(t *testing.T) {
			payload := manifest.Payload{Version: "1.0.1", File: manifest.File{URL: githubArchiveURL, SHA256: githubArchiveSHA256, Size: githubArchiveSize}}
			switch scenario {
			case "url":
				payload.File.URL = strings.Replace(githubArchiveURL, "kalakarun/", "another-owner/", 1)
			case "hash":
				payload.File.SHA256 = strings.Repeat("0", 64)
			case "size":
				payload.File.Size++
			case "old-version":
				payload.Version = "1.0.0"
			case "invalid-version":
				payload.Version = "invalid"
			}
			err := validateGitHubSample(payload)
			if (err == nil) != (scenario == "valid") {
				t.Fatal("unexpected GitHub sample policy result", scenario, err)
			}
		})
	}
}

func TestGitHubSampleInstaller(t *testing.T) {
	for _, valid := range []bool{true, false} {
		t.Run(map[bool]string{true: "inspected-script", false: "different-script"}[valid], func(t *testing.T) {
			directory := githubTestDirectory(t)
			script := githubSampleScript
			if !valid {
				script += "\nprintf unexpected > unexpected.txt\n"
			}
			if err := os.WriteFile(filepath.Join(directory, "run.sh"), []byte(script), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := runGitHubSample(context.Background(), directory)
			if (err == nil) != valid {
				t.Fatal("unexpected sample installer result", err)
			}
			if !valid {
				for _, name := range []string{"run.log", "unexpected.txt"} {
					if _, err := os.Lstat(filepath.Join(directory, name)); !errors.Is(err, os.ErrNotExist) {
						t.Fatal("unapproved installer executed", name, err)
					}
				}
			}
		})
	}
}

func TestGitHubSignedSampleFlow(t *testing.T) {
	if os.Getenv("CPGUARD_GITHUB_TEST") != "1" {
		t.Skip("set CPGUARD_GITHUB_TEST=1 after publishing fresh signed GitHub metadata")
	}
	config, keyA, err := site.Load()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	metadataClient, err := network.New(ctx, []string{"raw.githubusercontent.com"})
	if err != nil {
		t.Fatal(err)
	}
	archives, err := network.New(ctx, config.CDNHosts)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := metadataClient.DownloadFile(config.MetadataURL, manifest.MaxBytes)
	if err != nil {
		t.Fatal("GitHub metadata download failed", err)
	}
	verified, err := manifest.Verify(metadata, keyA, time.Now(), archives.ValidateURL)
	if err != nil {
		t.Fatal("GitHub metadata must be a fresh envelope signed by Key A, not {} or offline-test metadata", err)
	}
	if err := validateGitHubSample(verified.Payload); err != nil {
		t.Fatal(err)
	}
	directory := githubTestDirectory(t)
	versionPath := filepath.Join(directory, "swversion")
	statePath := filepath.Join(directory, "state.json")
	if err := state.Initialize(statePath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(versionPath, []byte(githubInitialVersion), 0600); err != nil {
		t.Fatal(err)
	}
	installs := 0
	service := updater.Service{
		KeyA: keyA, MetadataURL: config.MetadataURL, StateFile: statePath, VersionFile: versionPath, TempParent: directory, Archives: archives,
		Metadata: func(address string, limit int64) ([]byte, error) {
			current, err := metadataClient.DownloadFile(address, limit)
			if err != nil {
				return nil, err
			}
			if !bytes.Equal(current, metadata) {
				return nil, errors.New("GitHub metadata changed during the test; rerun against one published release")
			}
			return current, nil
		},
		Install: func(ctx context.Context, packagePath string) error {
			installs++
			output, err := runGitHubSample(ctx, packagePath)
			if err == nil {
				t.Logf("verified sample run.log before temporary extraction cleanup: %s", output)
			}
			return err
		},
	}
	for _, step := range []struct {
		install bool
		outcome string
		calls   int
	}{{false, "available", 0}, {true, "installed", 1}, {true, "up_to_date", 1}} {
		result, err := service.Run(ctx, step.install)
		if err != nil || result.Outcome != step.outcome || installs != step.calls {
			t.Fatal("unexpected HTTPS updater flow result", result, installs, err)
		}
		t.Logf("HTTPS flow: %s", result.Outcome)
	}
	version, err := os.ReadFile(versionPath)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := release.ParseVersion(verified.Payload.Version)
	if err != nil || !bytes.Equal(version, candidate.Installed([]byte(githubInitialVersion))) {
		t.Fatal("temporary version was not recorded correctly", string(version), err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "update-") {
			t.Fatal("temporary update files were not cleaned up")
		}
	}
	t.Logf("test swversion %s -> %s; real cPGuard paths were not used", githubInitialVersion, version)
}
