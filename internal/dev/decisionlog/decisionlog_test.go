package decisionlog

import (
	"strings"
	"testing"
)

const validDecision = `# Decision log

## HARN - Establish the development harness

- Date: 2026-07-17
- Status: accepted
- Owner: Daniel Scardini
- Context: Development needs a recoverable reasoning trail.
- Decision: Use decisions and tests as the development loop.
- Consequences: Behavioral changes start with contract evidence.
- Refs: specs/005-development-harness.md
- Supersedes: none
`

func TestParseValidDecision(t *testing.T) {
	entries, err := Parse(strings.NewReader(validDecision))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Code != "HARN" {
		t.Fatalf("Parse() entries = %#v", entries)
	}
}

func TestParseRejectsDuplicateCode(t *testing.T) {
	input := validDecision + strings.Replace(validDecision, "# Decision log\n\n", "", 1)
	_, err := Parse(strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), "duplicate decision HARN") {
		t.Fatalf("Parse() error = %v, want duplicate code", err)
	}
}

func TestParseRejectsMissingRequiredField(t *testing.T) {
	input := strings.Replace(validDecision, "- Decision: Use decisions and tests as the development loop.\n", "", 1)
	_, err := Parse(strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), `missing required field "Decision"`) {
		t.Fatalf("Parse() error = %v, want missing Decision", err)
	}
}

func TestAvailableRequiresExactlyFourUppercaseLetters(t *testing.T) {
	for _, code := range []string{"ABC", "ABCDE", "AbCD", "A1CD"} {
		if err := Available(nil, code); err == nil {
			t.Errorf("Available(%q) error = nil, want format error", code)
		}
	}
	if err := Available(nil, "XPTO"); err != nil {
		t.Fatalf("Available(XPTO) error = %v", err)
	}
}

func TestAvailableRejectsExistingCode(t *testing.T) {
	err := Available([]Entry{{Code: "EUWH", Title: "Existing decision"}}, "EUWH")
	if err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("Available(EUWH) error = %v, want collision", err)
	}
}

func TestParseRejectsUnknownSupersededCode(t *testing.T) {
	input := strings.Replace(validDecision, "- Supersedes: none", "- Supersedes: NOPE", 1)
	_, err := Parse(strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), "supersedes unknown decision NOPE") {
		t.Fatalf("Parse() error = %v, want unknown superseded code", err)
	}
}
