package proxy

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// loadOrCreateCertificate publishes certificate and key as one private PEM bundle.
// Existing malformed state fails closed; it must never silently change phone pins.
func loadOrCreateCertificate(path string) (tls.Certificate, error) {
	b, err := os.ReadFile(path)
	if err == nil {
		return tls.X509KeyPair(b, b)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return tls.Certificate{}, err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return tls.Certificate{}, err
	}
	pair, err := generateSelfSignedCert()
	if err != nil {
		return tls.Certificate{}, err
	}
	key, err := x509.MarshalPKCS8PrivateKey(pair.PrivateKey)
	if err != nil {
		return tls.Certificate{}, err
	}
	b = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: pair.Certificate[0]})
	b = append(b, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key})...)
	f, err := os.CreateTemp(filepath.Dir(path), ".tls-*")
	if err != nil {
		return tls.Certificate{}, err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return tls.Certificate{}, err
	}
	if closeErr != nil {
		return tls.Certificate{}, closeErr
	}
	// Hard link publishes a complete file without replacing a concurrent creator.
	if err = os.Link(f.Name(), path); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return tls.Certificate{}, fmt.Errorf("publish TLS certificate: %w", err)
		}
		b, err = os.ReadFile(path)
		if err != nil {
			return tls.Certificate{}, err
		}
		return tls.X509KeyPair(b, b)
	}
	return pair, nil
}

func certificateFingerprint(pair tls.Certificate) string {
	h := sha256.Sum256(pair.Certificate[0])
	return hex.EncodeToString(h[:])
}
