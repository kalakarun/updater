package network

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"cpguard/updater/internal/strictjson"
)

const MetadataLimit int64 = 262144

type HTTPError struct {
	StatusCode int
	URL        string
}

func (failure *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d downloading %s", failure.StatusCode, failure.URL)
}

type Client struct {
	client  *http.Client
	hosts   map[string]bool
	Context context.Context
}

func New(ctx context.Context, hosts []string) (*Client, error) {
	allowed := map[string]bool{}
	for _, host := range hosts {
		if host == "" || host != strings.ToLower(host) || strings.ContainsAny(host, "/:@?#\\") {
			return nil, errors.New("invalid allowed hostname")
		}
		allowed[host] = true
	}
	if len(allowed) == 0 {
		return nil, errors.New("no allowed hosts configured")
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		DisableCompression:    true,
	}
	return &Client{client: &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirects forbidden") }}, hosts: allowed, Context: ctx}, nil
}

func (client *Client) ValidateURL(text string) error {
	parsed, err := url.Parse(text)
	if err != nil || parsed.Scheme != "https" || parsed.Opaque != "" || parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" || !client.hosts[parsed.Hostname()] || (parsed.Port() != "" && parsed.Port() != "443") {
		return errors.New("URL must use HTTPS and an exact permitted host on port 443 without credentials, query, or fragment")
	}
	return nil
}

func (client *Client) request(ctx context.Context, address string, limit int64, output io.Writer) error {
	if err := client.ValidateURL(address); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return err
	}
	response, err := client.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return &HTTPError{StatusCode: response.StatusCode, URL: address}
	}
	if response.ContentLength > limit || response.Header.Get("Content-Encoding") != "" {
		return errors.New("oversized or content-encoded response")
	}
	written, err := io.Copy(output, io.LimitReader(response.Body, limit+1))
	if err != nil {
		return err
	}
	if written > limit {
		return errors.New("download exceeds size limit")
	}
	return nil
}

func (client *Client) DownloadFile(address string, maxLength int64) ([]byte, error) {
	if maxLength <= 0 {
		return nil, errors.New("invalid metadata size limit")
	}
	if maxLength > MetadataLimit {
		maxLength = MetadataLimit
	}
	ctx, cancel := context.WithTimeout(client.Context, 60*time.Second)
	defer cancel()
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		var buffer bytes.Buffer
		last = client.request(ctx, address, maxLength, &buffer)
		if last == nil {
			data := buffer.Bytes()
			if err := strictjson.Validate(data, int(maxLength)); err != nil {
				return nil, err
			}
			return data, nil
		}
		var status *HTTPError
		var transportError net.Error
		retry := errors.As(last, &transportError) || (errors.As(last, &status) && (status.StatusCode == 429 || status.StatusCode >= 500))
		if !retry || attempt == 2 {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * time.Second):
		}
	}
	return nil, last
}

func (client *Client) Archive(address, destination string, length int64, digest []byte) (result error) {
	if length <= 0 || length > 1073741824 || len(digest) != sha256.Size {
		return errors.New("invalid authenticated archive length or digest")
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() {
		closeErr := file.Close()
		if result == nil {
			result = closeErr
		}
		if result != nil {
			_ = os.Remove(destination)
		}
	}()
	ctx, cancel := context.WithTimeout(client.Context, 15*time.Minute)
	defer cancel()
	hasher := sha256.New()
	if err := client.request(ctx, address, length, io.MultiWriter(file, hasher)); err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() != length || !bytes.Equal(hasher.Sum(nil), digest) {
		return fmt.Errorf("archive length or SHA-256 mismatch (expected %s)", hex.EncodeToString(digest))
	}
	return file.Sync()
}
