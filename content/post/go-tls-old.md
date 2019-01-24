+++
title = "TLS with Go (old)"
date = "2015-06-21"

+++

_NOTE: An update version of this post can be found [here](../go-tls)._

For a long time my knowledge of TLS was Googling "how to configure nginx as an HTTPS proxy." Okay, the cert goes here and the key goes here, that's my job done. But with more and more pushes for things HTTPS and HTTP/2 (which defaults to using TLS), it sometimes helps to understand this a little better.

Unfortunately a lot of the articles on this topic are either too high level or too specific and, when I need to learn the topic, I ended up just reading the Go documentation.

This is an article to explain how TLS (and HTTPS) works by creating and using certificates in running servers with Go. If you want, you can follow along by programming the examples in another window.

## Public and Private Key Encryption

Public and private key cryptography is awesome and a good place to start. If you've ever used GitHub or had to log into an EC2 instance, you've seen these things before. A very common use case is to use them to prove your identity. You place your public key where anyone can see it, then use the private one to later confirm you are who you say you are.

Go has a fairly straight forward `crypto/rsa` package in the standard library. First we can just generate a new, random pair.

```go
// create a public/private keypair
// NOTE: Use crypto/rand not math/rand
privKey, err := rsa.GenerateKey(rand.Reader, 2048)
if err != nil {
    log.Fatalf("generating random key: %v", err)
}
```

Here's how this works. Things encrypt with a public key can only be decrypted by its paired private key. This involves some tricks with some very large prime numbers, but that's for another article.

Let's actually encrypt something using the public key.

```go
plainText := []byte("The bourgeois human is a virus on the hard drive of the working robot!")

// use the public key to encrypt the message
cipherText, err := rsa.EncryptPKCS1v15(rand.Reader, &privKey.PublicKey, plainText)
if err != nil {
    log.Fatalf("could not encrypt data: %v", err)
}
fmt.Printf("%s\n", strconv.Quote(string(cipherText)))
```

As you can see, it's even an effort to print the junk that's results from this encryption.

```
"\x83\xcc\x11\xe9\x1b<\x9a\xab\xa3H\fq\xfb]\xb7\x8a\xd3\xfb3\xad\xfe\x01\x1d\x86d\x1e\xf7\xf0t.\xc8\x03f\xd7J\xd6\u0086\\\xb83\xad\x82\xb0I\xe51:\xe0\x8c\x94\xfe}\xb5\x17\xeb_\x13S\x17\xfah\xbe\xcd=3\a\xee\xd0u\xd0\xf1$\xc2\b\xf0`\xb2x\xbd\x99\xc0\xf8\xbc`\xe7\x8f黭g\xe1\xa1j\x89\x15\xee,\u061d\xff\xfe\xb7\x84\xbf\x8b}t٫\xa0\x10Y)\xaa\xc4M\x18\xac5\xc9ٗD<\xc1&f\xeb\xf9S(\x97s\x01\xc2s\x1cu\a\x82\x1e1q\xe83Č9\x04\x17\x8c\x1b\xba`\x9f,.\xdc|%6\xa5f\xaf\xdb\xd51\xabJ\xf6#\x11+S=px\xcc +87\xe5\x16\x062\xb6\xda\x0e~_>f,S\x80\xb7\xca\x12w\xf1\xaa\x83\xe3\xde j\xc2\xfd\x1e\xe6s\x88|\xf2?{\x80\x8c\xfb\x916\xbf\xb8\xc7\xee\x81U\x9e1\xc1s\x86p\x01\x80]r\xa5\v\xdb|\x84ץ\xce8\xb7\x0f\xf6\xd7\x02E\xc5u"
```

To decrypt this cipher text, we simply use the private key.

```go
// decrypt with the private key
decryptedText, err := rsa.DecryptPKCS1v15(nil, privKey, cipherText)
if err != nil {
	log.Fatalf("error decrypting cipher text: %v", err)
}
fmt.Printf("%s\n", decryptedText)
// The bourgeois human is a virus on the hard drive of the working robot!
```

Cool, but what good is this?

Let's say I had your public key and, while talking on the Internet, I want to confirm that you are really you. What I could do is think of a random phrase, let's say `"Well, that's no ordinary rabbit."` and encrypted it with your public key. You would have to have the private key to decrypt it, and if you where able to say that phrase back to me, I could confirm that I was really talking to you.

The cool part about this is __you can prove you hold a private key without ever showing it to somebody__.

## Digital Signatures

A second trait of public private key-pairs is the ability to create a digital signature for a given message. These signatures can be used to ensure the validity of the document it signs.

To to this, the document is run through a hashing algorithm (we'll use SHA256), then the __private__ key computes a signature for the hashed results.

The __public__ key can then confirm, again through math we'll ignore, if its private key combined with a particular hash would have created that signature. Here's what that looks like using `crypto/rsa`.

```go
// compute the hash of our original plain text
hash := sha256.Sum256(plainText)
fmt.Printf("The hash of my message is: %#x\n", hash)
// The hash of my message is: 0xe6a8502561b8e2328b856b4dbe6a9448d2bf76f02b7820e5d5d4907ed2e6db80

// generate a signature using the private key
signature, err := rsa.SignPKCS1v15(rand.Reader, privKey, crypto.SHA256, hash[:])
if err != nil {
    log.Fatalf("error creating signature: %v", err)
}
// let's not print the signature, it's big and ugly
```

We can then attempt to verify the result with different combinations of messages and signatures.

```go
// use a public key to verify the signature for a message was created by the private key
verify := func(pub *rsa.PublicKey, msg, signature []byte) error {
    hash := sha256.Sum256(msg)
    return rsa.VerifyPKCS1v15(pub, crypto.SHA256, hash[:], signature)
}

fmt.Println(verify(&privKey.PublicKey, plainText, []byte("a bad signature")))
// crypto/rsa: verification error
fmt.Println(verify(&privKey.PublicKey, []byte("a different plain text"), signature))
// crypto/rsa: verification error
fmt.Println(verify(&privKey.PublicKey, plainText, signature))
// <nil>
```

What this signature is doing is confirming that a document has not been changed since the private key signed it. And because a public key is public and can be posted anywhere, anyone can run this same test.

This might be very helpful for say, a certificate authority, who wants to be able to distribute documents which can't be altered without everyone detecting.

## Go and x509

Go's `crypto/x509` package is what I'll be using to actually generate and work with certificates. It's a package with a lot of options and a somewhat intimidating interface. For instance, the ridiculous number of fields on the <a href="https://golang.org/pkg/crypto/x509/#Certificate" target="_blank">`Certificate`</a> struct.

To create a new certificate, we first have to provide a template for one. Because we'll be doing this a couple times, I've made a helper function to do some of the busy work.

```go
// helper function to create a cert template with a serial number and other required fields
func CertTemplate() (*x509.Certificate, error) {
    // generate a random serial number (a real cert authority would have some logic behind this)
    serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
    serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
    if err != nil {
        return nil, errors.New("failed to generate serial number: " + err.Error())
    }

    tmpl := x509.Certificate{
        SerialNumber:          serialNumber,
        Subject:               pkix.Name{Organization: []string{"Yhat, Inc."}},
        SignatureAlgorithm:    x509.SHA256WithRSA,
        NotBefore:             time.Now(),
        NotAfter:              time.Now().Add(time.Hour), // valid for an hour
        BasicConstraintsValid: true,
    }
    return &tmpl, nil
}
```

Certificates are public keys with some attached information (like what domains they work for). In order to create a certificate, we need to both specify that information and provide a public key.

In this next block, we create a key-pair called `rootKey` and a certificate template called `rootCertTmpl`, then fill out some information about what we want to use it for.

```go
// generate a new key-pair
rootKey, err := rsa.GenerateKey(rand.Reader, 2048)
if err != nil {
	log.Fatalf("generating random key: %v", err)
}

rootCertTmpl, err := CertTemplate()
if err != nil {
	log.Fatalf("creating cert template: %v", err)
}
// describe what the certificate will be used for
rootCertTmpl.IsCA = true
rootCertTmpl.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature
rootCertTmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
rootCertTmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
```

Now the fun part.

## Making a Self-Signed Certificate

Okay, it's time to actually create a certificate.

Certificates must be signed by the private key of a parent certificate. Of course, there always has to be a certificate without a parent, and in these cases the certificate's private key must be used in lieu of a parent's.

`x509.CreateCertificate` takes 4 arguments (plus a source of randomness). The template of the certificate we want to create, the public key we want to wrap, the parent certificate, and the parent's private key.

```go
func CreateCert(template, parent *x509.Certificate, pub interface{}, parentPriv interface{}) (
    cert *x509.Certificate, certPEM []byte, err error) {

    certDER, err := x509.CreateCertificate(rand.Reader, template, parent, pub, parentPriv)
    if err != nil {
        return
    }
    // parse the resulting certificate so we can use it again
    cert, err = x509.ParseCertificate(certDER)
    if err != nil {
        return
    }
    // PEM encode the certificate (this is a standard TLS encoding)
    b := pem.Block{Type: "CERTIFICATE", Bytes: certDER}
    certPEM = pem.EncodeToMemory(&b)
    return
}
```

To create our self-signed cert (named `rootCert`), we provide the arguments listed above. But instead of using a parent certificate, the root key's information is used instead.

```go
rootCert, rootCertPEM, err := CreateCert(rootCertTmpl, rootCertTmpl, &rootKey.PublicKey, rootKey)
if err != nil {
	log.Fatalf("error creating cert: %v", err)
}
fmt.Printf("%s\n", rootCertPEM)
fmt.Printf("%#x\n", rootCert.Signature) // more ugly binary
```

While printing out the signature isn't incredibly useful, `rootCertPEM` should look very familiar for anyone who's configured HTTPS or SSH'd into a server. Here's what my code generated.

```nohighlight
-----BEGIN CERTIFICATE-----
MIIC+jCCAeSgAwIBAgIRAK2uh2q3B+iVYia2l87Tch8wCwYJKoZIhvcNAQELMBUx
EzARBgNVBAoTClloYXQsIEluYy4wHhcNMTUwNjIwMjI1MDEyWhcNMTUwNjIwMjM1
MDEyWjAVMRMwEQYDVQQKEwpZaGF0LCBJbmMuMIIBIjANBgkqhkiG9w0BAQEFAAOC
AQ8AMIIBCgKCAQEAr3Y+KLFritC5CAsTCvYlZj/jczJrGmBNaLHtIUDSOQlrwEXy
DJqyl5kY8osu0YyZOFVsSbs/xNk5Hm9TmU/NSIxhGxJXkgd2QgeAzUP/zWWvvDiW
DL3KBu1FVbKnEdFd+7b3FHguHHh8/iHaeB09QgrX0cuf7ePC4PGKeIa9C8yQ8MNO
q6foQJ9H3p83oSUyl53obMP199Dseu8wVoTekzhesm/N6D2Rhb745T+RcQ8AguXd
xIob0x0D/orPprcvGDaabqiZnIS5zXVtdbgzKdpBc5Gwnb9b8cFICriOapVFWSLO
3Ta5uUDuUIDuwg/4Q66bJZqnNHlLoC/h1zvS6QIDAQABo0kwRzAOBgNVHQ8BAf8E
BAMCAIQwEwYDVR0lBAwwCgYIKwYBBQUHAwEwDwYDVR0TAQH/BAUwAwEB/zAPBgNV
HREECDAGhwR/AAABMAsGCSqGSIb3DQEBCwOCAQEAJgNp51KA3v774tx8gFP4OMrL
wfpFZVhIOid35ol0dX0/oOXSUXs28AMIhpou/vWH5REkFadPxtZD1ErHzgB/h7Ce
Iln9L9ZIC/QMA93chNsDaj+M+Np9p4AckrO9BthqhWjqIbdwkRC4cb4gN1vei1MP
Pu1nhdvE3PKX4VG5pqc1DaMyKDotc1pc5jaOkz3NAGyTPn9PUyfQP88FqnYaf5/a
K5Vulo8NmzMOCcBjAJ9B0IXOLg9ba+dyiOK8pIayBiX28FRaxRUiU31iEPI8gbTN
/6W3f//C3eTDCCLwEmGOmOalpBnaF4wsA6CTxDmwDyTmj9+TRkaEEylEQTlXZA==
-----END CERTIFICATE-----
```

Right away we can use this PEM encoded block in a server. However, let's remember that a certificate is just a public key. To prove ownership of a certificate you must have the private key as well. In the case of our server, we have to load `rootKey` if we want to use `rootCert`.

To keep this code cleaner, I'm going to use the <a href="https://golang.org/pkg/net/http/httptest/" target="_blank">`httptest`</a> package, which will allow us to spin up and shut down servers on a whim.

```go
// PEM encode the private key
rootKeyPEM := pem.EncodeToMemory(&pem.Block{
	Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rootKey),
})

// Create a TLS cert using the private key and certificate
rootTLSCert, err := tls.X509KeyPair(rootCertPEM, rootKeyPEM)
if err != nil {
	log.Fatalf("invalid key pair: %v", err)
}

ok := func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("HI!")) }
s := httptest.NewUnstartedServer(http.HandlerFunc(ok))

// Configure the server to present the certficate we created
s.TLS = &tls.Config{
	Certificates: []tls.Certificate{rootTLSCert},
}
```

We can now make a HTTP request to the server, where we get a very familiar error message.

```go
// make a HTTPS request to the server
s.StartTLS()
_, err = http.Get(s.URL)
s.Close()

fmt.Println(err)
// http: TLS handshake error from 127.0.0.1:52944: remote error: bad certificate
```

The `net/http` package has rejected the certificate.

By default, `net/http` loads trusted certificates (public keys) from your computer. These are the same ones your browser uses when you surf the web. The issue is, the certificate we create, which the test server provided, has a digital signature. But, none of the public keys trusted by your browser validated that signature.

## Getting the Client to Trust the Server

Rather than using a self-signed certificate, let's create a setup that mimics a real situation where a certificate authority provides a organization with a cert for their website. To do this, we'll pretend the `rootCert` we created before belongs to the certificate authority, and we'll be attempting to create another certificate for our server.

First things first, we'll create a new key-pair and template.

```go
// create a key-pair for the server
servKey, err := rsa.GenerateKey(rand.Reader, 2048)
if err != nil {
	log.Fatalf("generating random key: %v", err)
}

// create a template for the server
servCertTmpl, err := CertTemplate()
if err != nil {
	log.Fatalf("creating cert template: %v", err)
}
servCertTmpl.KeyUsage = x509.KeyUsageDigitalSignature
servCertTmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
servCertTmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
```

To create the server certificate, we're going to use a real parent this time. And again, we provide a public key for the certificate, and the parents private key (`rootKey`) to do the signing.

```go
// create a certificate which wraps the server's public key, sign it with the root private key
_, servCertPEM, err := CreateCert(servCertTmpl, rootCert, &servKey.PublicKey, rootKey)
if err != nil {
	log.Fatalf("error creating cert: %v", err)
}
```

We now have a PEM encoded certificate. To use this in a server, we have to have the private key to prove we own it.

```go
// provide the private key and the cert
servKeyPEM := pem.EncodeToMemory(&pem.Block{
	Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(servKey),
})
servTLSCert, err := tls.X509KeyPair(servCertPEM, servKeyPEM)
if err != nil {
	log.Fatalf("invalid key pair: %v", err)
}
// create another test server and use the certificate
s = httptest.NewUnstartedServer(http.HandlerFunc(ok))
s.TLS = &tls.Config{
	Certificates: []tls.Certificate{servTLSCert},
}
```

If we made another request here, we'd still be in the same situation as before when our client reject the certificate.

To avoid this, we need to create a client which "trusts" `servCert`. Specifically, we have to trust a public key which validates `servCert`'s signature. Since we use the `root` key-pair to sign the certificate, if we trust `rootCert` (the public key), we'll trust the server's certificate.

```go
// create a pool of trusted certs
certPool := x509.NewCertPool()
certPool.AppendCertsFromPEM(rootCertPEM)

// configure a client to use trust those certificates
client := &http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: certPool},
	},
}
```

When the server provides a certificate, the client will now validate the signature using all the certificates in `certPool` rather than the ones on my laptop. Let's see if this worked.

```go
s.StartTLS()
resp, err := client.Get(s.URL)
s.Close()
if err != nil {
	log.Fatalf("could not make GET request: %v", err)
}
dump, err := httputil.DumpResponse(resp, true)
if err != nil {
	log.Fatalf("could not dump response: %v", err)
}
fmt.Printf("%s\n", dump)
```

And boom, we're speaking HTTPS.

```nohighlight
HTTP/1.1 200 OK
Content-Length: 3
Content-Type: text/plain; charset=utf-8
Date: Sat, 20 Jun 2015 22:50:14 GMT

HI!
```

## Conclusion

Oddly enough, TLS is often more about managing certificates and private keys than worrying about how the actual over the wire encryption works. It's also important to make sure that your servers and clients work with things like HTTPS. And it's a bit of a hack to just turn verification off.

But as you learn more about TLS, you can find that it's really powerful. Even if you aren't serving HTTP traffic, being able to doing able to do this kind of verification and encryption is a lot easier than trying to set something else up yourself. And the next time a website dumps a bunch of `.crt` files on you, you'll hopefully be able to understand exactly what to do with them.

## Bonus: Getting the Server to Trust the Client

Most web servers don't care who the client is who's accessing them. Or at least the client authentication they do do isn't at the TCP layer, it's done with session tokens and HTTP middleware.

While websites don't find this kind of auth particularly useful, databases and other architecture like a compute clusters, when a server wants to verify it's client without a password, can use this to both restrict access and encrypt communications. For instance, the is what `boot2docker` does in its more recent releases, while Google's <a href="https://github.com/GoogleCloudPlatform/kubernetes/issues/3168#issuecomment-104503217" target="_blank">Kubernetes platform</a> has plans to use this for secure master to worker communication.

It's easy to turn on client authentication for a Go server.

```go
// create a new server which requires client authentication
s = httptest.NewUnstartedServer(http.HandlerFunc(ok))
s.TLS = &tls.Config{
	Certificates: []tls.Certificate{servTLSCert},
	ClientAuth:   tls.RequireAndVerifyClientCert,
}

s.StartTLS()
_, err = client.Get(s.URL)
s.Close()
fmt.Println(err)
```

After the request is made, we'll actually see the server log something like this. It's rejected the client.

```nohighlight
http: TLS handshake error from 127.0.0.1:47038: tls: client didn't provide a certificate
```

In order for the client to provide a certificate, we have to create a template first.

```go
// create a key-pair for the client
clientKey, err := rsa.GenerateKey(rand.Reader, 2048)
if err != nil {
	log.Fatalf("generating random key: %v", err)
}

// create a template for the client
clientCertTmpl, err := CertTemplate()
if err != nil {
	log.Fatalf("creating cert template: %v", err)
}
clientCertTmpl.KeyUsage = x509.KeyUsageDigitalSignature
clientCertTmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
```

When creating a new certificate we'll again have the `rootCert` sign it. It doesn't have to be the same parent as the server, but this makes the example easier.

```go
// the root cert signs the cert by again providing its private key
_, clientCertPEM, err := CreateCert(clientCertTmpl, rootCert, &clientKey.PublicKey, rootKey)
if err != nil {
	log.Fatalf("error creating cert: %v", err)
}

// encode and load the cert and private key for the client
clientKeyPEM := pem.EncodeToMemory(&pem.Block{
	Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(clientKey),
})
clientTLSCert, err := tls.X509KeyPair(clientCertPEM, clientKeyPEM)
if err != nil {
	log.Fatalf("invalid key pair: %v", err)
}
```

The client now needs to trust the server's cert by trusting the cert pool we made earlier. As a reminder this holds the `rootCert`. It's also needs to present its own certificate.

```go
authedClient := &http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:      certPool,
			Certificates: []tls.Certificate{clientTLSCert},
		},
	},
}
```

Of course, the server still can't verify the client. If we made a request now, we'd see something like this.

```nohighlight
http: TLS handshake error from 127.0.0.1:59756: tls: failed to verify client's certificate: x509: certificate signed by unknown authority
```

To get around this, we have to configure a new test server to both present the server certificate, and trust the client's (by trusting `certPool` which holds `rootCert`).

```go
s = httptest.NewUnstartedServer(http.HandlerFunc(ok))
s.TLS = &tls.Config{
	Certificates: []tls.Certificate{servTLSCert},
	ClientAuth:   tls.RequireAndVerifyClientCert,
	ClientCAs:    certPool,
}
s.StartTLS()
_, err = authedClient.Get(s.URL)
s.Close()
fmt.Println(err)
// <nil>
```

And there you go, we've negotiated a secure conversation between a client and a server who both trust that each is properly authenticated.
