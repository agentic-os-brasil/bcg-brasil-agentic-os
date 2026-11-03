package product_test

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func wikiCall(t *testing.T, cs *mcp.ClientSession, operation string, fields map[string]any, fail bool) map[string]any {
	t.Helper()
	args := map[string]any{"operation": "wiki." + operation}
	for k, v := range fields {
		args[k] = v
	}
	out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_query", Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError != fail || len(out.Content) != 1 {
		t.Fatalf("wiki.%s: %+v", operation, out)
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(out.Content[0].(*mcp.TextContent).Text), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestPrivateWikiProductSourceToReadback(t *testing.T) {
	for _, transport := range []string{"sdk", "stdio"} {
		t.Run(transport, func(t *testing.T) {
			f := newOwnerFixture(t, transport == "stdio")
			prepareMemoryFixture(t, f)
			cs := f.connect(t, cadenceApprove)
			const marker = "private-wiki-body: preserve OBSERVABLE_NAVIGATION and source drilldown"
			memoryCall(t, cs, "memory.capture", map[string]any{"text": marker}, false)
			memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "daily", "date": time.Now().UTC().Format("2006-01-02")}, false)
			q := cadenceCall(t, cs, "qualify", map[string]any{"job_id": "wiki-incremental-sync", "expected_revision": 0}, false)
			if q["qualification_state"] != "qualified" || q["state"] != "inactive" {
				t.Fatal(q)
			}
			// Reconciliation needs genuinely changed committed input; current
			// navigation or a fresh clock cannot qualify another compiler run.
			memoryCall(t, cs, "memory.capture", map[string]any{"text": "private-wiki-body: SECOND_COMMITTED_SOURCE"}, false)
			memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "daily", "date": time.Now().UTC().Format("2006-01-02")}, false)
			reconciled := cadenceCall(t, cs, "qualify", map[string]any{"job_id": "wiki-reconcile", "expected_revision": 0}, false)
			integrity := cadenceCall(t, cs, "qualify", map[string]any{"job_id": "wiki-integrity-check", "expected_revision": 0}, false)
			for _, qualified := range []map[string]any{reconciled, integrity} {
				if qualified["qualification_state"] != "qualified" || qualified["state"] != "inactive" {
					t.Fatal(qualified)
				}
			}
			for _, job := range []string{"wiki-incremental-sync", "wiki-reconcile", "wiki-integrity-check"} {
				report := maintenanceReportCall(t, cs, job, false)
				if report["wiki"] == nil || report["finding"] == "" || strings.Contains(reportJSON(t, report), "private-wiki-body") {
					t.Fatal("actual body-free domain report missing or private text leaked")
				}
			}
			cadenceCall(t, cs, "qualify", map[string]any{"job_id": "wiki-reconcile", "expected_revision": reconciled["revision"]}, true)
			active := cadenceCall(t, cs, "enroll", map[string]any{"job_id": "wiki-incremental-sync", "expected_revision": q["revision"], "timezone": "UTC"}, false)
			if active["schedule"].(map[string]any)["event_only"] != true {
				t.Fatal("incremental enrollment became a calendar schedule")
			}
			memoryCall(t, cs, "memory.capture", map[string]any{"text": "private-wiki-body: SUBSEQUENT_EVENT"}, false)
			memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "daily", "date": time.Now().UTC().Format("2006-01-02")}, false)
			run := cadenceCall(t, cs, "run", nil, false)
			if run["state"] != "processed" || len(run["receipts"].([]any)) != 1 {
				t.Fatalf("actual enrolled event did not run once: %+v", run)
			}
			if replay := cadenceCall(t, cs, "run", nil, false); replay["state"] != "no_due_work" {
				t.Fatal(replay)
			}
			if err := cs.Close(); err != nil {
				t.Fatal(err)
			}
			cs = f.connect(t, cadenceApprove)
			status := wikiCall(t, cs, "status", nil, false)
			if status["state"] != "current" || status["implementation_current"] != true || status["configuration_current"] != true {
				t.Fatal(status)
			}
			generation := status["generation"].(string)
			page := func(reference string) string {
				return wikiCall(t, cs, "read", map[string]any{"generation": generation, "reference": reference, "offset": 0, "max_bytes": 4096}, false)["content"].(string)
			}
			index := page("index.md")
			match := regexp.MustCompile(`\((concepts/[a-f0-9]{64}\.md)\)`).FindStringSubmatch(index)
			if len(match) != 2 {
				t.Fatalf("actual concept missing from index: %s", index)
			}
			concept := page(match[1])
			if !strings.Contains(concept, "OBSERVABLE_NAVIGATION") || !strings.Contains(page("backlinks.json"), match[1]) {
				t.Fatal("independently expected concept/backlinks absent")
			}
			if !strings.Contains(concept, "SUBSEQUENT_EVENT") {
				t.Fatal("restarted readback missed the enrolled event")
			}
			reference := regexp.MustCompile(`maestro://memory/[a-f0-9]{64}`).FindString(concept)
			if reference == "" {
				t.Fatal("source pointer absent")
			}
			source := wikiCall(t, cs, "source", map[string]any{"generation": generation, "reference": reference, "offset": 0, "max_bytes": 4096}, false)
			if !strings.Contains(source["content"].(string), "OBSERVABLE_NAVIGATION") {
				t.Fatal("canonical source drilldown absent")
			}
		})
	}
}

func reportJSON(t *testing.T, value any) string {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
