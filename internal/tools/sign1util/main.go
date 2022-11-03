package main

import (
	"flag"
	"os"

	"log"

	"github.com/Microsoft/hcsshim/pkg/cosesign1"
	"github.com/veraison/go-cose"
)

func checkCoseSign1(inputFilename string, optionalPubKeyFilename string, rootCAFile string, requireKnownAuthority bool, verbose bool) (cosesign1.UnpackedCoseSign1, error) {
	coseBlob := cosesign1.ReadBlob(inputFilename)
	var optionalPubKeyPEM []byte
	if optionalPubKeyFilename != "" {
		optionalPubKeyPEM = cosesign1.ReadBlob(optionalPubKeyFilename)
	}

	var optionalRootCAPEM []byte
	if rootCAFile != "" {
		optionalRootCAPEM = cosesign1.ReadBlob(rootCAFile)
	}

	var unpacked cosesign1.UnpackedCoseSign1
	var err error
	unpacked, err = cosesign1.UnpackAndValidateCOSE1CertChain(coseBlob, optionalPubKeyPEM, optionalRootCAPEM, requireKnownAuthority, verbose)
	if err != nil {
		log.Print("checkCoseSign1 failed - " + err.Error())
	} else {
		log.Print("checkCoseSign1 passed:")
		if verbose {
			log.Printf("iss:\n%s\n", unpacked.Issuer) // eg the DID:x509:blah....
			log.Printf("feed: %s", unpacked.Feed)
			log.Printf("cty: %s", unpacked.ContentType)
			log.Printf("pubkey: %s", unpacked.Pubkey)
			log.Printf("pubcert: %s", unpacked.Pubcert)
			log.Printf("payload:\n%s\n", string(unpacked.Payload[:]))
		}
	}
	return unpacked, err
}

func createCoseSign1(payloadFilename string, issuer string, feed string, contentType string, chainFilename string, keyFilename string, saltType string, algo cose.Algorithm, verbose bool) ([]byte, error) {

	var payloadBlob = cosesign1.ReadBlob(payloadFilename)
	var keyPem = cosesign1.ReadBlob(keyFilename)
	var chainPem = cosesign1.ReadBlob(chainFilename)

	return cosesign1.CreateCoseSign1(payloadBlob, issuer, feed, contentType, chainPem, keyPem, saltType, algo, verbose)
}

// example scitt usage to try tro match
// scitt sign --claims <fragment>.rego --content-type application/unknown+json --did-doc ~/keys/did.json --key ~/keys/key.pem --out <fragment>.cose
func main() {
	var payloadFilename string
	var contentType string
	var chainFilename string
	var keyFilename string
	var rootCAFile string
	var outputFilename string
	var outputCertFilename string
	var outputKeyFilename string
	var inputFilename string
	var saltType string
	var requireKNownAuthority bool
	var verbose bool
	var algo string
	var feed string
	var issuer string
	var didPolicy string
	var didFingerprintIndex int
	var didFingerprintAlgorithm string

	createCmd := flag.NewFlagSet("create", flag.ExitOnError)

	createCmd.StringVar(&payloadFilename, "claims", "fragment.rego", "filename of payload")
	createCmd.StringVar(&contentType, "content-type", "application/unknown+json", "content type, eg appliation/json")
	createCmd.StringVar(&chainFilename, "chain", "chain.pem", "key or cert file to use (pem)")
	createCmd.StringVar(&keyFilename, "key", "key.pem", "key to sign with (private key of the leaf of the chain)")
	createCmd.StringVar(&outputFilename, "out", "out.cose", "output file")
	createCmd.StringVar(&saltType, "salt", "rand", "rand or zero")
	createCmd.StringVar(&algo, "algo", "PS384", "PS256, PS384 etc")
	createCmd.StringVar(&issuer, "issuer", "", "the party making the claims") // see https://ietf-scitt.github.io/draft-birkholz-scitt-architecture/draft-birkholz-scitt-architecture.html#name-terminology
	createCmd.StringVar(&feed, "feed", "", "identifier for an artifact within the scope of an issuer")
	createCmd.BoolVar(&verbose, "verbose", false, "verbose output")

	checkCmd := flag.NewFlagSet("check", flag.ExitOnError)

	checkCmd.StringVar(&inputFilename, "in", "input.cose", "input file")
	checkCmd.StringVar(&keyFilename, "pub", "", "input public key (PEM)")
	checkCmd.StringVar(&rootCAFile, "root", "", "(trusted) root CA certificate filename (PEM)")
	checkCmd.BoolVar(&requireKNownAuthority, "requireKNownAuthority", false, "false => allow chain validation to fail")
	checkCmd.BoolVar(&verbose, "verbose", false, "verbose output")

	printCmd := flag.NewFlagSet("print", flag.ExitOnError)

	printCmd.StringVar(&inputFilename, "in", "input.cose", "input file")
	printCmd.StringVar(&rootCAFile, "root", "", "(trusted) root CA certificate filename (PEM)")

	leafCmd := flag.NewFlagSet("leaf", flag.ExitOnError)

	leafCmd.StringVar(&inputFilename, "in", "input.cose", "input file")
	leafCmd.StringVar(&outputKeyFilename, "keyout", "leafkey.pem", "leaf key output file")
	leafCmd.StringVar(&outputCertFilename, "certout", "leafcert.pem", "leaf cert output file")
	leafCmd.BoolVar(&verbose, "verbose", false, "verbose output")

	didX509Cmd := flag.NewFlagSet("did:x509", flag.ExitOnError)

	didX509Cmd.StringVar(&didFingerprintAlgorithm, "fingerprint-algorithm", "sha256", "hash algorithm for certificate fingerprints")
	didX509Cmd.StringVar(&chainFilename, "chain", "chain.pem", "certificate chain to use (pem)")
	didX509Cmd.IntVar(&didFingerprintIndex, "i", 0, "index of the certificate fingerprint in the chain")
	didX509Cmd.StringVar(&didPolicy, "policy", "", "did:509 policy")
	didX509Cmd.BoolVar(&verbose, "verbose", false, "verbose output")

	if len(os.Args) > 1 {
		action := os.Args[1]
		switch action {
		case "create":
			err := createCmd.Parse(os.Args[2:])
			if err == nil {
				algorithm, err := cosesign1.StringToAlgorithm(algo)
				var raw []byte
				if err == nil {
					raw, err = createCoseSign1(payloadFilename, issuer, feed, contentType, chainFilename, keyFilename, saltType, algorithm, verbose)
				}

				if err != nil {
					log.Print("failed create: " + err.Error())
				} else {
					if len(outputFilename) > 0 {
						err = cosesign1.WriteBlob(outputFilename, raw)
						if err != nil {
							log.Printf("writeBlob failed for %s\n", outputFilename)
						}
					}
				}
			} else {
				log.Print("args parse failed: " + err.Error())
			}

		case "check":
			err := checkCmd.Parse(os.Args[2:])
			if err == nil {
				_, err := checkCoseSign1(inputFilename, keyFilename, rootCAFile, requireKNownAuthority, verbose)
				if err != nil {
					log.Print("failed check: " + err.Error())
				}
			} else {
				log.Print("args parse failed: " + err.Error())
			}

		case "print":
			err := printCmd.Parse(os.Args[2:])
			if err == nil {
				_, err := checkCoseSign1(inputFilename, "", rootCAFile, false, true)
				if err != nil {
					log.Print("failed print: " + err.Error())
				}
			} else {
				log.Print("args parse failed: " + err.Error())
			}

		case "leaf":
			err := leafCmd.Parse(os.Args[2:])
			if err == nil {
				unpacked, err := checkCoseSign1(inputFilename, "", rootCAFile, false, verbose)
				if err == nil {
					err = cosesign1.WriteString(outputKeyFilename, unpacked.Pubkey)
					if err != nil {
						log.Printf("writing the leaf pub key to %s failed: %s", outputKeyFilename, err.Error())
					} else {
						err = cosesign1.WriteString(outputCertFilename, unpacked.Pubcert)
						if err != nil {
							log.Printf("writing the leaf cert to %s failed: %s", outputCertFilename, err.Error())
						}
					}
				} else {
					log.Printf("reading the COSE Sign1 from %s failed: %s", inputFilename, err.Error())
				}
			} else {
				log.Print("args parse failed: " + err.Error())
			}

		case "did:x509":
			err := didX509Cmd.Parse(os.Args[2:])
			if err == nil {
				r, err := cosesign1.MakeDidX509(didFingerprintAlgorithm, didFingerprintIndex, chainFilename, didPolicy, verbose)
				if err != nil {
					log.Print("error: " + err.Error())
				} else {
					print(r + "\n")
				}
			} else {
				log.Print("args parse failed: " + err.Error())
			}

		default:
			os.Stderr.WriteString("Usage: sign1util [create|check|print|leafkey|did:x509] -h\n")
		}

	} else {
		os.Stderr.WriteString("Usage: sign1util [create|check|print|leafkey|did:x509] -h\n")
	}
}
