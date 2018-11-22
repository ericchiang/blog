package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	ct "github.com/google/certificate-transparency-go"
	"github.com/google/certificate-transparency-go/client"
	"github.com/google/certificate-transparency-go/jsonclient"
)

func main() {
	const ctEndpoint = "https://ct.googleapis.com/pilot"
	c, err := client.New(ctEndpoint, http.DefaultClient, jsonclient.Options{})
	if err != nil {
		log.Fatalf("creating client: %v", err)
	}
	head, err := c.GetSTH(context.Background())
	if err != nil {
		log.Fatalf("getting tree head: %v", err)
	}
	t := ct.TimestampToTime(head.Timestamp)
	fmt.Printf("number of entries at %s: %d\n", t, head.TreeSize)

	// Get the first 5 entries in the certificate log
	entries, err := c.GetEntries(context.Background(), 0, 4)
	if err != nil {
		log.Fatalf("get entries: %v", err)
	}
	for _, e := range entries {
		if e.X509Cert == nil {
			continue
		}
		t := ct.TimestampToTime(e.Leaf.TimestampedEntry.Timestamp)
		fmt.Println("index:", e.Index)
		fmt.Println("  cn:", e.X509Cert.Subject.CommonName)
		fmt.Println("  timestamp:", t)
		fmt.Println("  dns names:", e.X509Cert.DNSNames)
	}
}
