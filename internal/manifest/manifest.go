package manifest

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"time"

	"cpguard/updater/internal/release"
	"cpguard/updater/internal/strictjson"
)

const MaxBytes = 262144
const TimeLayout = "2006-01-02T15:04:05Z"

type Envelope struct {
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

type File struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type Payload struct {
	Product   string `json:"product"`
	Version   string `json:"version"`
	Sequence  uint64 `json:"sequence"`
	IssuedAt  string `json:"issued_at"`
	ExpiresAt string `json:"expires_at"`
	File      File   `json:"file"`
}

type Verified struct {
	Payload Payload
	Digest  string
}

func HTTPS(address string) error {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.Opaque != "" || parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" || (parsed.Port() != "" && parsed.Port() != "443") {
		return errors.New("archive URL must use HTTPS on port 443 without credentials, query, or fragment")
	}
	return nil
}

func Parse(data []byte, now time.Time, validateURL func(string) error) (Payload, error) {
	var payload Payload
	if err := strictjson.Decode(data, &payload, "product", "version", "sequence", "issued_at", "expires_at", "file"); err != nil {
		return payload, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return payload, err
	}
	if err := strictjson.Decode(fields["file"], &payload.File, "url", "sha256", "size"); err != nil {
		return payload, err
	}
	if payload.Product != "cpguard" || payload.Sequence == 0 {
		return payload, errors.New("invalid product or sequence")
	}
	if _, err := release.ParseVersion(payload.Version); err != nil {
		return payload, err
	}
	issued, err := exactTime(payload.IssuedAt)
	if err != nil {
		return payload, err
	}
	expires, err := exactTime(payload.ExpiresAt)
	if err != nil {
		return payload, err
	}
	if !expires.After(issued) || expires.Sub(issued) > 7*24*time.Hour {
		return payload, errors.New("metadata validity must be positive and at most seven days")
	}
	if !now.Before(expires) || issued.After(now.Add(5*time.Minute)) {
		return payload, errors.New("metadata expired or issued more than five minutes in the future")
	}
	if !release.ValidSHA256(payload.File.SHA256) || payload.File.Size <= 0 || payload.File.Size > release.MaxArchiveBytes {
		return payload, errors.New("invalid archive SHA-256 or size")
	}
	if err := HTTPS(payload.File.URL); err != nil {
		return payload, err
	}
	if validateURL == nil {
		return payload, errors.New("archive host validation is required")
	}
	return payload, validateURL(payload.File.URL)
}

func exactTime(text string) (time.Time, error) {
	parsed, err := time.Parse(TimeLayout, text)
	if err != nil || parsed.Format(TimeLayout) != text {
		return time.Time{}, errors.New("timestamps must use exact UTC seconds ending in Z")
	}
	return parsed, nil
}

func Verify(data []byte, keyA ed25519.PublicKey, now time.Time, validateURL func(string) error) (Verified, error) {
	var result Verified
	var envelope Envelope
	if err := strictjson.Decode(data, &envelope, "payload", "signature"); err != nil {
		return result, err
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(envelope.Payload)
	if err != nil {
		return result, errors.New("invalid payload base64")
	}
	signature, err := base64.StdEncoding.Strict().DecodeString(envelope.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return result, errors.New("invalid signature encoding or length")
	}
	if len(keyA) != ed25519.PublicKeySize || !ed25519.Verify(keyA, decoded, signature) {
		return result, errors.New("metadata signature does not match key A")
	}
	payload, err := Parse(decoded, now, validateURL)
	if err != nil {
		return result, err
	}
	digest := sha256.Sum256(decoded)
	return Verified{Payload: payload, Digest: hex.EncodeToString(digest[:])}, nil
}

func Sign(payload Payload, private ed25519.PrivateKey, now time.Time) ([]byte, error) {
	if len(private) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid signing key")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if _, err := Parse(data, now, HTTPS); err != nil {
		return nil, err
	}
	signature := ed25519.Sign(private, data)
	if !ed25519.Verify(private.Public().(ed25519.PublicKey), data, signature) {
		return nil, errors.New("signature self-check failed")
	}
	envelope := Envelope{Payload: base64.StdEncoding.EncodeToString(data), Signature: base64.StdEncoding.EncodeToString(signature)}
	encoded, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(encoded) > MaxBytes {
		return nil, errors.New("metadata exceeds size limit")
	}
	return encoded, nil
}
