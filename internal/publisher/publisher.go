package publisher

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"time"

	"cpguard/updater/internal/durable"
	"cpguard/updater/internal/manifest"
	"cpguard/updater/internal/release"
	"filippo.io/age"
)

func Generate(passphrase string) ([]byte, ed25519.PublicKey, error) {
	if len(passphrase) < 16 {
		return nil, nil, errors.New("use a unique passphrase of at least 16 bytes")
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	defer clear(private)
	encoded, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		return nil, nil, err
	}
	defer clear(encoded)
	recipient, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		return nil, nil, err
	}
	var output bytes.Buffer
	writer, err := age.Encrypt(&output, recipient)
	if err != nil {
		return nil, nil, err
	}
	if _, err := writer.Write(encoded); err != nil {
		return nil, nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, nil, err
	}
	return output.Bytes(), public, nil
}

func Decrypt(encrypted []byte, passphrase string) (ed25519.PrivateKey, error) {
	if len(encrypted) > 65536 {
		return nil, errors.New("encrypted key exceeds limit")
	}
	identity, err := age.NewScryptIdentity(passphrase)
	if err != nil {
		return nil, err
	}
	reader, err := age.Decrypt(bytes.NewReader(encrypted), identity)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(reader, 4097))
	defer clear(data)
	if err != nil {
		return nil, err
	}
	if len(data) > 4096 {
		return nil, errors.New("decrypted key exceeds limit")
	}
	private, err := x509.ParsePKCS8PrivateKey(data)
	if err != nil {
		return nil, err
	}
	key, ok := private.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("expected Ed25519 private key")
	}
	return key, nil
}

func Make(archivePath, address, version string, sequence uint64, now time.Time, validity time.Duration) (manifest.Payload, error) {
	payload := manifest.Payload{Product: "cpguard", Version: version, Sequence: sequence, IssuedAt: now.UTC().Format(manifest.TimeLayout), ExpiresAt: now.Add(validity).UTC().Format(manifest.TimeLayout), File: manifest.File{URL: address}}
	initial, err := os.Lstat(archivePath)
	if err != nil {
		return payload, err
	}
	if !initial.Mode().IsRegular() || initial.Size() <= 0 || initial.Size() > release.MaxArchiveBytes {
		return payload, errors.New("archive must be a nonempty regular file within size limits")
	}
	file, err := durable.OpenRegular(archivePath)
	if err != nil {
		return payload, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return payload, err
	}
	if !os.SameFile(initial, opened) || initial.Size() != opened.Size() || !initial.ModTime().Equal(opened.ModTime()) {
		return payload, errors.New("archive changed before hashing")
	}
	hasher := sha256.New()
	length, err := io.Copy(hasher, io.LimitReader(file, release.MaxArchiveBytes+1))
	if err != nil {
		return payload, err
	}
	current, err := file.Stat()
	if err != nil {
		return payload, err
	}
	pathInfo, err := os.Lstat(archivePath)
	if err != nil {
		return payload, err
	}
	if length != initial.Size() || !os.SameFile(initial, current) || !os.SameFile(initial, pathInfo) || !pathInfo.Mode().IsRegular() || initial.Size() != current.Size() || initial.Size() != pathInfo.Size() || !initial.ModTime().Equal(current.ModTime()) || !initial.ModTime().Equal(pathInfo.ModTime()) {
		return payload, errors.New("archive changed while hashing")
	}
	payload.File.Size = length
	payload.File.SHA256 = hex.EncodeToString(hasher.Sum(nil))
	return payload, nil
}

func Renew(previous []byte, private ed25519.PrivateKey, sequence uint64, now time.Time, validity time.Duration) ([]byte, error) {
	if len(private) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid signing key")
	}
	verified, err := manifest.Verify(previous, private.Public().(ed25519.PublicKey), now, manifest.HTTPS)
	if err != nil {
		return nil, err
	}
	if sequence <= verified.Payload.Sequence {
		return nil, errors.New("renewal sequence must increase")
	}
	payload := verified.Payload
	payload.Sequence = sequence
	payload.IssuedAt = now.UTC().Format(manifest.TimeLayout)
	payload.ExpiresAt = now.Add(validity).UTC().Format(manifest.TimeLayout)
	return manifest.Sign(payload, private, now)
}

func WriteNew(name string, data []byte) error {
	return durable.WriteNew(name, data, 0600)
}
