package main

import (
	"fmt"
	"log"
	"net/http"

	"golang.org/x/crypto/acme/autocert"
)

func main() {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "You're using HTTPS")
	})

	m := &autocert.Manager{
		Cache:      autocert.DirCache("secret-dir"),
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist("example.org", "www.example.org"),
	}

	server := http.Server{
		Addr:      ":https",
		Handler:   handler,
		TLSConfig: m.TLSConfig(),
	}
	log.Fatal(server.ListenAndServeTLS("", ""))
}
