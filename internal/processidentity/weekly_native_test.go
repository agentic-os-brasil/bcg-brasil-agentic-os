package processidentity

import (
	"strings"
	"testing"
)

func TestWeeklyNativeDeclarationIsClosedLiteralMetadata(t *testing.T) {
	d := WeeklyNativeDeclaration{Topology: WeeklyNativeDarwinStandalone, TeamID: "ABCDEFGHIJ", SigningIdentifier: "test.synthetic.runtime", CodeDirectoryHash: strings.Repeat("a", 40), Architecture: "arm64"}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*WeeklyNativeDeclaration){func(d *WeeklyNativeDeclaration) { d.Topology = "script" }, func(d *WeeklyNativeDeclaration) { d.TeamID = "ABC\" or true" }, func(d *WeeklyNativeDeclaration) { d.SigningIdentifier = "x\" or true" }, func(d *WeeklyNativeDeclaration) { d.CodeDirectoryHash = strings.Repeat("a", 64) }, func(d *WeeklyNativeDeclaration) { d.Architecture = "unknown" }, func(d *WeeklyNativeDeclaration) { d.SignerCertificateSHA256 = strings.Repeat("b", 64) }} {
		value := d
		change(&value)
		if err := value.Validate(); err == nil {
			t.Fatal("unbounded, injected or cross-platform declaration accepted", value)
		}
	}
	w := WeeklyNativeDeclaration{Topology: WeeklyNativeWindowsStandalone, Architecture: "amd64", SignatureMode: "embedded_primary_index0", SignerCertificateSHA256: strings.Repeat("c", 64), RevocationPolicy: "cache_only_chain"}
	if err := w.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*WeeklyNativeDeclaration){func(d *WeeklyNativeDeclaration) { d.SignatureMode = "any_signature" }, func(d *WeeklyNativeDeclaration) { d.RevocationPolicy = "disabled" }, func(d *WeeklyNativeDeclaration) { d.SignerCertificateSHA256 = strings.Repeat("C", 64) }, func(d *WeeklyNativeDeclaration) { d.TeamID = "ABCDEFGHIJ" }} {
		value := w
		change(&value)
		if err := value.Validate(); err == nil {
			t.Fatal("unsupported signature policy accepted")
		}
	}
}

func TestWeeklyNativeFactsZeroCannotClaimIdentity(t *testing.T) {
	var zero WeeklyNativeChild
	if zero.Same(zero) || zero.Digest() != "" || zero.PID() != 0 || !zero.StartedAt().IsZero() || zero.ImagePath() != "" {
		t.Fatal("zero observation became identity")
	}
}
