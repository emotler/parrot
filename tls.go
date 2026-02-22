package main

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
	"net/http"
	"time"
)

// TLSConfig holds TLS-specific settings.
type TLSConfig struct {
	Enabled   bool
	TLSOffset int    // added to each HTTP port to get the HTTPS port
	CertFile  string // path to PEM cert, empty = auto-generate
	KeyFile   string // path to PEM key,  empty = auto-generate
}

// buildTLSConfig returns a *tls.Config ready to use.
// If certFile/keyFile are empty a self-signed cert is generated in memory.
func buildTLSConfig(certFile, keyFile string) (*tls.Config, error) {
	var cert tls.Certificate
	var err error

	if certFile != "" && keyFile != "" {
		cert, err = tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("loading TLS key pair: %w", err)
		}
		log.Println("[parrot:tls] using provided certificate")
	} else {
		cert, err = generateSelfSigned()
		if err != nil {
			return nil, fmt.Errorf("generating self-signed cert: %w", err)
		}
		log.Println("[parrot:tls] generated self-signed certificate (valid 1 year)")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// generateSelfSigned creates an in-memory ECDSA P-256 self-signed certificate
// valid for localhost and 127.0.0.1.
func generateSelfSigned() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"parrot (self-signed)"},
			CommonName:   "localhost",
		},
		DNSNames:    []string{"localhost"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		NotBefore:   time.Now().Add(-time.Minute), // 1 min skew tolerance
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return tls.X509KeyPair(certPEM, keyPEM)
}

// startTLSParrot starts the HTTPS companion for a given HTTP port.
func startTLSParrot(httpPort int, tlsPort int, tlsCfg *tls.Config, store *Store, cfg Config, knownPorts []int) {
	mux := buildMux(httpPort, tlsPort, true, store, cfg, knownPorts)

	addr := fmt.Sprintf(":%d", tlsPort)
	log.Printf("[parrot:%d] squawking on https://localhost%s", httpPort, addr)

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		TLSConfig:    tlsCfg,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
		log.Printf("[parrot:%d tls] error: %v", httpPort, err)
	}
}
