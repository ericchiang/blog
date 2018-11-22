package main

import (
	"context"
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

func main() {
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

	// PEM encode the certificate and private key
	pemEncode := func(b []byte, t string) []byte {
		return pem.EncodeToMemory(&pem.Block{Bytes: b, Type: t})
	}
	caCertPEM := pemEncode(caCertDER, "CERTIFICATE")
	caPrivDER, err := x509.MarshalECPrivateKey(caPriv)
	if err != nil {
		log.Fatalf("marshaling x509 private key: %v", err)
	}
	caPrivPEM := pemEncode(caPrivDER, "EC PRIVATE KEY")

	fmt.Printf("%s", caCertPEM)
	fmt.Printf("%s", caPrivPEM)

	// Generate a key pair and certificate template
	servPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatalf("generating self-signed cert: %v", err)
	}
	servPub := servPriv.Public()
	servTmpl := &x509.Certificate{
		Subject: pkix.Name{
			CommonName: "my-server", Organization: []string{"Acme Co"},
		},
		SerialNumber: newSerialNum(),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	// Sign the serving cert with the CA private key
	servCertDER, err := x509.CreateCertificate(rand.Reader, servTmpl, caCert, servPub, caPriv)
	if err != nil {
		log.Fatalf("creating servingCert cert: %v", err)
	}
	servPrivDER, err := x509.MarshalECPrivateKey(servPriv)
	if err != nil {
		log.Fatalf("marshaling x509 private key: %v", err)
	}

	servCertPEM := pemEncode(servCertDER, "CERTIFICATE")
	servPrivPEM := pemEncode(servPrivDER, "EC PRIVATE KEY")

	// Load the certificate and private key as a TLS certificate
	servTLSCert, err := tls.X509KeyPair(servCertPEM, servPrivPEM)
	if err != nil {
		log.Fatalf("parsing x509 key pair: %v", err)
	}

	serv := http.Server{
		Addr: "127.0.0.1:8443",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "You're using HTTPS")
		}),
		// Configure TLS options
		TLSConfig: &tls.Config{
			Certificates:             []tls.Certificate{servTLSCert},
			MinVersion:               tls.VersionTLS12,
			PreferServerCipherSuites: true,
		},
	}
	// Begin serving TLS
	l, err := net.Listen("tcp", serv.Addr)
	if err != nil {
		log.Fatalf("creating tcp listener: %v", err)
	}
	go func() {
		if err := serv.ServeTLS(l, "", ""); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

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
	l.Close()

	cliPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatalf("generating self-signed cert: %v", err)
	}
	cliPub := cliPriv.Public()

	cliTmpl := &x509.Certificate{
		Subject: pkix.Name{
			CommonName: "my-client", Organization: []string{"Acme Co"},
		},
		SerialNumber: newSerialNum(),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	cliCert, err := x509.CreateCertificate(rand.Reader, cliTmpl, caCert, cliPub, caPriv)
	if err != nil {
		log.Fatalf("creating cliingCert cert: %v", err)
	}
	cliPrivDER, err := x509.MarshalECPrivateKey(cliPriv)
	if err != nil {
		log.Fatalf("marshaling x509 private key: %v", err)
	}

	cliCertPEM := pemEncode(cliCert, "CERTIFICATE")
	cliPrivPEM := pemEncode(cliPrivDER, "EC PRIVATE KEY")

	cliTLSCert, err := tls.X509KeyPair(cliCertPEM, cliPrivPEM)
	if err != nil {
		log.Fatalf("parsing x509 key pair: %v", err)
	}
	client = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{cliTLSCert},
				RootCAs:      certPool,
				MinVersion:   tls.VersionTLS12,
			},
		},
	}

	serv = http.Server{
		Addr: "127.0.0.1:8443",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "You're using HTTPS")
		}),
		TLSConfig: &tls.Config{
			// MUST use RequireAndVerifyClientCert to require a client cert
			ClientAuth:               tls.RequireAndVerifyClientCert,
			Certificates:             []tls.Certificate{servTLSCert},
			ClientCAs:                certPool,
			MinVersion:               tls.VersionTLS12,
			PreferServerCipherSuites: true,
		},
	}
	l, err = net.Listen("tcp", serv.Addr)
	if err != nil {
		log.Fatalf("creating tcp listener: %v", err)
	}
	go func() {
		if err := serv.ServeTLS(l, "", ""); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	resp, err = client.Get("https://127.0.0.1:8443/")
	if err != nil {
		log.Fatalf("GET failed: %v", err)
	}
	resp.Body.Close()

	serv.Shutdown(context.Background())
	l.Close()
}
