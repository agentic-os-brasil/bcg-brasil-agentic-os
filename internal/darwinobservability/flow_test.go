package darwinobservability

import (
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maestroflow"
)

func TestMaestroFlowReceiptProjectsWalterSkipAndDirectPath(t *testing.T) {
	now := time.Now().UTC()
	evidence := strings.Repeat("a", 64)
	request, err := maestroflow.NewRequest("flow-direct", "low-materiality", maestroflow.CaseDirect, maestroflow.DirectCaseReason, false, maestroflow.WalterSkipReason, evidence, now)
	if err != nil {
		t.Fatal(err)
	}
	state, _, err := maestroflow.Start(request)
	if err != nil {
		t.Fatal(err)
	}
	state, _, _ = state.CompleteCase(now)
	state, _, _ = state.SkipWalter(now)
	state, receipt, err := state.Deliver(now)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.WalterSkipped || receipt.PostAccountValidationRequired || !state.WalterSkipped {
		t.Fatalf("state = %#v receipt = %#v", state, receipt)
	}
	record, err := FromMaestroFlowReceipt(receipt, "win-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	if record.Kind != KindFlow || record.Flow.EntryPath != "case_direct" || !record.Flow.WalterSkipped || record.Flow.WalterRequired {
		t.Fatalf("flow record = %#v", record)
	}
}

func TestWeeklyReportAggregatesMaestroFlow(t *testing.T) {
	scope := strings.Repeat("c", 64)
	start := time.Now().UTC().Add(-time.Hour)
	window := Window{ID: "win-dddddddddddddddddddddddddddddddd", ScopeSHA256: scope, Start: start, End: start.Add(2 * time.Hour)}
	evidence := strings.Repeat("d", 64)
	request, err := maestroflow.NewRequest("flow-week", "low-materiality", maestroflow.CaseDirect, maestroflow.DirectCaseReason, false, maestroflow.WalterSkipReason, evidence, start.Add(10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	state, _, _ := maestroflow.Start(request)
	state, _, _ = state.CompleteCase(start.Add(10 * time.Minute))
	state, _, _ = state.SkipWalter(start.Add(10 * time.Minute))
	state, receipt, _ := state.Deliver(start.Add(10 * time.Minute))
	record, err := FromMaestroFlowReceipt(receipt, window.ID, scope)
	if err != nil {
		t.Fatal(err)
	}
	report, err := BuildWeekly([]Record{record}, window)
	if err != nil {
		t.Fatal(err)
	}
	if report.Flow.Records != 1 || report.Flow.DirectCase != 1 || report.Flow.WalterSkipped != 1 || len(report.Flow.WalterSkipReasons) != 1 {
		t.Fatalf("flow scorecard = %#v", report.Flow)
	}
}
