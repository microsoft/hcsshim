package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/Microsoft/hcsshim/ext4/tar2ext4"
)

var (
	input  = flag.String("i", "", "input file")
	output = flag.String("o", "", "output file")
)

func main() {
	flag.Parse()
	if flag.NArg() != 0 || len(*output) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	err := func() (err error) {
		in := os.Stdin
		if *input != "" {
			in, err = os.Open(*input)
			if err != nil {
				return err
			}
		}
		out, err := os.Create(*output)
		if err != nil {
			return err
		}

		_, err = io.Copy(out, in)
		if err != nil {
			return err
		}

		err = tar2ext4.ConvertToVhd(out)
		if err != nil {
			return err
		}

		// Exhaust the tar stream.
		_, _ = io.Copy(io.Discard, in)
		return nil
	}()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}