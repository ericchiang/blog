package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
)

var handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "You're using HTTPS")
})

func main() {
	certExample, err := tls.LoadX509KeyPair("example-org.crt", "example-org.key")
	if err != nil {
		log.Fatalf("loading cert for example.org: %v", err)
	}
	certSpam, err := tls.LoadX509KeyPair("spam-org.crt", "spam-org.key")
	if err != nil {
		log.Fatalf("loading cert for spam.org: %v", err)
	}

	tlsConfig := &tls.Config{
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			// Return a different certificate depending on the host requested
			switch hello.ServerName {
			case "example.org":
				return &certExample, nil
			case "spam.org":
				return &certSpam, nil
			default:
				return nil, fmt.Errorf("unknown server name: %s", hello.ServerName)
			}
		},
	}
	s := &http.Server{
		Addr:      ":443",
		Handler:   handler,
		TLSConfig: tlsConfig,
	}
	log.Fatal(s.ListenAndServe())
}
