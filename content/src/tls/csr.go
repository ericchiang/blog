package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"log"
	"math/big"
	"net"
	"time"
)

func newSerialNum() *big.Int {
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		panic(err)
	}
	return serialNumber
}

func main() {
	servPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatalf("generating self-signed cert: %v", err)
	}
	csr := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: "my-server", Organization: []string{"Acme Co"},
		},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csr, servPriv)
	if err != nil {
		log.Fatalf("generate csr: %v", err)
	}

	caPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatalf("generating self-signed cert: %v", err)
	}
	caPub := caPriv.Public()
	// Generate a self-signed certificate
	caTmpl := &x509.Certificate{
		Subject: pkix.Name{
			CommonName: "my-ca", Organization: []string{"Acme Co"},
		},
		SerialNumber:          newSerialNum(),
		BasicConstraintsValid: true,
		IsCA:                  true,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageKeyEncipherment |
			x509.KeyUsageDigitalSignature |
			x509.KeyUsageCertSign,
	}
	caCertDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, caPub, caPriv)
	if err != nil {
		log.Fatalf("creating self-signed cert: %v", err)
	}
	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		log.Fatalf("parsing ca cert: %v", err)
	}

	servCSR, err := x509.ParseCertificateRequest(csrDER)
	if err != nil {
		log.Fatalf("parsing csr: %v", err)
	}
	if err := servCSR.CheckSignature(); err != nil {
		log.Fatalf("checking csr signature: %v", err)
	}

	// Certificate authority MUST validate CSR fields before using them to
	// generate a certificate

	servTmpl := &x509.Certificate{
		// Fields taken from CSR
		Subject:     servCSR.Subject,
		IPAddresses: servCSR.IPAddresses,
		DNSNames:    servCSR.DNSNames,

		// Fields that must be requested externally from the CSR
		SerialNumber: newSerialNum(),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	servPub := servCSR.PublicKey
	servCertDER, err := x509.CreateCertificate(rand.Reader, servTmpl, caCert, servPub, caPriv)
	if err != nil {
		log.Fatalf("signing server cert: %v", err)
	}
	_ = servCert
}
