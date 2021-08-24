/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pki

import (
	"crypto"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/keyutil"
)

const (
	rsaKeySize          = 2048
	CertificateValidity = time.Hour * 24 * 365
)

// RootCA for a given config will check existing certs if they are valid, else
// will generate new root CA for the certutil.Config provided
func RootCA(config *certutil.Config) (keyBytes, certBytes []byte, err error) {
	// create private key, defaults to x509.RSA
	key, err := rsa.GenerateKey(cryptorand.Reader, rsaKeySize)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create private key while generating CA certificate, %w", err)
	}
	cert, err := newSelfSignedCACert(config.CommonName, key)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create self-signed CA certificate, %w", err)
	}
	return encodePrivateKey(key), encodeCertificate(cert), nil
}

// GenerateCertAndKey for a given config and valid CA, will check existing certs
// if they are valid, else will generate new cert and key for the
// certutil.Config provided
func GenerateSignedCertAndKey(config *certutil.Config, caCertBytes, caKeyBytes []byte) (keyBytes, certBytes []byte, err error) {
	caKey, caCert, err := parseCerts(caCertBytes, caKeyBytes)
	if err != nil {
		return nil, nil, err
	}
	key, err := rsa.GenerateKey(cryptorand.Reader, rsaKeySize)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create private key while generating CA certificate, %w", err)
	}
	cert, err := signedCert(config, key, caKey, caCert)
	if err != nil {
		return nil, nil, fmt.Errorf("creating signed cert, %w", err)
	}
	return encodePrivateKey(key), encodeCertificate(cert), nil
}

func GenerateKeyPair() (private, public []byte, err error) {
	key, err := rsa.GenerateKey(cryptorand.Reader, rsaKeySize)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to generate key, %w", err)
	}
	der, err := x509.MarshalPKIXPublicKey(key.Public())
	if err != nil {
		return nil, nil, err
	}
	return encodePrivateKey(key), encodePublicKey(der), nil
}

func parseCerts(certBytes, keyBytes []byte) (crypto.Signer, *x509.Certificate, error) {
	cert, err := certutil.ParseCertsPEM(certBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing cert, %w", err)
	}
	key, err := keyutil.ParsePrivateKeyPEM(keyBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing private key, %w", err)
	}
	return key.(crypto.Signer), cert[0], nil
}

func encodeCertificate(cert *x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
}

func encodePrivateKey(key crypto.Signer) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key.(*rsa.PrivateKey)),
	})
}

func encodePublicKey(key []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: key,
	})
}

// signedCert creates a signed certificate using the given CA certificate and key
func signedCert(cfg *certutil.Config, key, caKey crypto.Signer, caCert *x509.Certificate) (*x509.Certificate, error) {
	serial, err := cryptorand.Int(cryptorand.Reader, new(big.Int).SetInt64(math.MaxInt64))
	if err != nil {
		return nil, err
	}
	if len(cfg.CommonName) == 0 {
		return nil, errors.New("commonName is missing")
	}
	keyUsage := x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature
	cfg.AltNames = removeDuplicateAltNames(&cfg.AltNames)
	certTmpl := x509.Certificate{
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: cfg.Organization,
		},
		DNSNames:              cfg.AltNames.DNSNames,
		IPAddresses:           cfg.AltNames.IPs,
		SerialNumber:          serial,
		NotBefore:             caCert.NotBefore,
		NotAfter:              time.Now().Add(CertificateValidity).UTC(),
		KeyUsage:              keyUsage,
		ExtKeyUsage:           cfg.Usages,
		BasicConstraintsValid: true,
		IsCA:                  false,
	}
	certDERBytes, err := x509.CreateCertificate(cryptorand.Reader, &certTmpl, caCert, key.Public(), caKey)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(certDERBytes)
}

// removeDuplicateAltNames removes duplicate items in altNames.
func removeDuplicateAltNames(altNames *certutil.AltNames) certutil.AltNames {
	if altNames == nil {
		return certutil.AltNames{}
	}
	if altNames.DNSNames != nil {
		altNames.DNSNames = sets.NewString(altNames.DNSNames...).List()
	}
	ipsKeys := make(map[string]struct{})
	var ips []net.IP
	for _, one := range altNames.IPs {
		if _, ok := ipsKeys[one.String()]; !ok {
			ipsKeys[one.String()] = struct{}{}
			ips = append(ips, one)
		}
	}
	altNames.IPs = ips
	return *altNames
}

// newSelfSignedCACert creates a certificate authority
func newSelfSignedCACert(commonName string, key crypto.Signer) (*x509.Certificate, error) {
	now := time.Now()
	cert := x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(0),
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:             now.UTC(),
		NotAfter:              now.Add(CertificateValidity * 10).UTC(),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	certDERBytes, err := x509.CreateCertificate(cryptorand.Reader, &cert, &cert, key.Public(), key)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(certDERBytes)
}
