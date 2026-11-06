package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cpguard/updater/internal/manifest"
)

func TestSequenceAndRetry(t *testing.T) {
	name := filepath.Join(t.TempDir(), "state.json")
	if err := Initialize(name); err != nil {
		t.Fatal(err)
	}
	verified := manifest.Verified{Payload: manifest.Payload{Sequence: 10}, Digest: strings.Repeat("a", 64)}
	if err := Accept(name, verified); err != nil {
		t.Fatal(err)
	}
	if err := Accept(name, verified); err != nil {
		t.Fatal("retry rejected", err)
	}
	verified.Payload.Sequence = 9
	if err := Accept(name, verified); err == nil {
		t.Fatal("accepted rollback")
	}
	verified.Payload.Sequence = 10
	verified.Digest = strings.Repeat("b", 64)
	if err := Accept(name, verified); err == nil {
		t.Fatal("accepted conflicting equal sequence")
	}
	verified.Payload.Sequence = 11
	if err := Accept(name, verified); err != nil {
		t.Fatal(err)
	}
	if err := Initialize(name); err == nil {
		t.Fatal("reset existing state")
	}
}

func TestMissingAndCorruptedState(t *testing.T) {
	name := filepath.Join(t.TempDir(), "state.json")
	if _, err := Load(name); err == nil {
		t.Fatal("accepted missing state")
	}
	for _, data := range []string{"", `{}`, `{"sequence":1,"payload_sha256":""}`, `{"sequence":0,"payload_sha256":"abc"}`, `{"sequence":-1,"payload_sha256":""}`} {
		if err := os.WriteFile(name, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(name); err == nil {
			t.Fatal("accepted corrupted state", data)
		}
	}
}
