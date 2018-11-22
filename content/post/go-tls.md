+++
title = "TLS with Go"
date = "2018-11-22"
+++

_NOTE: This is an update to a post from 2015. The original version can be found [here](../go-tls-old/)._

If data moves, use TLS.

That’s the expectation of modern infrastructure. A simple rule that says, if your website, backend microservice, cloud native database, or smart light bulbs communicates over the network, it’s got to use TLS for basic levels of security.

Who knew a simple premise could be so challenging? Many projects provide insecure defaults that have a habit of finding their way into the wild. Others generate TLS certificates automatically without being able to integrate with an organization’s existing PKI. Threading this needle is hard, and relies on both a good understanding of how TLS works, and how it’s often deployed.

This post focuses on how to use Go’s TLS packages, with the broader goal of understanding how TLS, HTTPS, and PKI operate.

{{% toc %}}

# Signatures and encryption

TLS leverages various crypto primitives for verifying certificates and encrypting data. While these are unlikely to be directly, it’s helpful to understand the core concepts.

## Signatures

TLS certificates are documents signed by a certificate authority asserting a server can serve traffic for a DNS name or IP. To sign a document, an authority creates a signature using its private key, and publishes its public key for others to verify the signature:

```go
{{% src "src/tls/sign.go" 13 27 %}}
```

The signature verifies correctly, so the code prints:

```
true
```

## Diffie-Hellman

Diffie-Hellman is a key agreement used by TLS to establish a shared symmetric encryption key. The strategy boils down to pub a + priv b = pub b + priv a. For elliptic curves, this means given two keys, multiplying the public key (a point on the curve) by the other’s private key produces the same point. That point can be used to generate a shared key:

```go
{{% src "src/tls/ecdh.go" 11 26 %}}
```

This code prints:

```
43623630832052469319367308988582748734066180954437890962811274082899666546650 48545236146773211130298563748924181841567757775226341947024614668801640669744
43623630832052469319367308988582748734066180954437890962811274082899666546650 48545236146773211130298563748924181841567757775226341947024614668801640669744
```

During a handshake the client and server each generate ephemeral key pairs, then use Diffie-Hellman to establish a shared secret for encryption. Because the keys are thrown away once the connection is established, compromising the server’s certificate key doesn’t let an attacker decrypt old sessions.

## Encryption

Once both parties have agreed on a shared symmetric key, they can use it to encrypt data.

Modern TLS suites use an constructed called Authenticated Encryption with Additional Data (AEAD). AEADs are an abstraction that take a shared key and plain text and “do the right thing” handling complexities like padding, authentication/encryption ordering, and timing attacks.

The following code, copied from [cryptopasta][cryptopasta], encrypts and decrypts a message using a shared secret:

```go
{{% src "src/tls/encrypt.go" 13 43 %}}
```

This program prints:

```
The bourgeois human is a virus on the hard drive of the working robot!
"'۽\xabp\xda\xdaot\xf5\x90\xb8E\xe6\xcd\xe9\x1fq\xb6#pg\xac\x18\x98\x12\xcdp\xde9\x1e4\x16\r87W\\f\x9b\xd9H\xcbT\xf5\xf3\x00m\xa6=*\xef\xd4d\x1f\x04\x0ft\x0e\x1d\x80Y\xed\xbd\x01\xa9{6\xcd_\xe3\x1fO\x11\xb3ۍ\xcaB#S\xae+U\xc8!"
The bourgeois human is a virus on the hard drive of the working robot!
```

For more discussion about the encryption used in TLS, I'd strongly recommend Adam Langley's talk [_"Lessons Learnt from Questions."_][aead-talk]

[aead-talk]: https://www.yahoo.com/news/video/yahoo-trust-unconference-tls-adam-223046696.html
[cryptopasta]: https://github.com/gtank/cryptopasta

## Elliptic Curves vs. RSA

Even though elliptic curves are understood to be a modern and superior choice for asymmetric cryptography, many developers end up using RSA for familiarity. Indeed, this post originally used RSA before its revamp.

Unless there’s concern about compatibility with existing systems, use elliptic curve keys instead of RSA. Elliptic curves are both smaller and [more performant][rsa-certs], and even if a certificate uses an RSA key, the handshake should use elliptic curves to establish a shared key for forward secrecy (cipher suites with “ECDHE” in the name).

[rsa-certs]: https://github.com/golang/go/issues/20058

# X509 Certificates

A TLS certificate is a public key with additional information like “this key can be used to serve traffic on ‘example.com’,” or “this certificate is valid for 6 months.” Using a certificate requires the private key.

## Generating a CA

The role of a certificate authority is to sign other certificates. Clients trust the CA and subsequently trust any certificates the CA signs. This prevents clients from having to individually trust each certificate in the system.

Go programs can use [`crypto/x509.CreateCertificate`][create-certificate] to generate a certificate. This takes a template of the certificate to create, the parent certificate, the certificate’s public key, and the parent certificate’s private key.

Since there’s no other key yet, the CA uses its own key to self-sign the certificate. The important fields in the template are `IsCA` and `BasicConstraintsValid`:

```go
{{% src "src/tls/certs.go" 21 70 %}}
```

This program prints familiar PEM encoded certificates:

```
-----BEGIN CERTIFICATE-----
MIIBRDCB66ADAgECAhAKqNAicZ1SD0Dmj/ngOs5zMAoGCCqGSM49BAMCMBIxEDAO
BgNVBAoTB0FjbWUgQ28wHhcNMTgwOTE2MjIyNTM1WhcNMTgwOTE2MjMyNTM1WjAS
MRAwDgYDVQQKEwdBY21lIENvMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEbgBk
maJipMszxOoyrIibrSDk6mx3yo5Xabzvk3l3fEA64Td5nMhHVBGWe2KV04IWTM9a
BvHrfkF+MGQ9X//dNaMjMCEwDgYDVR0PAQH/BAQDAgKkMA8GA1UdEwEB/wQFMAMB
Af8wCgYIKoZIzj0EAwIDSAAwRQIgPclaygGOaORV8/Av20OQUYYctSyPUvcdIem6
WHf1vekCIQCoREcl7sNV5t2+MJXQyVhMOXN/m//bUom56t2eNQWi2g==
-----END CERTIFICATE-----
-----BEGIN EC PRIVATE KEY-----
MHcCAQEEILeoo0r304KnBJT8/TrX5AXzFx8P5TuY6jt4rYECria0oAoGCCqGSM49
AwEHoUQDQgAEbgBkmaJipMszxOoyrIibrSDk6mx3yo5Xabzvk3l3fEA64Td5nMhH
VBGWe2KV04IWTM9aBvHrfkF+MGQ9X//dNQ==
-----END EC PRIVATE KEY-----
```

[create-certificate]: https://golang.org/pkg/crypto/x509/#CreateCertificate

## Signing a serving certificate

The next thing we’ll do is generate a serving certificate.

The serving certificate is signed by the CA, so we pass the CA’s private key to the `CreateCertificate` call, 

```go
{{% src "src/tls/certs.go" 72 101 %}}
```

## Serving TLS

Servers and clients use [`crypto/tls.Config`][tls-config] to control which certificates they use and trust. The default config is intended for publicly distributed certificates, so since we’ve generated our own we’ll need to customize it a little.

The [`crypto/tls.X509KeyPair`][x509-key-pair] (and [LoadX509KeyPair][load-x509-key-pair]) can be used to convert the certificate we generated earlier to something the `crypto/tls` package understands. The HTTP server then uses this as its sole certificate.

The client is then configured to trust the certificate authority by trusting the CA’s certificate using the `RootCAs` field. Finally, our client makes an encrypted request over HTTPS to the server:

```go
{{% src "src/tls/certs.go" 103 150 %}}
```

[tls-config]: https://golang.org/pkg/crypto/tls/#Config
[x509-key-pair]: https://golang.org/pkg/crypto/tls/#X509KeyPair
[load-x509-key-pair]: https://golang.org/pkg/crypto/tls/#LoadX509KeyPair

## Client certificates

While the client authenticates the server in our previous example, the server still accepts requests from any client.

The process looks similar to generating a server certificate, except the `ExtKeyUsage` holds `ExtKeyUsageClientAuth` instead of `ExtKeyUsageServerAuth`:

```go
{{% src "src/tls/certs.go" 152 179 %}}
```

This time, the client also includes its own certificate in its `tls.Config`. Server uses `ClientCAs` to trust the certificate authority for client certificates and `RequireAndVerifyClientCert` for its `ClientAuth` policy:

```go
{{% src "src/tls/certs.go" 181 226 %}}
```

# Public Key Infrastructure

Public Key Infrastructure (PKI) is a loose term for how certificates are distributed within a environment. “PKI” can be a bash script calling OpenSSL, or centralized service like Let’s Encrypt. Modern PKI is usually just an API for clients use to request a certificate.

What makes PKI hard is real world stacks have different strategies for authenticating and authorizing users, applications, and infrastructure.

## Certificate signing requests

The previous examples all assumed that one system has access to both the CA’s and server’s private keys. In the real world, there’s no way a CA would another party access to its private key, and clients should never hand over their server’s private key either.

The client generates a key locally and signs a CSR:

```go
{{% src "src/tls/csr.go" 24 37 %}}
```

```go
{{% src "src/tls/csr.go" 67 95 %}}
```

CSRs omit many fields that an actual certificate holds. For example, it’s not possible to specify if the certificate should be a client or serving certificate.

## Certificate revocation lists

```go
{{% src "src/tls/crl.go" 103 110 %}}
```

```go
{{% src "src/tls/crl.go" 112 142 %}}
```

```go
{{% src "src/tls/crl.go" 166 170 %}}
```

```
2018/11/24 15:00:16 GET failed: Get https://127.0.0.1:8443/: certificate was revoked: /cn=my-server
2018/11/24 15:00:16 http: TLS handshake error from 127.0.0.1:53763: remote error: tls: bad certificate
```

```go
{{% src "src/tls/ocsp.go" 105 113 %}}
```

```go
{{% src "src/tls/ocsp.go" 115 137 %}}
```

```go
{{% src "src/tls/ocsp.go" 139 156 %}}
```

## Rotating a CA

TLS configs are designed to trust multiple certificates. To rotate a certificate generate a new certificate then include both the old and new certificate in the tls.Config.RootCAs and tls.Config.ClientCAs:

```go
{{% src "src/tls/rotate-ca.go" 12 28 %}}
```

Once all services have the new CA certificate, certificates signed by the new CA will be trusted.

## SNI

Server Name Identification is a way for an endpoint to host certificates for multiple domains. For example, a load balancer will frequently serve multiple sites and needs to provide the correct certificate for each request. To accommodate this, clients include the domain they’re attempting to connect to as part of the handshake using the SNI extension.

Both clients and servers automatically enable this in Go; a `tls.Config` with multiple certificates will automatically use the cert for the correct host when a client connects. For more customized behavior, such as looking up a certificate dynamically, use the `GetCertificate` callback:

```go
{{% src "src/tls/sni.go" 15 42 %}}
```

SNI brings additional challenges because the domain name is sent in plain text. This allows snoopers, such as firewalls, to inspect the domain being connected to. Though there’s a strong push towards [encrypted SNI][encrypted-sni] with Cloudflare and [FireFox][encrypted-sni-firefox] adopting the spec, as of this post Go doesn’t support encrypted SNI or the extensions that would allow an external package to provide it.

Real world SNI implementations have had interesting interactions with shared hosting environments, including hacks like [domain fronting][domain-fronting] and attacks on Let’s Encrypt’s now disabled [TLS SNI challenge][tls-sni-challenge].

[encrypted-sni]: https://blog.cloudflare.com/encrypted-sni/
[encrypted-sni-firefox]: https://blog.mozilla.org/security/2018/10/18/encrypted-sni-comes-to-firefox-nightly/
[domain-fronting]: https://en.wikipedia.org/wiki/Domain_fronting
[tls-sni-challenge]: https://community.letsencrypt.org/t/2018-01-09-issue-with-tls-sni-01-and-shared-hosting-infrastructure/49996

## Hardware bound keys

Generating private keys on hardware makes those keys unexportable, requiring user interaction or access to a machine to use. While a FIPS compliant HSM might be overkill for most users, TPMs and security keys are common hardware that can accomplish something similar.

The following code uses [pault.ag/go/ykpiv][ykpiv] to generate a private key on a YubiKey, sign a CSR, and serving HTTPs traffic with the signed certificate:

```go
{{% src "src/tls/yubikey.go" 31 63 %}}
```

```go
{{% src "src/tls/yubikey.go" 136 158 %}}
```

In cloud environments Key Management Services (KMS) can provide similar properties, exposing signing capabilities to clients without exposing the actual key.

[ykpiv]: https://github.com/paultag/go-ykpiv

# HTTPS

## Secure Cookies

```go
{{% src "src/tls/cookies.go" 9 18 %}}
```

## HSTS

HTTP Strict Transport Security is a way for a server to indicate that it only intends to communicate over HTTPS. After receiving an HSTS response header, if a user attempts to visit that domain using HTTP, the browser automatically converts the URL to HTTPS.

The following `net/http` middleware adds an HSTS header to all responses:

```go
{{% src "src/tls/hsts.go" 6 13 %}}
```

Because HSTS headers don’t protect against the first time a user visits a website, browsers ship with their own preloaded list of domains known to operate only over HTTPS. Sites that meet a certain criteria can apply to be included in this list through [hstspreload.org](https://hstspreload.org/). 

## Let's Encrypt

Let’s Encrypt is an amazing certificate authority that popped up around 2015 with the mission to issue automated, free, publicly trusted certificates. If you’re looking for a certificate for your domain, just use Let’s Encrypt.

When issuing a certificate, Let’s Encrypt challenges a server to prove it controls a domain and the [golang.org/x/crypto/acme/autocert][autocert] package instruments a server to initiate and respond to those challenges. `autocert` stores the Let’s Encrypt API keys and certificates in a cache, providing a `DirCache` helper, though scalable apps will want to write a cache implementation that uses a secret store.

```go
{{% src "src/tls/letsencrypt.go" 12 27 %}}
```

[autocert]: https://godoc.org/golang.org/x/crypto/acme/autocert

## Certificate Transparency

[Certificate Transparency][ct] is infrastructure for recording all certificates issued by publicly trusted certificate authorities. New certificates must be disclosed via Certificate Transparency [to be trusted by Chrome][ct-chrome], and websites can assert that they’re using a logged cert on other browsers with the `[Expect-CT][expect-ct]` HTTP header. Users can query Certificate Transparency logs to determine whenever a publically trusted certificate is issued for a domain. Google maintains a Go client `[github.com/google/certificate-transparency-go][ct-go]` that can be used to query entries:

```go
{{% src "src/tls/certificate-transparency.go" 15 41 %}}
```

When run, this returns the following entries:

```
number of entries at 2018-11-22 14:06:55.552 -0800 PST: 439070912
index: 0
  cn: ttmail.npp.co.th
  timestamp: 2013-03-25 17:09:58.992 -0700 PDT
  dns names: [ttmail.npp.co.th www.ttmail.npp.co.th]
index: 1
  cn: mail.google.com
  timestamp: 2013-03-25 17:37:32.899 -0700 PDT
  dns names: [mail.google.com]
index: 2
  cn: www.struleartscentre.purchase-tickets-online.co.uk
  timestamp: 2013-03-26 02:00:07.155 -0700 PDT
  dns names: [www.struleartscentre.purchase-tickets-online.co.uk]
index: 3
  cn: www.netkeiba.com
  timestamp: 2013-03-26 02:00:07.155 -0700 PDT
  dns names: [www.netkeiba.com]
index: 4
  cn: www.oxfordplayhouse.com
  timestamp: 2013-03-26 02:00:07.156 -0700 PDT
  dns names: [www.oxfordplayhouse.com oxfordplayhouse.com]
```

[ct]: https://www.certificate-transparency.org/
[ct-chrome]: https://chromium.googlesource.com/chromium/src/+/master/net/docs/certificate-transparency.md
[ct-go]: https://github.com/google/certificate-transparency-go
[expect-ct]: https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Expect-CT

# TLS in practice

## Who's responsible for TLS?

## Best practices for apps

## Open source PKI
