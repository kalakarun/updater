package state

import (
	"encoding/json"
	"errors"
	"os"

	"cpguard/updater/internal/durable"
	"cpguard/updater/internal/manifest"
	"cpguard/updater/internal/release"
	"cpguard/updater/internal/strictjson"
)

type State struct {
	Sequence      uint64 `json:"sequence"`
	PayloadSHA256 string `json:"payload_sha256"`
}

func Initialize(name string) error {
	if _, err := os.Lstat(name); !errors.Is(err, os.ErrNotExist) {
		return errors.New("refusing to replace existing or inaccessible sequence state")
	}
	return write(name, State{})
}

func Load(name string) (State, error) {
	var current State
	data, err := durable.Read(name, 4096)
	if err != nil {
		return current, err
	}
	if err := strictjson.Decode(data, &current, "sequence", "payload_sha256"); err != nil {
		return current, err
	}
	if (current.Sequence == 0 && current.PayloadSHA256 != "") || (current.Sequence > 0 && !release.ValidSHA256(current.PayloadSHA256)) {
		return current, errors.New("corrupted sequence state")
	}
	return current, nil
}

func Accept(name string, verified manifest.Verified) error {
	current, err := Load(name)
	if err != nil {
		return err
	}
	if verified.Payload.Sequence == 0 || !release.ValidSHA256(verified.Digest) {
		return errors.New("invalid verified metadata")
	}
	if verified.Payload.Sequence < current.Sequence {
		return errors.New("metadata sequence is lower than the last accepted sequence")
	}
	if verified.Payload.Sequence == current.Sequence {
		if verified.Digest != current.PayloadSHA256 {
			return errors.New("same sequence used for different payloads")
		}
		return nil
	}
	return write(name, State{Sequence: verified.Payload.Sequence, PayloadSHA256: verified.Digest})
}

func write(name string, current State) error {
	data, err := json.Marshal(current)
	if err != nil {
		return err
	}
	return durable.Write(name, data, 0600, -1, -1)
}
