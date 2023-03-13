package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Microsoft/hcsshim/internal/cosesign1"
	didx509resolver "github.com/Microsoft/hcsshim/internal/did-x509-resolver"
	"github.com/urfave/cli"
)

func checkCoseSign1(inputFilename string, chainFilename string, didString string, verbose bool) (*cosesign1.UnpackedCoseSign1, error) {
	coseBlob, err := os.ReadFile(inputFilename)
	if err != nil {
		return nil, err
	}

	var chainPEM []byte
	var chainPEMString string
	if chainFilename != "" {
		chainPEM, err = os.ReadFile(chainFilename)
		if err != nil {
			return nil, err
		}
		chainPEMString = string(chainPEM[:])
	}

	unpacked, err := cosesign1.UnpackAndValidateCOSE1CertChain(coseBlob)
	if err != nil {
		fmt.Fprintf(os.Stdout, "checkCoseSign1 failed - %s\n", err)
		return nil, err
	}

	fmt.Fprint(os.Stdout, "checkCoseSign1 passed\n")
	if verbose {
		fmt.Fprintf(os.Stdout, "iss: %s\n", unpacked.Issuer)
		fmt.Fprintf(os.Stdout, "feed: %s\n", unpacked.Feed)
		fmt.Fprintf(os.Stdout, "cty: %s\n", unpacked.ContentType)
		fmt.Fprintf(os.Stdout, "pubkey: %s\n", unpacked.Pubkey)
		fmt.Fprintf(os.Stdout, "pubcert: %s\n", unpacked.Pubcert)
		fmt.Fprintf(os.Stdout, "payload:\n%s\n", string(unpacked.Payload[:]))
	}
	if len(didString) > 0 {
		if len(chainPEMString) == 0 {
			chainPEMString = unpacked.ChainPem
		}
		didDoc, err := didx509resolver.Resolve(chainPEMString, didString, true)
		if err == nil {
			fmt.Fprintf(os.Stdout, "DID resolvers passed:\n%s\n", didDoc)
		} else {
			fmt.Fprintf(os.Stdout, "DID resolvers failed: err: %s doc:\n%s\n", err.Error(), didDoc)
		}
	}
	return unpacked, err
}

var createCmd = cli.Command{
	Name:  "create",
	Usage: "",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "claims",
			Usage: "filename of payload",
			Value: "fragment.rego",
		},
		cli.StringFlag{
			Name:  "content-type",
			Usage: "payload content type",
			Value: "application/unknown+json",
		},
		cli.StringFlag{
			Name:  "chain",
			Usage: "key or cert file to use (pem)",
			Value: "chain.pem",
		},
		cli.StringFlag{
			Name:  "key",
			Usage: "key to sign with - private key of the leaf of the chain",
			Value: "key.pem",
		},
		cli.StringFlag{
			Name:     "algo",
			Usage:    "PS256, PS384 etc (required)",
			Required: true,
		},
		cli.StringFlag{
			Name:  "out",
			Usage: "output file (default: out.cose)",
			Value: "out.cose",
		},
		cli.StringFlag{
			Name:  "salt",
			Usage: "salt type [rand|zero] (default: rand)",
			Value: "rand",
		},
		cli.StringFlag{
			Name: "issuer",
			Usage: "the party making the claims (optional). See https://ietf-scitt.github." +
				"io/draft-birkholz-scitt-architecture/draft-birkholz-scitt-architecture.html#name-terminology",
		},
		cli.StringFlag{
			Name:  "feed",
			Usage: "identifier for an artifact within the scope of an issuer (optional)",
		},
		cli.BoolFlag{
			Name:  "verbose,v",
			Usage: "verbose output (optional)",
		},
	},
	Action: func(ctx *cli.Context) error {
		payloadBlob, err := os.ReadFile(ctx.String("claims"))
		if err != nil {
			return err
		}
		keyPem, err := os.ReadFile(ctx.String("key"))
		if err != nil {
			return err
		}
		chainPem, err := os.ReadFile(ctx.String("chain"))
		if err != nil {
			return err
		}
		algo, err := cosesign1.StringToAlgorithm(ctx.String("algo"))
		if err != nil {
			return err
		}

		raw, err := cosesign1.CreateCoseSign1(
			payloadBlob,
			ctx.String("issuer"),
			ctx.String("feed"),
			ctx.String("content-type"),
			chainPem,
			keyPem,
			ctx.String("salt"),
			algo,
		)
		if err != nil {
			return fmt.Errorf("create failed: %w", err)
		}

		err = cosesign1.WriteBlob(ctx.String("out"), raw)
		if err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		fmt.Fprint(os.Stdout, "create completed\n")
		return nil
	},
}

var checkCmd = cli.Command{
	Name:  "check",
	Usage: "",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "in",
			Usage: "input COSE Sign1 file (default: input.cose)",
			Value: "input.cose",
		},
		cli.StringFlag{
			Name:  "chain",
			Usage: "key or cert file to use (pem) (optional)",
		},
		cli.StringFlag{
			Name:  "did",
			Usage: "DID x509 string to resolve against cert chain (optional)",
		},
		cli.BoolFlag{
			Name:  "verbose",
			Usage: "verbose output (optional)",
		},
	},
	Action: func(ctx *cli.Context) error {
		_, err := checkCoseSign1(
			ctx.String("in"),
			ctx.String("chain"),
			ctx.String("did"),
			ctx.Bool("verbose"),
		)
		if err != nil {
			return fmt.Errorf("failed check: %w", err)
		}
		return nil
	},
}

var printCmd = cli.Command{
	Name:  "print",
	Usage: "",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "in",
			Usage: "input COSE Sign1 file",
			Value: "input.cose",
		},
	},
	Action: func(ctx *cli.Context) error {
		_, err := checkCoseSign1(ctx.String("in"), "", "", true)
		if err != nil {
			return fmt.Errorf("failed verbose checkCoseSign1: %w", err)
		}
		return nil
	},
}

var leafCmd = cli.Command{
	Name:  "leaf",
	Usage: "",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "in",
			Usage: "input COSE Sign1 file",
			Value: "input.cose",
		},
		cli.StringFlag{
			Name:  "keyout",
			Usage: "leaf key output file",
			Value: "leafkey.pem",
		},
		cli.StringFlag{
			Name:  "certout",
			Usage: "leaf cert output file",
			Value: "leafcert.pem",
		},
		cli.BoolFlag{
			Name:  "verbose",
			Usage: "print information about COSE Sign1 document",
		},
	},
	Action: func(ctx *cli.Context) error {
		inputFilename := ctx.String("in")
		outputKeyFilename := ctx.String("keyout")
		outputCertFilename := ctx.String("certout")
		unpacked, err := checkCoseSign1(
			inputFilename,
			"",
			"",
			ctx.Bool("verbose"),
		)
		if err != nil {
			return fmt.Errorf("reading the COSE Sign1 from %s failed: %w", inputFilename, err)
		}

		// fixme(maksiman): instead of just printing the error, consider returning
		// it right away and skipping cert writing.
		keyWriteErr := cosesign1.WriteString(outputKeyFilename, unpacked.Pubkey)
		if keyWriteErr != nil {
			fmt.Fprintf(os.Stderr, "writing the leaf pub key to %s failed: %s\n", outputKeyFilename, keyWriteErr)
		}
		certWriteErr := cosesign1.WriteString(outputCertFilename, unpacked.Pubcert)
		if certWriteErr != nil {
			fmt.Fprintf(os.Stderr, "writing the leaf cert to %s failed: %s", outputCertFilename, certWriteErr)
		}

		var retErr error
		if keyWriteErr != nil {
			retErr = fmt.Errorf("key write failed: %s", retErr)
		}
		if certWriteErr != nil {
			if retErr != nil {
				return fmt.Errorf("cert write failed: %s: %s", certWriteErr, retErr)
			}
			return fmt.Errorf("cert write failed: %s", certWriteErr)
		}
		return nil
	},
}

var didX509Cmd = cli.Command{
	Name:  "did-x509",
	Usage: "",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "in",
			Usage: "input file",
		},
		cli.StringFlag{
			Name:  "fingerprint-algorithm",
			Usage: "hash algorithm for certificate fingerprints",
			Value: "sha256",
		},
		cli.StringFlag{
			Name:  "chain",
			Usage: "certificate chain to use (pem)",
		},
		cli.IntFlag{
			Name:  "index, i",
			Usage: "index of the certificate fingerprint in the chain",
			Value: 1,
		},
		cli.StringFlag{
			Name:  "policy",
			Usage: "did:509 policy, can be one of [cn|eku|custom]",
			Value: "cn",
		},
	},
	Action: func(ctx *cli.Context) error {
		chainFilename := ctx.String("chain")
		inputFilename := ctx.String("in")
		if len(chainFilename) > 0 && len(inputFilename) > 0 {
			return fmt.Errorf("cannot specify chain with cose file - it comes from the chain in the file")
		}
		var chainPEM string
		if len(chainFilename) > 0 {
			chainPEMBytes, err := os.ReadFile(chainFilename)
			if err != nil {
				return err
			}
			chainPEM = string(chainPEMBytes)
		}
		if len(inputFilename) > 0 {
			unpacked, err := checkCoseSign1(inputFilename, "", "", true)
			if err != nil {
				return err
			}
			chainPEM = unpacked.ChainPem
		}
		r, err := cosesign1.MakeDidX509(
			ctx.String("fingerprint-algorithm"),
			ctx.Int("index"),
			chainPEM,
			ctx.String("policy"),
			ctx.Bool("verbose"),
		)
		if err != nil {
			return fmt.Errorf("failed make DID: %w", err)
		}
		fmt.Fprint(os.Stdout, r)
		return nil
	},
}

var chainCmd = cli.Command{
	Name:  "chain",
	Usage: "",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "in",
			Usage: "input COSE Sign1 file",
			Value: "input.cose",
		},
		cli.StringFlag{
			Name:  "out",
			Usage: "output chain PEM text file",
		},
	},
	Action: func(ctx *cli.Context) error {
		pems, err := cosesign1.ParsePemChain(ctx.String("in"))
		if err != nil {
			return err
		}
		if len(ctx.String("out")) > 0 {
			return cosesign1.WriteString(ctx.String("out"), strings.Join(pems, "\n"))
		} else {
			fmt.Fprintf(os.Stdout, "%s\n", strings.Join(pems, "\n"))
			return nil
		}
	},
}

func main() {
	app := cli.NewApp()
	app.Name = "sign1util"
	app.Commands = []cli.Command{
		createCmd,
		checkCmd,
		printCmd,
		leafCmd,
		didX509Cmd,
		chainCmd,
	}

	if err := app.Run(os.Args); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
