package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"time"
)

// Config holds TLS configuration
type Config struct {
	// Dev/test: paths to self-signed cert and key
	CertFile string
	KeyFile  string

	// Production: Let's Encrypt settings (handled by Nginx, not Go directly)
	// Go only needs cert/key paths — Nginx or certbot writes them here.
	// Set ENV=production to enforce cert presence.
	Env string
}

// IsProd returns true when running in production mode
func (c *Config) IsProd() bool {
	return c.Env == "production"
}

// MustGetTLSConfig returns a *tls.Config ready to use with http.Server.
// In dev/test: auto-generates a self-signed cert if none exists.
// In production: loads the cert/key provided by Let's Encrypt (via Nginx/certbot).
func MustGetTLSConfig(cfg *Config) *tls.Config {
	if cfg.IsProd() {
		return loadProdTLS(cfg)
	}
	return loadDevTLS(cfg)
}

func loadProdTLS(cfg *Config) *tls.Config {
	if cfg.CertFile == "" || cfg.KeyFile == "" {
		log.Fatal("TLS: CERT_FILE and KEY_FILE must be set in production")
	}
	cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		log.Fatalf("TLS: failed to load production cert/key: %v", err)
	}
	log.Printf("TLS: loaded production certificate from %s", cfg.CertFile)
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
	}
}

func loadDevTLS(cfg *Config) *tls.Config {
	certFile := cfg.CertFile
	keyFile := cfg.KeyFile
	if certFile == "" {
		certFile = "certs/dev-cert.pem"
	}
	if keyFile == "" {
		keyFile = "certs/dev-key.pem"
	}

	// Auto-generate if either file is missing
	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		log.Println("TLS: no dev cert found, generating self-signed certificate...")
		if err := generateSelfSigned(certFile, keyFile); err != nil {
			log.Fatalf("TLS: failed to generate self-signed cert: %v", err)
		}
		log.Printf("TLS: self-signed cert written to %s and %s", certFile, keyFile)
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Fatalf("TLS: failed to load dev cert/key: %v", err)
	}
	log.Println("TLS: loaded self-signed development certificate")
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
}

// generateSelfSigned creates a self-signed ECDSA cert valid for 1 year.
func generateSelfSigned(certFile, keyFile string) error {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"go-microservice (dev)"},
			CommonName:   "localhost",
		},
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return fmt.Errorf("create cert: %w", err)
	}

	// Write cert
	certOut, err := os.Create(certFile)
	if err != nil {
		return fmt.Errorf("open cert file: %w", err)
	}
	defer certOut.Close()
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return fmt.Errorf("write cert: %w", err)
	}

	// Write key
	keyOut, err := os.Create(keyFile)
	if err != nil {
		return fmt.Errorf("open key file: %w", err)
	}
	defer keyOut.Close()
	privDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: privDER}); err != nil {
		return fmt.Errorf("write key: %w", err)
	}

	return nil
}
