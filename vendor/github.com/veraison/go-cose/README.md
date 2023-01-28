# go-cose

[![go.dev](https://pkg.go.dev/badge/github.com/veraison/go-cose.svg)](https://pkg.go.dev/github.com/veraison/go-cose)
[![tests](https://github.com/veraison/go-cose/workflows/ci/badge.svg)](https://github.com/veraison/go-cose/actions?query=workflow%3Aci)
[![codecov](https://codecov.io/gh/veraison/go-cose/branch/main/graph/badge.svg?token=SL18TCTC03)](https://codecov.io/gh/veraison/go-cose)

A golang library for the [COSE specification][cose-spec]

## Project Status

**Current Release**: [go-cose alpha 1][release-alpha-1] 

The project was *initially* forked from the  upstream [mozilla-services/go-cose][mozilla-go-cose] project, however the Veraison and Mozilla maintainers have agreed to retire the mozilla-services/go-cose project and focus on [veraison/go-cose][veraison-go-cose] as the active project.

We thank the [Mozilla maintainers and contributors][mozilla-contributors] for their great work that formed the base of the [veraison/go-cose][veraison-go-cose] project.

## Installation

go-cose is compatible with modern Go releases in module mode, with Go installed:

```bash
go get github.com/veraison/go-cose
```

will resolve and add the package to the current development module, along with its dependencies.

Alternatively the same can be achieved if you use import in a package:

```go
import "github.com/veraison/go-cose"
```

and run `go get` without parameters.

Finally, to use the top-of-trunk version of this repo, use the following command:

```bash
go get github.com/veraison/go-cose@main
```

## Usage

```go
import "github.com/veraison/go-cose"
```

Construct a new COSE_Sign1 message, then sign it using ECDSA w/ SHA-512 and finally marshal it. For example:

```go
// create a signer
privateKey, _ := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
signer, _ := cose.NewSigner(cose.AlgorithmES512, privateKey)

// create message header
headers := cose.Headers{
    Protected: cose.ProtectedHeader{
        cose.HeaderLabelAlgorithm: cose.AlgorithmES512,
    },
}

// sign and marshal message
sig, _ := cose.Sign1(rand.Reader, signer, headers, []byte("hello world"), nil)
```

Verify a raw COSE_Sign1 message. For example:

```go
// create a verifier from a trusted private key
publicKey := privateKey.Public()
verifier, _ := cose.NewVerifier(cose.AlgorithmES512, publicKey)

// create a sign message from a raw COSE_Sign1 payload
var msg cose.Sign1Message
_ = msg.UnmarshalCBOR(raw)
_ = msg.Verify(nil, verifier)
```

## Features

### Signing and Verifying Objects

go-cose supports two different signature structures:
- [cose.Sign1Message](https://pkg.go.dev/github.com/veraison/go-cose#Sign1Message) implements [COSE_Sign1](https://datatracker.ietf.org/doc/html/rfc8152#section-4.2).
- [cose.SignMessage](https://pkg.go.dev/github.com/veraison/go-cose#SignMessage) implements [COSE_Sign](https://datatracker.ietf.org/doc/html/rfc8152#section-4.1).
> :warning: The COSE_Sign API is currently **EXPERIMENTAL** and may be changed or removed in a later release.  In addition, the amount of functional and security testing it has received so far is significantly lower than the COSE_Sign1 API.

### Built-in Algorithms

go-cose has built-in supports the following algorithms:
- PS{256,384,512}: RSASSA-PSS w/ SHA as defined in RFC 8230.
- ES{256,384,512}: ECDSA w/ SHA as defined in RFC 8152.
- Ed25519: PureEdDSA as defined in RFC 8152.

### Custom Algorithms

The supported algorithms can be extended at runtime by using [cose.RegisterAlgorithm](https://pkg.go.dev/github.com/veraison/go-cose#RegisterAlgorithm).

[API docs](https://pkg.go.dev/github.com/veraison/go-cose)

### Integer Ranges

CBOR supports integers in the range [-2<sup>64</sup>, -1] ∪ [0, 2<sup>64</sup> - 1].

This does not map onto a single Go integer type.

`go-cose` uses `int64` to encompass both positive and negative values to keep data sizes smaller and easy to use.

The main effect is that integer label values in the [-2<sup>64</sup>, -2<sup>63</sup> - 1] and the [2<sup>63</sup>, 2<sup>64</sup> - 1] ranges, which are nominally valid
per RFC 8152, are rejected by the go-cose library.

### Conformance Tests

go-cose runs the [GlueCOSE](https://github.com/gluecose/test-vectors) test suite on every local `go test` execution.
These are also executed on every CI job.

### Fuzz Tests

go-cose implements several fuzz tests using [Go's native fuzzing](https://go.dev/doc/fuzz).

Fuzzing requires Go 1.18 or higher, and can be executed as follows:

```bash
go test -fuzz=FuzzSign1
```

[cose-spec]:            https://datatracker.ietf.org/doc/draft-ietf-cose-rfc8152bis-struct/
[mozilla-contributors]: https://github.com/mozilla-services/go-cose/graphs/contributors
[mozilla-go-cose]:      http://github.com/mozilla-services/go-cose
[veraison-go-cose]:     https://github.com/veraison/go-cose
[release-alpha-1]:      https://github.com/veraison/go-cose/releases/tag/v1.0.0-alpha.1
