package site

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestDevelopmentConfigurationFailsClosed(t *testing.T) {
	if _, _, err := parse([]byte(`{"key_a":"","metadata_url":"https://metadata.test/update_check.json","allowed_cdn_hosts":[]}`)); err == nil {
		t.Fatal("unprovisioned trust configuration accepted")
	}
}

func TestOnlyKeyAIsConfigured(t *testing.T) {
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	config := map[string]any{"key_a": base64.StdEncoding.EncodeToString(public), "metadata_url": "https://metadata.test/update_check.json", "allowed_cdn_hosts": []string{"cdn.test"}}
	data, _ := json.Marshal(config)
	_, keyA, err := parse(data)
	if err != nil || !public.Equal(keyA) {
		t.Fatal("single-key configuration rejected", err)
	}
	config["key_b"] = base64.StdEncoding.EncodeToString(public)
	data, _ = json.Marshal(config)
	if _, _, err := parse(data); err == nil {
		t.Fatal("removed key_b configuration accepted")
	}
}
