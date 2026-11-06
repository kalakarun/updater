package site

import (
	"crypto/ed25519"
	_ "embed"
	"encoding/base64"
	"errors"

	"cpguard/updater/internal/strictjson"
)

//go:embed site.json
var embedded []byte

type Config struct {
	KeyA        string   `json:"key_a"`
	MetadataURL string   `json:"metadata_url"`
	CDNHosts    []string `json:"allowed_cdn_hosts"`
}

func Load() (Config, ed25519.PublicKey, error) { return parse(embedded) }

func parse(data []byte) (Config, ed25519.PublicKey, error) {
	var config Config
	if err := strictjson.Decode(data, &config, "key_a", "metadata_url", "allowed_cdn_hosts"); err != nil {
		return config, nil, err
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(config.KeyA)
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return config, nil, errors.New("public key A has not been provisioned in the build")
	}
	if config.MetadataURL == "" || len(config.CDNHosts) == 0 {
		return config, nil, errors.New("metadata URL and allowed CDN hosts must be provisioned in the build")
	}
	return config, ed25519.PublicKey(decoded), nil
}
