//go:build !cpguardtest

package main

import (
	"context"
	"errors"
	"fmt"
	"os/user"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"cpguard/updater/internal/installer"
	"cpguard/updater/internal/network"
	"cpguard/updater/internal/updater"
	"cpguard/updater/internal/versionfile"
)

type updaterProfile struct {
	name            string
	stateDir        string
	versionFile     string
	versionAnchor   string
	versionRelative string
}

var buildProfile = updaterProfile{
	name:            "production",
	stateDir:        "/var/lib/cpguard-updater",
	versionFile:     "/opt/cpguard/app/setup/swversion",
	versionAnchor:   "/opt",
	versionRelative: "cpguard/app/setup/swversion",
}

func profileArchives(client *network.Client) updater.Archives {
	return client
}

func profileInstaller(runner installer.Runner) func(context.Context, string) error {
	return runner.Run
}

func openVersionFile() (*versionfile.File, error) {
	allowedUID, err := cpguardUID()
	if err != nil {
		return nil, err
	}
	return versionfile.Open(buildProfile.versionAnchor, buildProfile.versionRelative, allowedUID)
}

func openInstallLog() (*versionfile.Appender, error) {
	allowedUID, err := cpguardUID()
	if err != nil {
		return nil, err
	}
	return versionfile.OpenAppend("/opt", "cpguard/logs/install_log", allowedUID)
}

func cpguardUID() (int, error) {
	account, err := user.Lookup("cpguard")
	if err != nil {
		var unknown user.UnknownUserError
		if errors.As(err, &unknown) {
			return 0, nil
		}
		return 0, fmt.Errorf("look up cpguard file owner: %w", err)
	}
	allowedUID, err := strconv.Atoi(account.Uid)
	if err != nil || allowedUID < 0 {
		return 0, errors.New("cpguard account has an invalid numeric UID")
	}
	return allowedUID, nil
}

const maxResultReasonBytes = 2048

type resultLog interface {
	Append([]byte) error
	Close() error
}

func finishResultLog(output resultLog, outcome string, failure error, now time.Time) error {
	status := ""
	if failure == nil {
		switch outcome {
		case "available":
			status = "update available"
		case "up_to_date":
			status = "not available"
		case "installed":
			status = "success"
		default:
			failure = fmt.Errorf("unknown updater result: %q", outcome)
		}
	}
	if failure != nil {
		if outcome == "installed" {
			failure = fmt.Errorf("update already installed; %w", failure)
		}
		status = "failed: " + resultReason(failure.Error())
	}
	if output == nil {
		return errors.Join(failure, resultLogFailure(outcome, "result log is not available", nil))
	}
	entry := now.UTC().Format(time.RFC3339) + " sw-updater " + status + "\n"
	appendErr := output.Append([]byte(entry))
	closeErr := output.Close()
	if appendErr != nil {
		appendErr = resultLogFailure(outcome, "cannot append updater result to install_log", appendErr)
	}
	if closeErr != nil {
		closeErr = resultLogFailure(outcome, "cannot close updater install_log", closeErr)
	}
	return errors.Join(failure, appendErr, closeErr)
}

func resultLogFailure(outcome, message string, failure error) error {
	if outcome == "installed" {
		message = "update already installed; " + message
	}
	if failure == nil {
		return errors.New(message)
	}
	return fmt.Errorf("%s: %w", message, failure)
}

func resultReason(reason string) string {
	reason = strings.ToValidUTF8(reason, "\uFFFD")
	reason = strings.Map(func(character rune) rune {
		if unicode.IsSpace(character) || !unicode.IsPrint(character) {
			return ' '
		}
		return character
	}, reason)
	reason = strings.Join(strings.Fields(reason), " ")
	if reason == "" {
		return "unspecified error"
	}
	if len(reason) <= maxResultReasonBytes {
		return reason
	}
	reason = reason[:maxResultReasonBytes-3]
	for !utf8.ValidString(reason) {
		reason = reason[:len(reason)-1]
	}
	return strings.TrimSpace(reason) + "..."
}
