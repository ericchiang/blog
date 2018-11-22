package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"log"
)

func main() {
	// Generate a key
	key := [32]byte{}
	if _, err := io.ReadFull(rand.Reader, key[:]); err != nil {
		log.Fatalf("generating symmetric key: %v", err)
	}
	// Create an "authenticated encryption" construct
	block, err := aes.NewCipher(key[:])
	if err != nil {
		log.Fatalf("creating new cipher: %v", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		log.Fatalf("creating new AEAD: %v", err)
	}
	// Generate a nonce
	nonce := make([]byte, aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		log.Fatalf("generating random nonce: %v", err)
	}

	// Encrypt then decrypt
	plainText := []byte("The bourgeois human is a virus on the hard drive of the working robot!")
	cipherText := aead.Seal([]byte{}, nonce, plainText, nil)
	text, err := aead.Open(nil, nonce, cipherText, nil)
	if err != nil {
		log.Printf("decrypting data: %v", err)
	}

	fmt.Printf("%s\n", plainText)
	fmt.Printf("%q\n", cipherText)
	fmt.Printf("%s\n", text)
}
