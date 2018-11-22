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

	"golang.org/x/crypto/ocsp"
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
	servCert, err := x509.ParseCertificate(servCertDER)
	if err != nil {
		log.Fatalf("parsing server cert: %v", err)
	}

	servCertPEM := pemEncode(servCertDER, "CERTIFICATE")
	servPrivPEM := pemEncode(servPrivDER, "EC PRIVATE KEY")
	// Load the certificate and private key as a TLS certificate
	servTLSCert, err := tls.X509KeyPair(servCertPEM, servPrivPEM)
	if err != nil {
		log.Fatalf("parsing x509 key pair: %v", err)
	}

	servTLSCert.OCSPStaple, err = ocsp.CreateResponse(caCert, servCert, ocsp.Response{
		Status:       ocsp.Good,
		SerialNumber: servCert.SerialNumber,
		ThisUpdate:   time.Now(),
		NextUpdate:   time.Now().Add(time.Minute),
	}, caPriv)
	if err != nil {
		log.Fatalf("creating ocsp staple: %v", err)
	}

	verifyOCSP := func(conn *tls.Conn) error {
		s := conn.ConnectionState()
		if len(s.OCSPResponse) == 0 {
			return fmt.Errorf("remote didn't provide ocsp staple response")
		}
		for _, chain := range s.VerifiedChains {
			for i := 1; i < len(chain); i++ {
				issuer, cert := chain[i], chain[i-1]
				resp, err := ocsp.ParseResponseForCert(s.OCSPResponse, cert, issuer)
				if err != nil {
					return fmt.Errorf("invalid ocsp staple data: %v", err)
				}
				if err := resp.CheckSignatureFrom(issuer); err != nil {
					return fmt.Errorf("invalid ocsp signature: %v", err)
				}
				if resp.Status != ocsp.Good {
					return fmt.Errorf("certificate revoked /cn=%s",
						cert.Subject.CommonName)
				}
			}
		}
		return nil
	}

	rootCAs := x509.NewCertPool()
	rootCAs.AddCert(caCert)
	tlsClientConfig := &tls.Config{RootCAs: rootCAs}
	client := http.Client{
		Transport: &http.Transport{
			DialTLS: func(network, addr string) (net.Conn, error) {
				conn, err := tls.Dial(network, addr, tlsClientConfig)
				if err != nil {
					return nil, err
				}
				if err := verifyOCSP(conn); err != nil {
					conn.Close()
					return nil, fmt.Errorf("ocsp validation failed: %v", err)
				}
				return conn, nil
			},
		},
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
	resp, err := client.Get("https://127.0.0.1:8443/")
	if err != nil {
		log.Fatalf("GET failed: %v", err)
	}
	resp.Body.Close()

	serv.Shutdown(context.Background())
	l.Close()

}
