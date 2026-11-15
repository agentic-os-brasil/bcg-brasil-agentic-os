package product_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The existing ordinary-account owner journey owns these SDK and shipping
// stdio assertions; no new platform parent or private test API is required.
func assertOwnerAtlasDailyWorkflow(t *testing.T) {
	t.Helper()
	for _, stdio := range []bool{false, true} {
		f := newOwnerFixture(t, stdio)
		allow := true
		var reviewText string
		cs := f.connect(t, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			reviewText = req.Params.Message
			if !allow {
				return &mcp.ElicitResult{Action: "decline"}, nil
			}
			return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
		})
		call := func(op string, fields map[string]any, fail bool) map[string]any {
			tool := "maestro_apply"
			if op == "status" || op == "collect" {
				tool = "maestro_query"
			}
			args := map[string]any{"operation": "owner.atlas." + op}
			for k, v := range fields {
				args[k] = v
			}
			out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
			if err != nil {
				t.Fatal(err)
			}
			var value map[string]any
			if len(out.Content) != 1 {
				t.Fatal("owner atlas returned no single domain result")
			}
			if err := json.Unmarshal([]byte(out.Content[0].(*mcp.TextContent).Text), &value); err != nil {
				t.Fatal(err)
			}
			if out.IsError != fail {
				t.Fatalf("%s: is_error=%t code=%v", args["operation"], out.IsError, value["code"])
			}
			return value
		}
		plan := contentCall(t, cs, "maestro_query", map[string]any{"operation": "methods.plan", "method_id": "start-day"}, false)
		domainReady, hostPending := false, false
		for _, raw := range plan["dependencies"].([]any) {
			dependency := raw.(map[string]any)
			if dependency["id"] == "domain.owner_atlas" {
				domainReady = dependency["state"] == "available"
			}
			if dependency["id"] == "host.trusted_review" {
				hostPending = dependency["state"] == "unavailable"
			}
		}
		if !domainReady || !hostPending || plan["execution_state"] != "not_executed" {
			t.Fatal("method discovery did not separate registered owner operations from host authority", plan)
		}
		resource := contentCall(t, cs, "maestro_query", map[string]any{"operation": "methods.read", "resource_uri": "maestro://methods/start-day/resources/owner-atlas-mcp.md"}, false)
		if !strings.Contains(resource["text"].(string), "owner.atlas.collect") {
			t.Fatal("shared owner method resource is not readable", resource)
		}
		root := filepath.Join(filepath.Dir(f.root), "atlas", "owner")
		if got := call("status", nil, false); got["prepared"] != false {
			t.Fatal("fresh owner atlas was not reported unprepared", got)
		}
		call("collect", map[string]any{"purpose": "Open the synthetic working day", "pages": []string{"daily/2026-11-15.md"}}, true)
		if _, err := os.Lstat(root); !os.IsNotExist(err) {
			t.Fatal("owner atlas queries initialized or repaired state", err)
		}
		call("prepare", nil, false)
		if !strings.Contains(reviewText, "Owner atlas") {
			t.Fatal("preparation did not present its canonical scaffold")
		}
		const page = "daily/2026-11-15.md"
		const body = "# Synthetic working day\n\n## Priorities\n\n## Notes\n"
		created := call("create", map[string]any{"page": page, "text": body}, false)
		var presentation struct{ Pages []struct{ Text string } }
		if err := json.Unmarshal([]byte(reviewText[strings.Index(reviewText, "{"):]), &presentation); err != nil || len(presentation.Pages) != 1 || presentation.Pages[0].Text != body || created["state"] != "written" {
			t.Fatal("reviewed daily page was not created with exact content", created)
		}
		args := map[string]any{"page": page, "section": "## Priorities", "text": "- SYNTHETIC_OPEN_DAY: finish the reviewed fixture.", "expected_page_revision": created["revision"]}
		appended := call("append", args, false)
		if appended["state"] != "written" {
			t.Fatal("reviewed daily opening was not appended", appended)
		}
		if replay := call("append", args, false); replay["operation_id"] != appended["operation_id"] || replay["revision"] != appended["revision"] {
			t.Fatal("identical attended append did not retain its canonical receipt", replay)
		}
		args["text"] = "- STALE_DAILY_ENTRY"
		call("append", args, true)
		allow = false
		call("append", map[string]any{"page": page, "section": "## Notes", "text": "- DECLINED_DAY_CLOSE", "expected_page_revision": appended["revision"]}, true)
		allow = true
		closed := call("append", map[string]any{"page": page, "section": "## Notes", "text": "- SYNTHETIC_CLOSED_DAY: the real fixture finished.", "expected_page_revision": appended["revision"]}, false)
		if closed["state"] != "written" {
			t.Fatal("reviewed daily closure was not persisted", closed)
		}
		const craftPage = "concepts/reviewed-method.md"
		craft := call("create", map[string]any{"page": craftPage, "text": "# Synthetic method\n"}, false)
		updated := call("update", map[string]any{"page": craftPage, "text": "# Synthetic method\n\nReviewed correction.\n", "expected_page_revision": craft["revision"]}, false)
		if err := os.Remove(filepath.Join(root, "index.md")); err != nil {
			t.Fatal(err)
		}
		if got := call("status", nil, false); got["prepared"] != false {
			t.Fatal("partial scaffold was reported prepared", got)
		}
		call("prepare", nil, false)
		assertOwnerAtlasReviewBoundaries(t, call, root, func() string { return reviewText })
		if err := cs.Close(); err != nil {
			t.Fatal(err)
		}
		cs = f.connect(t, nil)
		read := call("collect", map[string]any{"purpose": "Resume the reviewed synthetic daily after restart", "pages": []string{page, craftPage}}, false)
		pages := read["pages"].([]any)
		if len(pages) != 2 {
			t.Fatal("restarted owner read did not return its named page", read)
		}
		entry := pages[0].(map[string]any)
		text := entry["content"].(string)
		if entry["revision"] != closed["revision"] || strings.Count(text, "SYNTHETIC_OPEN_DAY") != 1 || strings.Count(text, "SYNTHETIC_CLOSED_DAY") != 1 || strings.Contains(text, "STALE_DAILY_ENTRY") || strings.Contains(text, "DECLINED_DAY_CLOSE") {
			t.Fatal("daily restart lost genuine work or included declined/stale text", read)
		}
		correction := pages[1].(map[string]any)
		if correction["revision"] != updated["revision"] || correction["content"] != "# Synthetic method\n\nReviewed correction.\n" {
			t.Fatal("reviewed correction did not survive restart", correction)
		}
		refused := call("append", map[string]any{"page": page, "section": "## Notes", "text": "- NO_HOST_REVIEW", "expected_page_revision": closed["revision"]}, true)
		if refused["code"] != "confirmation_unavailable" {
			t.Fatal("missing trusted review was not explicit", refused)
		}
		after := call("collect", map[string]any{"purpose": "Verify the unreviewed change had no effect", "pages": []string{page}}, false)["pages"].([]any)[0].(map[string]any)
		if after["revision"] != closed["revision"] || strings.Contains(after["content"].(string), "NO_HOST_REVIEW") {
			t.Fatal("missing review changed the owner page", after)
		}
		call("collect", map[string]any{"purpose": "Reject outside owner access", "pages": []string{"../outside.md"}}, true)
	}
}

// Legal page bytes must remain fully reviewable even when JSON escaping expands
// their wire representation; an existing scaffold can contain four such pages.
func assertOwnerAtlasReviewBoundaries(t *testing.T, call func(string, map[string]any, bool) map[string]any, root string, lastReview func() string) {
	t.Helper()
	const maximumPageBytes = 256 << 10
	largePage := func(token string) string {
		const heading = "# Boundary review\n\n"
		remaining := maximumPageBytes - len(heading)
		return heading + strings.Repeat(token, remaining/len(token)) + strings.Repeat("<", remaining%len(token))
	}
	before, after := largePage("<>&"), largePage("&><")
	readReview := func() []struct {
		Page, Text string
		Current    *struct{ Content string }
	} {
		message := lastReview()
		start := strings.Index(message, "{")
		if start < 0 {
			t.Fatal("native review did not contain its complete domain presentation")
		}
		var presentation struct {
			Pages []struct {
				Page, Text string
				Current    *struct{ Content string }
			}
		}
		if err := json.Unmarshal([]byte(message[start:]), &presentation); err != nil {
			t.Fatal("full native review is not valid JSON", err)
		}
		return presentation.Pages
	}
	scaffold := []string{"index.md", "learnings/index.md", "development/index.md", "concepts/index.md"}
	for _, name := range scaffold {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(before), 0600); err != nil {
			t.Fatal(err)
		}
	}
	call("prepare", nil, false)
	t.Logf("full owner preparation review: existing_pages=%d each_page_bytes=%d message_bytes=%d", len(scaffold), len(before), len(lastReview()))
	pages := readReview()
	if len(pages) != len(scaffold) {
		t.Fatal("large preparation did not review every fixed page")
	}
	for _, selected := range pages {
		if selected.Current == nil || selected.Current.Content != before || selected.Text == "" {
			t.Fatal("preparation omitted a complete existing page or proposed scaffold")
		}
	}
	for _, name := range scaffold {
		if body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name))); err != nil || string(body) != before {
			t.Fatal("reviewed preparation changed existing maximum-size scaffold content", err)
		}
	}
	const page = "concepts/expansion-boundary.md"
	created := call("create", map[string]any{"page": page, "text": before}, false)
	t.Logf("full owner create review: page_bytes=%d message_bytes=%d", len(before), len(lastReview()))
	pages = readReview()
	if created["state"] != "written" || len(pages) != 1 || pages[0].Text != before || pages[0].Current != nil {
		t.Fatal("maximum-size create did not present its complete exact body")
	}
	call("create", map[string]any{"page": page, "text": after}, false)
	pages = readReview()
	if len(pages) != 1 || pages[0].Text != after || pages[0].Current == nil || pages[0].Current.Content != before {
		t.Fatal("occupied-page review lost its full proposed or current body")
	}
	if body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(page))); err != nil || string(body) != before {
		t.Fatal("large occupied-page creation did not preserve existing work", err)
	}
	updated := call("update", map[string]any{"page": page, "text": after, "expected_page_revision": created["revision"]}, false)
	t.Logf("full owner update review: each_page_bytes=%d message_bytes=%d", len(after), len(lastReview()))
	pages = readReview()
	if updated["state"] != "written" || len(pages) != 1 || pages[0].Text != after || pages[0].Current == nil || pages[0].Current.Content != before {
		t.Fatal("maximum-size update did not present full current and proposed bodies")
	}
	if body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(page))); err != nil || string(body) != after {
		t.Fatal("maximum-size reviewed update was not persisted", err)
	}

	const appendPage = "concepts/expansion-append.md"
	base := call("create", map[string]any{"page": appendPage, "text": "# Bounded entry\n\n## Notes\n"}, false)
	entry := strings.Repeat("<>&", (8<<10)/3) + "<>"
	appended := call("append", map[string]any{"page": appendPage, "section": "## Notes", "text": entry, "expected_page_revision": base["revision"]}, false)
	if appended["state"] != "written" {
		t.Fatal("legal expanded8KiB entry was not persisted", appended)
	}
	pages = readReview()
	if len(pages) != 1 || !strings.Contains(pages[0].Text, entry) {
		t.Fatal("expanded entry was not completely reviewed")
	}
	requested := make([]string, 24)
	for index := range requested {
		requested[index] = "concepts/" + strings.Repeat("é", 120) + "/" + strings.Repeat("é", 120) + fmt.Sprintf("%02d.md", index)
	}
	collected := call("collect", map[string]any{"purpose": strings.Repeat("<>&", 256), "pages": requested}, false)
	if len(collected["omissions"].([]any)) != len(requested) {
		t.Fatal("legal bounded collection lost its explicit missing-page outcomes")
	}

}
