package manifest

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, 11, 5, 6, 0, 0, 0, time.UTC)

const validJSON = `{"product":"cpguard","version":"5.89.00","sequence":1042,"issued_at":"2026-11-05T06:00:00Z","expires_at":"2026-11-12T06:00:00Z","file":{"url":"https://cdn.test/cpguard.tar.gz","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size":123}}`

func envelope(t *testing.T, payload []byte, private ed25519.PrivateKey) []byte {
	t.Helper()
	encoded, err := json.Marshal(Envelope{Payload: base64.StdEncoding.EncodeToString(payload), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(private, payload))})
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestAcceptKeyAOnly(t *testing.T) {
	publicA, privateA, _ := ed25519.GenerateKey(rand.Reader)
	verified, err := Verify(envelope(t, []byte(validJSON), privateA), publicA, testNow, HTTPS)
	if err != nil || verified.Payload.Sequence != 1042 {
		t.Fatal(verified, err)
	}
	_, outsider, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := Verify(envelope(t, []byte(validJSON), outsider), publicA, testNow, HTTPS); err == nil {
		t.Fatal("accepted unknown signer")
	}
}

func TestRejectChangedPayloadAndEnvelope(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	data := envelope(t, []byte(validJSON), private)
	var original Envelope
	if err := json.Unmarshal(data, &original); err != nil {
		t.Fatal(err)
	}
	changed := original
	changed.Payload = base64.StdEncoding.EncodeToString([]byte(strings.Replace(validJSON, "5.89.00", "5.89.01", 1)))
	tampered, _ := json.Marshal(changed)
	for _, invalid := range [][]byte{tampered, []byte(`{"payload":"!","signature":"!"}`), []byte(`{"payload":"e30=","signature":"AA=="}`), []byte(`{"payload":"e30=","signature":"AA==","extra":1}`), []byte(`{"payload":"e30=","payload":"e30=","signature":"AA=="}`), []byte(strings.Repeat(" ", MaxBytes+1))} {
		if _, err := Verify(invalid, public, testNow, HTTPS); err == nil {
			t.Fatal("accepted invalid signed metadata")
		}
	}
}

func TestEachPayloadRule(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	cases := map[string]string{
		"product":           strings.Replace(validJSON, `"cpguard"`, `"other"`, 1),
		"version":           strings.Replace(validJSON, `"5.89.00"`, `"5.100.00"`, 1),
		"zero-sequence":     strings.Replace(validJSON, `1042`, `0`, 1),
		"negative-sequence": strings.Replace(validJSON, `1042`, `-1`, 1),
		"fraction-sequence": strings.Replace(validJSON, `1042`, `1.5`, 1),
		"overflow-sequence": strings.Replace(validJSON, `1042`, `18446744073709551616`, 1),
		"expired":           strings.Replace(validJSON, `2026-11-12T06:00:00Z`, `2026-11-05T05:59:59Z`, 1),
		"future":            strings.Replace(validJSON, `2026-11-05T06:00:00Z`, `2026-11-05T06:05:01Z`, 1),
		"validity":          strings.Replace(validJSON, `2026-11-12T06:00:00Z`, `2026-11-12T06:00:01Z`, 1),
		"offset":            strings.Replace(validJSON, `2026-11-05T06:00:00Z`, `2026-11-05T06:00:00+00:00`, 1),
		"fraction-time":     strings.Replace(validJSON, `2026-11-05T06:00:00Z`, `2026-11-05T06:00:00.1Z`, 1),
		"http":              strings.Replace(validJSON, `https://`, `http://`, 1),
		"credentials":       strings.Replace(validJSON, `cdn.test/`, `user@cdn.test/`, 1),
		"digest":            strings.Replace(validJSON, strings.Repeat("a", 64), strings.Repeat("A", 64), 1),
		"zero-size":         strings.Replace(validJSON, `"size":123`, `"size":0`, 1),
		"large-size":        strings.Replace(validJSON, `"size":123`, `"size":1073741825`, 1),
		"missing":           strings.Replace(validJSON, `"product":"cpguard",`, "", 1),
		"unknown":           strings.Replace(validJSON, `"product":`, `"unexpected":1,"product":`, 1),
		"duplicate":         strings.Replace(validJSON, `"product":`, `"product":"cpguard","product":`, 1),
		"null":              strings.Replace(validJSON, `"version":"5.89.00"`, `"version":null`, 1),
		"casing":            strings.Replace(validJSON, `"version":`, `"Version":`, 1),
		"nested-missing":    strings.Replace(validJSON, `,"size":123`, "", 1),
		"nested-unknown":    strings.Replace(validJSON, `"size":123`, `"size":123,"other":true`, 1),
		"nested-duplicate":  strings.Replace(validJSON, `"size":123`, `"size":123,"size":123`, 1),
		"trailing":          validJSON + ` {}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Verify(envelope(t, []byte(payload), private), public, testNow, HTTPS); err == nil {
				t.Fatal("accepted invalid payload")
			}
		})
	}
	if _, err := Verify(envelope(t, []byte(validJSON), private), public, testNow.Add(7*24*time.Hour), HTTPS); err == nil {
		t.Fatal("accepted metadata at expiry")
	}
}
