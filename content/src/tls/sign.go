package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha512"
	"fmt"
	"log"
)

func main() {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatalf("generate key: %v", err)
	}

	msg := []byte("The bourgeois human is a virus on the hard drive of the working robot!")
	hash := sha512.Sum512(msg)

	r, s, err := ecdsa.Sign(rand.Reader, priv, hash[:])
	if err != nil {
		log.Fatalf("signing message: %v", err)
	}

	pub := &priv.PublicKey
	fmt.Println(ecdsa.Verify(pub, hash[:], r, s))
}
