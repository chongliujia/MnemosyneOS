package model

import (
	"crypto/tls"
	"net/http"
	"testing"
)

func TestNewModelHTTPClientRespectsTLSInsecureEnv(t *testing.T) {
	t.Setenv("MNEMOSYNE_TLS_INSECURE", "true")

	c := newModelHTTPClient()
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.Transport)
	}
	if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("expected InsecureSkipVerify when MNEMOSYNE_TLS_INSECURE=true, got %#v", tr.TLSClientConfig)
	}
	if tr.TLSClientConfig.MinVersion < tls.VersionTLS12 {
		t.Fatalf("expected at least TLS 1.2 min version")
	}
}

func TestNewModelHTTPClientSecureByDefault(t *testing.T) {
	t.Setenv("MNEMOSYNE_TLS_INSECURE", "")
	c := newModelHTTPClient()
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.Transport)
	}
	if tr.TLSClientConfig != nil && tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("did not expect InsecureSkipVerify when MNEMOSYNE_TLS_INSECURE is unset")
	}
}
