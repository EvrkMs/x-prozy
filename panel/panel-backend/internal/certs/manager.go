// Package certs provides auto-generated TLS certificates for gRPC.
// On first start the panel generates a self-signed ECDSA CA and a server cert.
// On subsequent starts it loads existing certs from the data directory.
package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Manager manages auto-generated TLS certificates for the gRPC channel.
type Manager struct {
	dataDir     string
	caCert      *x509.Certificate
	caKey       *ecdsa.PrivateKey
	serverCert  tls.Certificate
	fingerprint string
	log         *slog.Logger
}

// NewManager initialises the TLS subsystem.
// If no CA is found in dataDir it generates one + a server certificate.
func NewManager(dataDir string, log *slog.Logger) (*Manager, error) {
	m := &Manager{
		dataDir: dataDir,
		log:     log,
	}

	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("certs: mkdir %s: %w", dataDir, err)
	}

	caPath := filepath.Join(dataDir, "ca.pem")
	caKeyPath := filepath.Join(dataDir, "ca-key.pem")
	serverPath := filepath.Join(dataDir, "server.pem")
	serverKeyPath := filepath.Join(dataDir, "server-key.pem")

	// ── CA ──────────────────────────────────────────────────
	if !fileExists(caPath) {
		log.Info("generating CA certificate", "dir", dataDir)
		if err := m.generateCA(caPath, caKeyPath); err != nil {
			return nil, fmt.Errorf("certs: generate CA: %w", err)
		}
	}
	if err := m.loadCA(caPath, caKeyPath); err != nil {
		return nil, fmt.Errorf("certs: load CA: %w", err)
	}

	// fingerprint = SHA256 of CA cert DER
	hash := sha256.Sum256(m.caCert.Raw)
	m.fingerprint = hex.EncodeToString(hash[:])

	// ── Server cert ─────────────────────────────────────────
	if !fileExists(serverPath) {
		log.Info("generating server certificate")
		if err := m.generateServerCert(serverPath, serverKeyPath); err != nil {
			return nil, fmt.Errorf("certs: generate server cert: %w", err)
		}
	}
	cert, err := tls.LoadX509KeyPair(serverPath, serverKeyPath)
	if err != nil {
		return nil, fmt.Errorf("certs: load server cert: %w", err)
	}
	// Append CA cert so clients receive the full chain.
	cert.Certificate = append(cert.Certificate, m.caCert.Raw)
	m.serverCert = cert

	log.Info("TLS certificates ready",
		"ca_fingerprint", m.fingerprint,
	)
	return m, nil
}

// Fingerprint returns the hex-encoded SHA256 of the CA certificate DER.
func (m *Manager) Fingerprint() string { return m.fingerprint }

// ServerTLSConfig returns *tls.Config ready for grpc.Creds.
func (m *Manager) ServerTLSConfig() *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{m.serverCert},
		MinVersion:   tls.VersionTLS13,
	}
}

// CACertPEM returns the CA public certificate in PEM format.
func (m *Manager) CACertPEM() []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: m.caCert.Raw,
	})
}

// ── Internal helpers ────────────────────────────────────────────────────────

func (m *Manager) generateCA(certPath, keyPath string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := cryptoRandSerial()
	if err != nil {
		return err
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"x-prozy"},
			CommonName:   "x-prozy CA",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour), // 10 лет
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	if err := writePEM(certPath, "CERTIFICATE", certDER); err != nil {
		return err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	return writePEM(keyPath, "EC PRIVATE KEY", keyDER)
}

func (m *Manager) loadCA(certPath, keyPath string) error {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return err
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return fmt.Errorf("failed to decode CA cert PEM")
	}
	m.caCert, err = x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}

	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return fmt.Errorf("failed to decode CA key PEM")
	}
	m.caKey, err = x509.ParseECPrivateKey(keyBlock.Bytes)
	return err
}

func (m *Manager) generateServerCert(certPath, keyPath string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := cryptoRandSerial()
	if err != nil {
		return err
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"x-prozy"},
			CommonName:   "x-prozy gRPC",
		},
		NotBefore: time.Now().Add(-1 * time.Hour),
		NotAfter:  time.Now().Add(5 * 365 * 24 * time.Hour), // 5 лет
		KeyUsage:  x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		DNSNames: []string{
			"localhost",
			"panel",
			"x-prozy-panel",
		},
		IPAddresses: []net.IP{
			net.ParseIP("127.0.0.1"),
			net.ParseIP("0.0.0.0"),
			net.IPv6loopback,
		},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, m.caCert, &key.PublicKey, m.caKey)
	if err != nil {
		return err
	}
	if err := writePEM(certPath, "CERTIFICATE", certDER); err != nil {
		return err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	return writePEM(keyPath, "EC PRIVATE KEY", keyDER)
}

func cryptoRandSerial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
}

func writePEM(path, blockType string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: blockType, Bytes: data})
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
