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

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/sets"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/keyutil"
)

const (
	rsaKeySize          = 2048
	CertificateValidity = time.Hour * 24 * 365
)

type CertConfig struct {
	*certutil.Config
	ExistingCert []byte
	ExistingKey  []byte
}

// RootCA for a given config will check existing certs if they are valid, else
// will generate new root CA for the certutil.Config provided
func RootCA(config *CertConfig) (certBytes, keyBytes []byte, err error) {
	// verify the existing certs if valid use
	cert, key, err := validCerts(config.ExistingCert, config.ExistingKey)
	if err == nil {
		zap.S().Infof("Reusing existing certs for %s", config.CommonName)
	} else {
		// create private key, defaults to x509.RSA
		if key, err = rsa.GenerateKey(cryptorand.Reader, rsaKeySize); err != nil {
			return nil, nil, fmt.Errorf("unable to create private key while generating CA certificate, %w", err)
		}
		if cert, err = newSelfSignedCACert(config.CommonName, key); err != nil {
			return nil, nil, fmt.Errorf("unable to create self-signed CA certificate, %w", err)
		}
	}
	certBytes, keyBytes = encode(cert, key)
	return
}

// GenerateCertAndKey for a given config and valid CA, will check existing certs
// if they are valid, else will generate new cert and key for the
// certutil.Config provided
func GenerateCertAndKey(config *CertConfig, caCertBytes, caKeyBytes []byte) (certBytes, keyBytes []byte, err error) {
	cert, key, err := privateKeyAndCertificate(config, caCertBytes, caKeyBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("creating certificate authority, %w,", err)
	}
	certBytes, keyBytes = encode(cert, key)
	return
}

func validCerts(certBytes, keyBytes []byte) (*x509.Certificate, crypto.Signer, error) {
	cert, err := certutil.ParseCertsPEM(certBytes)
	if err != nil {
		return nil, nil, err
	}
	key, err := keyutil.ParsePrivateKeyPEM(keyBytes)
	if err != nil {
		return nil, nil, err
	}
	return cert[0], key.(crypto.Signer), nil
}

func encode(cert *x509.Certificate, key crypto.Signer) (certBytes, keyBytes []byte) {
	certBytes = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
	keyBytes = pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key.(*rsa.PrivateKey)),
	})
	return
}

func privateKeyAndCertificate(config *CertConfig, caCertBytes, caKeyBytes []byte) (*x509.Certificate, crypto.Signer, error) {
	// verify the existing certs if valid use
	cert, key, err := validCerts(config.ExistingCert, config.ExistingKey)
	if err == nil {
		zap.S().Infof("Reusing existing certs for %s", config.CommonName)
		return cert, key, nil
	}
	// create private key, defaults to x509.RSA
	if key, err = rsa.GenerateKey(cryptorand.Reader, rsaKeySize); err != nil {
		return nil, nil, fmt.Errorf("unable to create private key while generating CA certificate, %w", err)
	}
	if cert, err = signedCert(config.Config, key, caCertBytes, caKeyBytes); err != nil {
		return nil, nil, fmt.Errorf("creating signed cert, %w", err)
	}
	return cert, key, nil
}

// signedCert creates a signed certificate using the given CA certificate and key
func signedCert(cfg *certutil.Config, key crypto.Signer, caCertBytes, caKeyBytes []byte) (*x509.Certificate, error) {
	caCert, caKey, err := validCerts(caCertBytes, caKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("root CA is invalid, %w", err)
	}
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
