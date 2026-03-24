package tls_test

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"

	appTLS "go-microservice/internal/tls"
)

func TestIsProd(t *testing.T) {
	tests := []struct {
		env  string
		want bool
	}{
		{"production", true},
		{"prod", true},
		{"development", false},
		{"test", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			cfg := &appTLS.Config{Env: tt.env}
			if got := cfg.IsProd(); got != tt.want {
				t.Errorf("IsProd(%q): got %v, want %v", tt.env, got, tt.want)
			}
		})
	}
}

func TestMustGetTLSConfig_DevAutoGeneratesCert(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")

	cfg := &appTLS.Config{
		Env:      "development",
		CertFile: certFile,
		KeyFile:  keyFile,
	}

	tlsCfg := appTLS.MustGetTLSConfig(cfg)
	if tlsCfg == nil {
		t.Fatal("MustGetTLSConfig: expected non-nil tls.Config")
	}

	// Cert and key files should have been created
	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		t.Error("expected cert file to be created")
	}
	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		t.Error("expected key file to be created")
	}

	// TLS config should have at least one certificate
	if len(tlsCfg.Certificates) == 0 {
		t.Error("expected at least one certificate in tls.Config")
	}
}

func TestMustGetTLSConfig_DevLoadsExistingCert(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")

	// First call generates the cert
	appTLS.MustGetTLSConfig(&appTLS.Config{
		Env:      "development",
		CertFile: certFile,
		KeyFile:  keyFile,
	})

	// Get modification times
	certInfo1, _ := os.Stat(certFile)

	// Second call should load existing cert, not regenerate
	appTLS.MustGetTLSConfig(&appTLS.Config{
		Env:      "development",
		CertFile: certFile,
		KeyFile:  keyFile,
	})

	certInfo2, _ := os.Stat(certFile)

	if !certInfo1.ModTime().Equal(certInfo2.ModTime()) {
		t.Error("cert file was regenerated on second call — should reuse existing")
	}
}

func TestMustGetTLSConfig_DevMinTLSVersion(t *testing.T) {
	dir := t.TempDir()
	cfg := &appTLS.Config{
		Env:      "development",
		CertFile: filepath.Join(dir, "cert.pem"),
		KeyFile:  filepath.Join(dir, "key.pem"),
	}

	tlsCfg := appTLS.MustGetTLSConfig(cfg)

	if tlsCfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion: got %d, want %d (TLS 1.2)", tlsCfg.MinVersion, tls.VersionTLS12)
	}
}

func TestMustGetTLSConfig_ProdMinTLSVersionAndCiphers(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")

	// Generate a dev cert to use as the prod cert for this test
	appTLS.MustGetTLSConfig(&appTLS.Config{
		Env:      "development",
		CertFile: certFile,
		KeyFile:  keyFile,
	})

	tlsCfg := appTLS.MustGetTLSConfig(&appTLS.Config{
		Env:      "production",
		CertFile: certFile,
		KeyFile:  keyFile,
	})

	if tlsCfg == nil {
		t.Fatal("MustGetTLSConfig: expected non-nil tls.Config for production")
	}
	if tlsCfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion: got %d, want %d (TLS 1.2)", tlsCfg.MinVersion, tls.VersionTLS12)
	}
	if len(tlsCfg.CipherSuites) == 0 {
		t.Error("expected cipher suites to be set in production TLS config")
	}
	if len(tlsCfg.Certificates) == 0 {
		t.Error("expected at least one certificate in production tls.Config")
	}
}
