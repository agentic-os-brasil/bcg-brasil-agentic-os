// Command pilot-acceptance creates isolated evidence or validates an approved
// corporate-device report. It never promotes a release.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/pilotacceptance"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "isolated":
		isolated(os.Args[2:])
	case "corporate":
		corporate(os.Args[2:])
	case "validate-phase":
		validatePhase(os.Args[2:])
	case "validate":
		validate(os.Args[2:])
	default:
		usage()
	}
}

func corporate(args []string) {
	flags := flag.NewFlagSet("corporate", flag.ExitOnError)
	installReceipt := flags.String("install-receipt", "", "sanitized clean-device install receipt")
	updateReceipt := flags.String("update-receipt", "", "sanitized clean-device update receipt")
	rollbackReceipt := flags.String("rollback-receipt", "", "sanitized clean-device rollback receipt")
	operator := flags.String("operator", "", "approved acceptance operator ID")
	deviceIDHash := flags.String("device-id-hash", "", "one-way sanitized device identifier")
	policyID := flags.String("policy-id", "", "sanitized approved policy identifier")
	channel := flags.String("channel", "", "approved canary, beta or stable channel")
	supportOwner := flags.String("support-owner", "", "accepted pilot support owner ID")
	output := flags.String("output", "", "new corporate-device report path")
	_ = flags.Parse(args)
	if *installReceipt == "" || *updateReceipt == "" || *rollbackReceipt == "" ||
		*operator == "" || *deviceIDHash == "" ||
		*policyID == "" || *channel == "" || *supportOwner == "" ||
		*output == "" || flags.NArg() != 0 {
		fmt.Fprintln(
			os.Stderr,
			"usage: go run ./dev/pilot-acceptance corporate --install-receipt FILE --update-receipt FILE --rollback-receipt FILE --operator ID --device-id-hash SHA256 --policy-id ID --channel canary|beta|stable --support-owner ID --output FILE",
		)
		os.Exit(2)
	}
	report, err := pilotacceptance.Corporate(pilotacceptance.CorporateOptions{
		Receipts: map[string]string{
			"install": *installReceipt, "update": *updateReceipt, "rollback": *rollbackReceipt,
		},
		Attestation: pilotacceptance.Attestation{
			Operator: *operator, DeviceIDHash: *deviceIDHash,
			PolicyID: *policyID, ApprovedChannel: *channel, SupportOwner: *supportOwner,
		},
	})
	fatalIf(err)
	fatalIf(pilotacceptance.Write(*output, report))
	fmt.Printf(
		"corporate-device operator attestation written to %s; external acceptance and pilot expansion remain human decisions\n",
		*output,
	)
}

func validatePhase(args []string) {
	flags := flag.NewFlagSet("validate-phase", flag.ExitOnError)
	receiptPath := flags.String("receipt", "", "clean-device phase receipt path")
	_ = flags.Parse(args)
	if *receiptPath == "" || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./dev/pilot-acceptance validate-phase --receipt FILE")
		os.Exit(2)
	}
	receipt, digest, err := pilotacceptance.ReadPhase(*receiptPath)
	fatalIf(err)
	fmt.Printf(
		"clean-device phase receipt valid: %s/%s %s->%s (%s)\n",
		receipt.Platform,
		receipt.Phase,
		receipt.FromVersion,
		receipt.ToVersion,
		digest,
	)
}

func isolated(args []string) {
	flags := flag.NewFlagSet("isolated", flag.ExitOnError)
	runID := flags.String("run-id", "", "CI run identifier")
	platform := flags.String("platform", "", "windows or macos")
	version := flags.String("version", "", "candidate MAJOR.MINOR.PATCH")
	output := flags.String("output", "", "new evidence report path")
	_ = flags.Parse(args)
	if *runID == "" || *platform == "" || *version == "" || *output == "" || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./dev/pilot-acceptance isolated --run-id ID --platform windows|macos --version MAJOR.MINOR.PATCH --output FILE")
		os.Exit(2)
	}
	finished := time.Now().UTC()
	report := pilotacceptance.Isolated(*runID, *platform, *version, finished.Add(-time.Second), finished)
	fatalIf(pilotacceptance.Write(*output, report))
	fmt.Printf("isolated engineering evidence written to %s; operator-attested device evidence and corporate acceptance remain pending\n", *output)
}

func validate(args []string) {
	flags := flag.NewFlagSet("validate", flag.ExitOnError)
	reportPath := flags.String("report", "", "acceptance report path")
	_ = flags.Parse(args)
	if *reportPath == "" || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./dev/pilot-acceptance validate --report FILE")
		os.Exit(2)
	}
	report, err := pilotacceptance.Read(*reportPath)
	fatalIf(err)
	fmt.Printf("%s evidence valid: %s/%s (%s)\n", report.Mode, report.Platform, report.CandidateVersion, report.ReadinessClaim)
}

func usage() {
	fmt.Fprintln(
		os.Stderr,
		"usage: go run ./dev/pilot-acceptance <isolated|corporate|validate-phase|validate> [options]",
	)
	os.Exit(2)
}

func fatalIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
