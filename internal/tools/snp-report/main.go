//go:build linux
// +build linux

package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"

	"github.com/Microsoft/hcsshim/internal/tools/snp-report/fake"
	"github.com/Microsoft/hcsshim/pkg/amdsevsnp"
)

// verboseReport returns formatted attestation report.
func verboseReport(r amdsevsnp.Report) string {
	fieldNameFmt := "%-20s"
	pretty := ""
	pretty += fmt.Sprintf(fieldNameFmt+"%08x\n", "Version", r.Version)
	pretty += fmt.Sprintf(fieldNameFmt+"%08x\n", "GuestSVN", r.GuestSVN)
	pretty += fmt.Sprintf(fieldNameFmt+"%016x\n", "Policy", r.Policy)
	pretty += fmt.Sprintf(fieldNameFmt+"%s\n", "FamilyID", r.FamilyID)
	pretty += fmt.Sprintf(fieldNameFmt+"%s\n", "ImageID", r.ImageID)
	pretty += fmt.Sprintf(fieldNameFmt+"%08x\n", "VMPL", r.VMPL)
	pretty += fmt.Sprintf(fieldNameFmt+"%08x\n", "SignatureAlgo", r.SignatureAlgo)
	pretty += fmt.Sprintf(fieldNameFmt+"%016x\n", "PlatformVersion", r.PlatformVersion)
	pretty += fmt.Sprintf(fieldNameFmt+"%016x\n", "PlatformInfo", r.PlatformInfo)
	pretty += fmt.Sprintf(fieldNameFmt+"%08x\n", "AuthorKeyEn", r.AuthorKeyEn)
	pretty += fmt.Sprintf(fieldNameFmt+"%s\n", "ReportData", r.ReportData)
	pretty += fmt.Sprintf(fieldNameFmt+"%s\n", "Measurement", r.Measurement)
	pretty += fmt.Sprintf(fieldNameFmt+"%x\n", "HostData", r.HostData)
	pretty += fmt.Sprintf(fieldNameFmt+"%s\n", "IDKeyDigest", r.IDKeyDigest)
	pretty += fmt.Sprintf(fieldNameFmt+"%s\n", "AuthorKeyDigest", r.AuthorKeyDigest)
	pretty += fmt.Sprintf(fieldNameFmt+"%s\n", "ReportID", r.ReportID)
	pretty += fmt.Sprintf(fieldNameFmt+"%s\n", "ReportIDMA", r.ReportIDMA)
	pretty += fmt.Sprintf(fieldNameFmt+"%016x\n", "ReportTCB", r.ReportTCB)
	pretty += fmt.Sprintf(fieldNameFmt+"%s\n", "ChipID", r.ChipID)
	pretty += fmt.Sprintf(fieldNameFmt+"%s\n", "CommittedSVN", r.CommittedSVN)
	pretty += fmt.Sprintf(fieldNameFmt+"%s\n", "CommittedVersion", r.CommittedVersion)
	pretty += fmt.Sprintf(fieldNameFmt+"%s\n", "LaunchSVN", r.LaunchSVN)
	pretty += fmt.Sprintf(fieldNameFmt+"%s\n", "Signature", r.Signature)
	return pretty
}

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

	var reportBytes []byte
	if *reportDataFlag != "" {
		var err error
		reportBytes, err = hex.DecodeString(*reportDataFlag)
		if err != nil {
			fmt.Printf("failed to decode report data:%s\n", err)
			os.Exit(1)
		}
	}
	if *binaryFmtFlag {
		var binaryReport []byte
		var err error
		if *fakeReportFlag {
			binaryReport, err = fake.FetchRawSNPReport()
		} else {
			binaryReport, err = amdsevsnp.FetchRawSNPReport(reportBytes)
		}
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Printf("%x\n", binaryReport)
		os.Exit(0)
	}

	var report amdsevsnp.Report
	var err error
	if *fakeReportFlag {
		report, err = fake.FetchSNPReport(*hostDataFlag)
	} else {
		report, err = amdsevsnp.FetchParsedSNPReport(reportBytes)
	}
	if err != nil {
		fmt.Printf("failed to fetch SNP report: %s", err)
		os.Exit(1)
	}

	if !*verbosePrintFlag {
		fmt.Printf("%+v\n", report)
	} else {
		fmt.Println(verboseReport(report))
	}
}
