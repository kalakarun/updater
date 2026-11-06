package publisher

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cpguard/updater/internal/manifest"
)

func TestEncryptedKeyRoundTrip(t *testing.T) {
	encrypted, public, err := Generate("test-only strong passphrase")
	if err != nil {
		t.Fatal(err)
	}
	private, err := Decrypt(encrypted, "test-only strong passphrase")
	if err != nil {
		t.Fatal(err)
	}
	defer clear(private)
	if !bytes.Equal(private.Public().(ed25519.PublicKey), public) {
		t.Fatal("key changed")
	}
	if _, err := Decrypt(encrypted, "incorrect passphrase"); err == nil {
		t.Fatal("wrong passphrase accepted")
	}
}

func TestMakeAndRenew(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Now().UTC().Truncate(time.Second)
	name := filepath.Join(t.TempDir(), "release.tar.gz")
	if err := os.WriteFile(name, []byte("test artifact"), 0600); err != nil {
		t.Fatal(err)
	}
	payload, err := Make(name, "https://cdn.test/release.tar.gz", "5.89.00", 1042, now, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := manifest.Sign(payload, private, now)
	if err != nil {
		t.Fatal(err)
	}
	renewed, err := Renew(encoded, private, 1043, now.Add(time.Hour), 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := manifest.Verify(renewed, public, now.Add(time.Hour), manifest.HTTPS)
	if err != nil {
		t.Fatal(err)
	}
	if verified.Payload.File != payload.File || verified.Payload.Version != payload.Version || verified.Payload.Sequence != 1043 {
		t.Fatal("renewal changed release identity")
	}
	if _, err := Renew(encoded, private, 1042, now, 24*time.Hour); err == nil {
		t.Fatal("renewed without increasing sequence")
	}
	if _, err := Renew(encoded, private, 1044, now.Add(25*time.Hour), 24*time.Hour); err == nil {
		t.Fatal("renew accepted expired source")
	}
}
