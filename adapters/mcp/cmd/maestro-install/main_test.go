package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestFailureSummaryPreservesReportedSetupPhase(t *testing.T) {
	for _, detail := range []string{
		"explicit confirmation and exact review digest required",
		"signed payload activated; owner data preserved; setup pending: packet destination exists",
	} {
		var output bytes.Buffer
		writeFailure(&output, errors.New(detail))
		text := output.String()
		if !strings.HasPrefix(text, "maestro-install: operation did not complete;") || !strings.HasSuffix(text, detail+"\n") || !strings.Contains(text, "review exact local setup before retrying") {
			t.Fatalf("failure summary obscures reported phase or recovery: %s", text)
		}
		if strings.Contains(text, "operation refused; installation/owner state preserved") {
			t.Fatalf("summary asserts no committed installation effects: %s", text)
		}
	}
}
