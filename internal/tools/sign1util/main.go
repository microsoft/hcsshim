package main

import (
	"flag"
	"os"
	"log"
	"github.com/Microsoft/hcsshim/internal/cosesign1"
	didx509resolver "github.com/Microsoft/hcsshim/internal/did-x509-resolver"
)

func checkCoseSign1(inputFilename string, optionalPubKeyFilename string, rootCAFile string, chainFilename string, didString string, verbose bool) (cosesign1.UnpackedCoseSign1, error) {
	coseBlob := cosesign1.ReadBlob(inputFilename)
	var optionalPubKeyPEM []byte
	if optionalPubKeyFilename != "" {
		optionalPubKeyPEM = cosesign1.ReadBlob(optionalPubKeyFilename)
	}

	var optionalRootCAPEM []byte
	if rootCAFile != "" {
		optionalRootCAPEM = cosesign1.ReadBlob(rootCAFile)
	}

	var chainPEM []byte
	var chainPEMString string
	if chainFilename != "" {
		chainPEM = cosesign1.ReadBlob(chainFilename)
		chainPEMString = string(chainPEM[:])
	}

	var unpacked cosesign1.UnpackedCoseSign1
	var err error
	unpacked, err = cosesign1.UnpackAndValidateCOSE1CertChain(coseBlob, optionalPubKeyPEM, optionalRootCAPEM, verbose)
	if err != nil {
		log.Print("checkCoseSign1 failed - " + err.Error())
	} else {
		log.Print("checkCoseSign1 passed:")
		if verbose {
			log.Printf("iss: %s", unpacked.Issuer)
			log.Printf("feed: %s", unpacked.Feed)
			log.Printf("cty: %s", unpacked.ContentType)
			log.Printf("pubkey: %s", unpacked.Pubkey)
			log.Printf("pubcert: %s", unpacked.Pubcert)
			log.Printf("payload:\n%s\n", string(unpacked.Payload[:]))
		}
		if len(didString) > 0 {
			if len(chainPEMString) == 0 {
				chainPEMString = unpacked.ChainPem
			}
			didDoc, err := didx509resolver.Resolve(chainPEMString, didString, true)
			if err == nil {
				log.Printf("DID resolvers passed:\n%s\n", didDoc)
			} else {
				log.Printf("DID resolvers failed: err: %s doc:\n%s\n", err.Error(), didDoc)
			}
		}

	}
	return unpacked, err
}

func createCoseSign1(payloadFilename string, issuer string, feed string, contentType string, chainFilename string, keyFilename string, saltType string, algo string, verbose bool) ([]byte, error) {

	var payloadBlob = cosesign1.ReadBlob(payloadFilename)
	var keyPem = cosesign1.ReadBlob(keyFilename)
	var chainPem = cosesign1.ReadBlob(chainFilename)
	algorithm, err := cosesign1.StringToAlgorithm(algo)
	if err != nil {
		return nil, err
	}

	return cosesign1.CreateCoseSign1(payloadBlob, issuer, feed, contentType, chainPem, keyPem, saltType, algorithm, verbose)
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
	var verbose bool
	var algo string
	var feed string
	var issuer string
	var didPolicy string
	var didString string
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
	checkCmd.StringVar(&chainFilename, "chain", "chain.pem", "key or cert file to use (pem)")
	checkCmd.StringVar(&didString, "did", "", "DID x509 string to resolve against cert chain")
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
	didX509Cmd.IntVar(&didFingerprintIndex, "i", 1, "index of the certificate fingerprint in the chain")
	didX509Cmd.StringVar(&didPolicy, "policy", "subject", "did:509 policy (cn/eku/custom)")
	didX509Cmd.BoolVar(&verbose, "verbose", false, "verbose output")

	chainCmd := flag.NewFlagSet("chain", flag.ExitOnError)
	chainCmd.StringVar(&inputFilename, "in", "input.cose", "input file")

	if len(os.Args) > 1 {
		action := os.Args[1]
		switch action {
		case "create":
			err := createCmd.Parse(os.Args[2:])
			if err == nil {
				var raw []byte
				if err == nil {
					raw, err = createCoseSign1(payloadFilename, issuer, feed, contentType, chainFilename, keyFilename, saltType, algo, verbose)
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
				_, err := checkCoseSign1(inputFilename, keyFilename, rootCAFile, chainFilename, didString, verbose)
				if err != nil {
					log.Print("failed check: " + err.Error())
				}
			} else {
				log.Print("args parse failed: " + err.Error())
			}

		case "print":
			err := printCmd.Parse(os.Args[2:])
			if err == nil {
				_, err := checkCoseSign1(inputFilename, "", rootCAFile, chainFilename, didString, true)
				if err != nil {
					log.Print("failed print: " + err.Error())
				}
			} else {
				log.Print("args parse failed: " + err.Error())
			}

		case "leaf":
			err := leafCmd.Parse(os.Args[2:])
			if err == nil {
				unpacked, err := checkCoseSign1(inputFilename, "", rootCAFile, chainFilename, didString, verbose)
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

		case "chain":
			err := chainCmd.Parse(os.Args[2:])
			if err == nil {
				err := cosesign1.PrintChain(inputFilename)
				if err != nil {
					log.Print("error: " + err.Error())
				}
			}

		default:
			os.Stderr.WriteString("Usage: sign1util [create|check|print|leafkey|did:x509] -h\n")
		}

	} else {
		os.Stderr.WriteString("Usage: sign1util [create|check|print|leafkey|did:x509] -h\n")
	}
}
