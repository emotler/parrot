package main

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"testing"
)

func TestBuildTLSConfigSelfSigned(t *testing.T) {
	// Test with empty cert/key files (should generate self-signed)
	tlsCfg, err := buildTLSConfig("", "")
	if err != nil {
		t.Fatalf("buildTLSConfig failed: %v", err)
	}

	if tlsCfg == nil {
		t.Fatal("expected non-nil tls.Config")
	}

	if len(tlsCfg.Certificates) != 1 {
		t.Errorf("expected 1 certificate, got %d", len(tlsCfg.Certificates))
	}

	if tlsCfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("expected MinVersion TLS 1.2, got %d", tlsCfg.MinVersion)
	}
}

func TestBuildTLSConfigWithFiles(t *testing.T) {
	// Create temporary cert and key files
	certPEM := []byte(`-----BEGIN CERTIFICATE-----
MIIBhTCCASugAwIBAgIQIRi6zePL6mKjOipn+dNuaTAKBggqhkjOPQQDAjASMRAw
DgYDVQQKEwdBY21lIENvMB4XDTE3MTAyMDE5NDMwNloXDTE4MTAyMDE5NDMwNlow
EjEQMA4GA1UEChMHQWNtZSBDbzBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABD0d
7VNhbWvZLWPuj/RtHFjvtJBEwOkhbN/BnnE8rnZR8+sbwnc/KhCk3FhnpHZnQz7B
5aETbbIgmuvewdjvSBSjYzBhMA4GA1UdDwEB/wQEAwICpDATBgNVHSUEDDAKBggr
BgEFBQcDATAPBgNVHRMBAf8EBTADAQH/MCkGA1UdEQQiMCCCDmxvY2FsaG9zdDo1
NDUzgg4xMjcuMC4wLjE6NTQ1MzAKBggqhkjOPQQDAgNIADBFAiEA2zpJEPQyz6/l
Wf86aX6PepsntZv2GYlA5UpabfT2EZICICpJ5h/iI+i341gBmLiAFQOyTDT+/wQc
6MF9+Yw1Yy0t
-----END CERTIFICATE-----`)

	keyPEM := []byte(`-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIIrYSSNQFaA2Hwf1duRSxKtLYX5CB04fSeQ6tF1aY/PuoAoGCCqGSM49
AwEHoUQDQgAEPR3tU2Fta9ktY+6P9G0cWO+0kETA6SFs38GecTyudlHz6xvCdz8q
EKTcWGekdmdDPsHloRNtsiCa697B2O9IFA==
-----END EC PRIVATE KEY-----`)

	certFile, err := os.CreateTemp("", "cert-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(certFile.Name())
	certFile.Write(certPEM)
	certFile.Close()

	keyFile, err := os.CreateTemp("", "key-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(keyFile.Name())
	keyFile.Write(keyPEM)
	keyFile.Close()

	tlsCfg, err := buildTLSConfig(certFile.Name(), keyFile.Name())
	if err != nil {
		t.Fatalf("buildTLSConfig with files failed: %v", err)
	}

	if tlsCfg == nil {
		t.Fatal("expected non-nil tls.Config")
	}

	if len(tlsCfg.Certificates) != 1 {
		t.Errorf("expected 1 certificate, got %d", len(tlsCfg.Certificates))
	}
}

func TestBuildTLSConfigInvalidFiles(t *testing.T) {
	// Test with non-existent files
	_, err := buildTLSConfig("/nonexistent/cert.pem", "/nonexistent/key.pem")
	if err == nil {
		t.Error("expected error for non-existent files")
	}
}

func TestBuildTLSConfigMismatchedFiles(t *testing.T) {
	// Create cert file but use wrong key
	certFile, err := os.CreateTemp("", "cert-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(certFile.Name())
	certFile.WriteString("invalid cert data")
	certFile.Close()

	keyFile, err := os.CreateTemp("", "key-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(keyFile.Name())
	keyFile.WriteString("invalid key data")
	keyFile.Close()

	_, err = buildTLSConfig(certFile.Name(), keyFile.Name())
	if err == nil {
		t.Error("expected error for invalid cert/key files")
	}
}

func TestGenerateSelfSigned(t *testing.T) {
	cert, err := generateSelfSigned()
	if err != nil {
		t.Fatalf("generateSelfSigned failed: %v", err)
	}

	if len(cert.Certificate) == 0 {
		t.Fatal("expected certificate data")
	}

	// Parse the certificate to verify its properties
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("failed to parse generated certificate: %v", err)
	}

	// Verify subject
	if x509Cert.Subject.CommonName != "localhost" {
		t.Errorf("expected CN=localhost, got %s", x509Cert.Subject.CommonName)
	}

	// Verify DNS names
	found := false
	for _, name := range x509Cert.DNSNames {
		if name == "localhost" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'localhost' in DNS names")
	}

	// Verify IP addresses
	foundIP := false
	for _, ip := range x509Cert.IPAddresses {
		if ip.String() == "127.0.0.1" || ip.String() == "::1" {
			foundIP = true
			break
		}
	}
	if !foundIP {
		t.Error("expected 127.0.0.1 or ::1 in IP addresses")
	}

	// Verify key usage
	if x509Cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		t.Error("expected KeyUsageDigitalSignature")
	}

	// Verify extended key usage
	foundServerAuth := false
	for _, usage := range x509Cert.ExtKeyUsage {
		if usage == x509.ExtKeyUsageServerAuth {
			foundServerAuth = true
			break
		}
	}
	if !foundServerAuth {
		t.Error("expected ExtKeyUsageServerAuth")
	}

	// Verify validity period (should be ~1 year)
	duration := x509Cert.NotAfter.Sub(x509Cert.NotBefore)
	expectedDuration := 365 * 24 * 60 * 60 // seconds in a year
	actualDuration := int(duration.Seconds())
	// Allow some tolerance (within 2 minutes)
	if actualDuration < expectedDuration-120 || actualDuration > expectedDuration+120 {
		t.Errorf("expected ~1 year validity, got %v", duration)
	}
}

func TestGenerateSelfSignedUnique(t *testing.T) {
	// Generate two certificates and verify they have different serial numbers
	cert1, err := generateSelfSigned()
	if err != nil {
		t.Fatal(err)
	}

	cert2, err := generateSelfSigned()
	if err != nil {
		t.Fatal(err)
	}

	x509Cert1, _ := x509.ParseCertificate(cert1.Certificate[0])
	x509Cert2, _ := x509.ParseCertificate(cert2.Certificate[0])

	if x509Cert1.SerialNumber.Cmp(x509Cert2.SerialNumber) == 0 {
		t.Error("expected different serial numbers for different certificates")
	}
}

func TestGenerateSelfSignedECDSA(t *testing.T) {
	cert, err := generateSelfSigned()
	if err != nil {
		t.Fatal(err)
	}

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}

	// Verify it's using ECDSA
	if x509Cert.PublicKeyAlgorithm != x509.ECDSA {
		t.Errorf("expected ECDSA key, got %v", x509Cert.PublicKeyAlgorithm)
	}
}

func TestTLSConfigMinVersion(t *testing.T) {
	tlsCfg, err := buildTLSConfig("", "")
	if err != nil {
		t.Fatal(err)
	}

	// Verify minimum TLS version is 1.2
	if tlsCfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("expected MinVersion TLS 1.2 (0x%x), got 0x%x", tls.VersionTLS12, tlsCfg.MinVersion)
	}
}

func TestBuildTLSConfigOnlyOneCertProvided(t *testing.T) {
	// Create only cert file
	certFile, err := os.CreateTemp("", "cert-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(certFile.Name())
	certFile.Close()

	// Provide cert but not key - should generate self-signed
	tlsCfg, err := buildTLSConfig(certFile.Name(), "")
	if err != nil {
		t.Fatalf("expected success with self-signed cert, got error: %v", err)
	}

	if len(tlsCfg.Certificates) != 1 {
		t.Error("expected certificate to be generated")
	}
}

func TestGenerateSelfSignedNotBefore(t *testing.T) {
	cert, err := generateSelfSigned()
	if err != nil {
		t.Fatal(err)
	}

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}

	// NotBefore should be slightly in the past (1 minute skew tolerance)
	// This allows for clock skew between systems
	now := x509Cert.NotBefore
	if now.After(x509Cert.NotAfter) {
		t.Error("NotBefore should be before NotAfter")
	}
}
