//go:build linux
// +build linux

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Microsoft/hcsshim/internal/guest/amdsev"
	"github.com/Microsoft/hcsshim/internal/tools/snp-report/fake"
)

func main() {
	fakeReportFlag := flag.Bool(
		"fake-report",
		false,
		"If true, don't issue an actual syscall to /dev/sev and return a fake predefined report",
	)
	hostDataFlag := flag.String(
		"host-data",
		"",
		"Use together with 'fake-report', to set 'HostData' field of fake SNP report.",
	)
	reportDataFlag := flag.String(
		"report-data",
		"",
		"Report data to use when fetching SNP attestation report",
	)
	binaryFmtFlag := flag.Bool(
		"binary",
		false,
		"Fetch report in binary format",
	)
	verbosePrintFlag := flag.Bool(
		"verbose",
		false,
		"Print report in a prettier format",
	)

	flag.Parse()

	if *binaryFmtFlag {
		var binaryReport []byte
		var err error
		if !*fakeReportFlag {
			binaryReport, err = fake.FetchRawSNPReport()
		} else {
			binaryReport, err = amdsev.FetchRawSNPReport(*reportDataFlag)
		}
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Printf("%x\n", binaryReport)
		os.Exit(0)
	}

	var report amdsev.Report
	var err error
	if *fakeReportFlag {
		report, err = fake.FetchSNPReport(*hostDataFlag)
	} else {
		report, err = amdsev.FetchParsedSNPReport(*reportDataFlag)
	}
	if err != nil {
		fmt.Printf("failed to fetch SNP report: %s", err)
		os.Exit(1)
	}

	if !*verbosePrintFlag {
		fmt.Printf("%+v\n", report)
	} else {
		fmt.Println(report.PrettyString())
	}
}
