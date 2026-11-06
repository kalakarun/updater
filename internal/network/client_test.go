package network

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (function transportFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func testServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), DNSNames: []string{"updates.test"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &private.PublicKey, private)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: private}}}
	server.StartTLS()
	return server
}

func localClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := testServer(t, handler)
	t.Cleanup(server.Close)
	client, err := New(context.Background(), []string{"updates.test"})
	if err != nil {
		t.Fatal(err)
	}
	endpoint, _ := url.Parse(server.URL)
	base := server.Client().Transport
	client.client.Transport = transportFunc(func(request *http.Request) (*http.Response, error) {
		copyRequest := request.Clone(request.Context())
		copyURL := *copyRequest.URL
		copyURL.Host = endpoint.Host
		copyRequest.URL = &copyURL
		return base.RoundTrip(copyRequest)
	})
	return client
}

func TestURLPolicy(t *testing.T) {
	client, err := New(context.Background(), []string{"updates.test"})
	if err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{"http://updates.test/a", "https://updates.test.evil/a", "https://evilupdates.test/a", "https://user@updates.test/a", "https://updates.test:444/a", "https://updates.test/a#fragment", "https://updates.test/a?token=x", "file:///tmp/a", "https://UPDATES.test/a"} {
		if err := client.ValidateURL(address); err == nil {
			t.Errorf("accepted %s", address)
		}
	}
	if err := client.ValidateURL("https://updates.test:443/a"); err != nil {
		t.Fatal(err)
	}
}

func TestMetadataNetworkLimits(t *testing.T) {
	for _, scenario := range []string{"redirect", "oversized", "duplicate", "trailing", "status", "encoded"} {
		t.Run(scenario, func(t *testing.T) {
			client := localClient(t, func(writer http.ResponseWriter, request *http.Request) {
				switch scenario {
				case "redirect":
					http.Redirect(writer, request, "https://updates.test/other", http.StatusFound)
				case "oversized":
					_, _ = io.WriteString(writer, strings.Repeat("a", 100))
				case "duplicate":
					_, _ = io.WriteString(writer, `{"a":1,"a":2}`)
				case "trailing":
					_, _ = io.WriteString(writer, `{} {}`)
				case "status":
					writer.WriteHeader(http.StatusForbidden)
				case "encoded":
					writer.Header().Set("Content-Encoding", "gzip")
					_, _ = io.WriteString(writer, `{}`)
				}
			})
			if _, err := client.DownloadFile("https://updates.test/meta", 32); err == nil {
				t.Fatal("accepted response")
			}
		})
	}
}

func TestArchiveVerificationAndCleanup(t *testing.T) {
	for _, scenario := range []string{"valid", "hash", "size", "overflow"} {
		t.Run(scenario, func(t *testing.T) {
			client := localClient(t, func(writer http.ResponseWriter, request *http.Request) { _, _ = io.WriteString(writer, "archive") })
			digest := sha256.Sum256([]byte("archive"))
			length := int64(7)
			switch scenario {
			case "hash":
				digest[0] ^= 1
			case "size":
				length = 8
			case "overflow":
				length = 6
			}
			file := filepath.Join(t.TempDir(), "archive")
			err := client.Archive("https://updates.test/artifact", file, length, digest[:])
			if scenario == "valid" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatal("accepted invalid archive")
			}
			if _, err := os.Stat(file); !os.IsNotExist(err) {
				t.Fatal("failed download retained")
			}
		})
	}
}

func TestTLSAndCancellation(t *testing.T) {
	server := testServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { _, _ = io.WriteString(writer, `{}`) }))
	defer server.Close()
	client, _ := New(context.Background(), []string{"updates.test"})
	endpoint, _ := url.Parse(server.URL)
	base := client.client.Transport
	client.client.Transport = transportFunc(func(request *http.Request) (*http.Response, error) {
		request.URL.Host = endpoint.Host
		return base.RoundTrip(request)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	client.Context = ctx
	if _, err := client.DownloadFile("https://updates.test/a", 100); err == nil {
		t.Fatal("accepted untrusted TLS certificate")
	}
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	client.Context = cancelled
	if _, err := client.DownloadFile("https://updates.test/a", 100); err == nil {
		t.Fatal("ignored cancellation")
	}
}
