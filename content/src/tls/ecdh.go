package main

import (
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"log"
)

func main() {
	curve := elliptic.P256()
	priv1, pubX1, pubY1, err := elliptic.GenerateKey(curve, rand.Reader)
	if err != nil {
		log.Fatalf("generate key: %v", err)
	}
	priv2, pubX2, pubY2, err := elliptic.GenerateKey(curve, rand.Reader)
	if err != nil {
		log.Fatalf("generate key: %v", err)
	}

	// priv1 + pub2
	gotX1, gotY1 := curve.ScalarMult(pubX2, pubY2, priv1)
	fmt.Println(gotX1, gotY1)
	// priv2 + pub1
	gotX2, gotY2 := curve.ScalarMult(pubX1, pubY1, priv2)
	fmt.Println(gotX2, gotY2)
}
