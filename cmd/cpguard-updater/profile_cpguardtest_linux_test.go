//go:build cpguardtest

package main

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"cpguard/updater/internal/installer"
	"cpguard/updater/internal/network"
)

func TestManualProfileArchiveGuard(t *testing.T) {
	client, err := network.New(context.Background(), []string{"raw.githubusercontent.com"})
	if err != nil {
		t.Fatal(err)
	}
	source := profileArchives(client)
	if err := source.ValidateURL(sampleURL); err != nil {
		t.Fatal(err)
	}
	digest, err := hex.DecodeString(sampleSHA256)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"url", "size", "hash"} {
		t.Run(scenario, func(t *testing.T) {
			address := sampleURL
			length := int64(284)
			candidate := append([]byte(nil), digest...)
			switch scenario {
			case "url":
				address += ".different"
			case "size":
				length++
			case "hash":
				candidate[0] ^= 1
			}
			destination := filepath.Join(t.TempDir(), "archive.tar.gz")
			if err := source.Archive(address, destination, length, candidate); err == nil {
				t.Fatal("manual-test artifact guard accepted unsupported input")
			}
			if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("unsupported archive created an output file", err)
			}
		})
	}
}

func TestManualProfileInstallerGuard(t *testing.T) {
	for _, valid := range []bool{true, false} {
		t.Run(map[bool]string{true: "inspected-script", false: "different-script"}[valid], func(t *testing.T) {
			directory := t.TempDir()
			script := sampleScript
			if !valid {
				script += "\nprintf unexpected > unexpected.txt\n"
			}
			if err := os.WriteFile(filepath.Join(directory, "run.sh"), []byte(script), 0600); err != nil {
				t.Fatal(err)
			}
			err := profileInstaller(installer.Runner{})(context.Background(), directory)
			if (err == nil) != valid {
				t.Fatal("unexpected manual-test installer result", err)
			}
			_, logErr := os.Stat(filepath.Join(directory, "run.log"))
			if valid && logErr != nil {
				t.Fatal("sample did not create run.log", logErr)
			}
			if !valid {
				for _, name := range []string{"run.log", "unexpected.txt"} {
					if _, err := os.Lstat(filepath.Join(directory, name)); !errors.Is(err, os.ErrNotExist) {
						t.Fatal("unsupported installer executed", name, err)
					}
				}
			}
		})
	}
}
