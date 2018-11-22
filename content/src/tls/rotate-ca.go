package main

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"log"
	"net/http"
)

func main() {
	// ca.pem contains multiple PEM encoded certificates
	caBundle, err := ioutil.ReadFile("ca.pem")
	if err != nil {
		log.Fatalf("loading certificate bundle: %v", err)
	}
	certPool := x509.NewCertPool()
	// AppendCertsFromPEM automatically handles multiple certificates
	certPool.AppendCertsFromPEM(caBundle)
	// Client trusts both the new and old certificate
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    certPool,
				MinVersion: tls.VersionTLS12,
			},
		},
	}
	_ = client
}
