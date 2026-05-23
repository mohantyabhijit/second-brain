package httpclient

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"
	"time"
)

func New() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if certFile := certFile(); certFile != "" {
		if roots, err := rootCAs(certFile); err == nil {
			transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
		}
	}

	return &http.Client{
		Transport: transport,
		Timeout:   120 * time.Second,
	}
}

func certFile() string {
	for _, key := range []string{"SSL_CERT_FILE", "NODE_EXTRA_CA_CERTS", "REQUESTS_CA_BUNDLE", "CURL_CA_BUNDLE"} {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

func rootCAs(certFile string) (*x509.CertPool, error) {
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}

	raw, err := os.ReadFile(certFile)
	if err != nil {
		return nil, err
	}
	roots.AppendCertsFromPEM(raw)
	return roots, nil
}
