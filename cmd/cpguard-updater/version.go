package main

import (
	"errors"
	"fmt"
	"os/user"
	"strconv"

	"cpguard/updater/internal/versionfile"
)

func openVersionFile() (*versionfile.File, error) {
	allowedUID := 0
	if buildProfile == "production" {
		account, err := user.Lookup("cpguard")
		if err != nil {
			var unknown user.UnknownUserError
			if !errors.As(err, &unknown) {
				return nil, fmt.Errorf("look up cpguard version-file owner: %w", err)
			}
		} else {
			allowedUID, err = strconv.Atoi(account.Uid)
			if err != nil || allowedUID < 0 {
				return nil, errors.New("cpguard account has an invalid numeric UID")
			}
		}
	}
	return versionfile.Open(versionAnchor, versionRelative, allowedUID)
}
