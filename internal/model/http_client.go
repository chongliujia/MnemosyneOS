package model

import (
	"crypto/tls"
	"net/http"
	"os"
	"strings"
	"time"
)

// tlsInsecureFromEnv is true when MNEMOSYNE_TLS_INSECURE is set to 1/true/yes.
// This disables server certificate verification for HTTPS calls to the model
// API — useful only behind broken MITM proxies or for local debugging. Never
// use in production.
func tlsInsecureFromEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("MNEMOSYNE_TLS_INSECURE")))
	return v == "1" || v == "true" || v == "yes"
}

func newModelHTTPClient() *http.Client {
	return newHTTPClientWithTimeout(45 * time.Second)
}

func newHTTPClientWithTimeout(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Client{Timeout: timeout}
	}
	transport := base.Clone()
	if tlsInsecureFromEnv() {
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{}
		}
		transport.TLSClientConfig.InsecureSkipVerify = true
		if transport.TLSClientConfig.MinVersion == 0 {
			transport.TLSClientConfig.MinVersion = tls.VersionTLS12
		}
	}
	return &http.Client{Timeout: timeout, Transport: transport}
}
