package main

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"time"

	"pault.ag/go/ykpiv"
)

func main() {
	var (
		defaultPIN              = "123456"
		defaultPUK              = "12345678"
		defaultManagementKey, _ = hex.DecodeString("010203040506070801020304050607080102030405060708")
	)

	yk, err := ykpiv.New(ykpiv.Options{
		PIN:           &defaultPIN, // Real use cases should change PINs
		PUK:           &defaultPUK,
		ManagementKey: defaultManagementKey,
	})
	if err != nil {
		log.Fatalf("opening yubikey: %v", err)
	}
	defer yk.Close()
	if err := yk.Login(); err != nil {
		log.Fatalf("logging into yubikey: %v", err)
	}
	if err := yk.Authenticate(); err != nil {
		log.Fatalf("authenticating to yubikey: %v", err)
	}

	// Generate a private key on the yubikey. The Returned slot must immediately
	// be used to sign a CSR or self-signed certificate.
	slotID := ykpiv.Authentication
	slot, err := yk.GenerateECWithPolicies(slotID, 256, ykpiv.PinPolicyNever, ykpiv.TouchPolicyNever)
	if err != nil {
		log.Fatalf("generating key on cert: %v", err)
	}
	csr := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: "my-server", Organization: []string{"Acme Co"},
		},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csr, slot)
	if err != nil {
		log.Fatalf("generate csr: %v", err)
	}

	// Generate a key-pair
	caPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatalf("generating self-signed cert: %v", err)
	}
	caPub := caPriv.Public()
	newSerialNum := func() *big.Int {
		serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		if err != nil {
			panic(err)
		}
		return serialNumber
	}

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
	servCert, err := x509.ParseCertificate(servCertDER)
	if err != nil {
		log.Fatalf("parsing server cert: %v", err)
	}

	// Store the cert in the yubikey slot
	if err := slot.Update(*servCert); err != nil {
		log.Fatalf("loading cert to yubikey: %v", err)
	}
	// Create a TLS certificate using the slot
	tlsCert := tls.Certificate{
		Certificate: [][]byte{slot.Certificate.Raw},
		// Suppress slot.Decrypt since this is an EC key (indirection isn't necessary
		// for RSA keys).
		PrivateKey: struct{ crypto.Signer }{slot},
		Leaf:       slot.Certificate,
	}
	serv := http.Server{
		Addr: "127.0.0.1:8443",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "You're using HTTPS")
		}),
		TLSConfig: &tls.Config{
			Certificates:             []tls.Certificate{tlsCert},
			MinVersion:               tls.VersionTLS12,
			PreferServerCipherSuites: true,
		},
	}
	// Begin serving TLS
	go func() {
		if err := serv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	pemEncode := func(b []byte, t string) []byte {
		return pem.EncodeToMemory(&pem.Block{Bytes: b, Type: t})
	}
	caCertPEM := pemEncode(caCertDER, "CERTIFICATE")

	// Configure a client to trust the server
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(caCertPEM)
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    certPool,
				MinVersion: tls.VersionTLS12,
			},
		},
	}
	resp, err := client.Get("https://127.0.0.1:8443/")
	if err != nil {
		log.Fatalf("GET failed: %v", err)
	}
	resp.Body.Close()

	serv.Shutdown(context.Background())
}
