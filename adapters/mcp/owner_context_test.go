package product_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const acceptedCommunication = "# Communication\n\nPrefer concise Portuguese summaries and explicit confirmation before external sends.\n"
const restrictedOwnerMarker = "PRIVATE_YODA_ONLY_FACET_MUST_NOT_REACH_SESSION"

func applyContextFixtureFacet(t *testing.T, root, facet, text string) {
	t.Helper()
	p, err := ownerctx.SubmitRefinement(root, ownerctx.RefinementInput{Facet: facet, Evidence: "explicit synthetic owner answer", ProposedBody: text})
	if err != nil {
		t.Fatal(err)
	}
	r, err := ownerctx.ReviewRefinement(root, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ownerctx.ApplyReviewedRefinement(root, r.ID, r.ReviewDigest, true); err != nil {
		t.Fatal(err)
	}
}

func acceptedContextFixture(t *testing.T, communication string) ownerFixture {
	t.Helper()
	f := newOwnerFixture(t, false)
	if _, err := ownerctx.SelectOnboardingTrack(f.root, ownerctx.OnboardingTrackQuick); err != nil {
		t.Fatal(err)
	}
	for _, facet := range []string{"owner-identity", "personal-context", "professional-role", "communication-style", "preferences", "quality-bar"} {
		text := "# Synthetic accepted profile\n\nExplicit answer for " + facet + ".\n"
		if facet == "communication-style" {
			text = communication
		}
		applyContextFixtureFacet(t, f.root, facet, text)
	}
	applyContextFixtureFacet(t, f.root, "psychological-profile", "# Restricted profile\n\n"+restrictedOwnerMarker+"\n")
	r, err := ownerctx.ReviewOnboarding(f.root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ownerctx.ConfirmOnboarding(f.root, r.ReviewDigest); err != nil {
		t.Fatal(err)
	}
	return f
}

func ownerContextQuery(t *testing.T, client *mcp.ClientSession, args map[string]any) (map[string]any, bool) {
	t.Helper()
	args["operation"] = "owner.context"
	out, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_query", Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Content) != 1 {
		t.Fatalf("unexpected context result: %+v", out)
	}
	text, ok := out.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("unexpected result content: %T", out.Content[0])
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(text.Text), &value); err != nil {
		t.Fatal(err)
	}
	return value, out.IsError
}

func TestAcceptedOwnerContextReachesBoundMCPSession(t *testing.T) {
	f := acceptedContextFixture(t, acceptedCommunication)
	client := f.connect(t, nil)
	view, failed := ownerContextQuery(t, client, map[string]any{})
	if failed {
		t.Fatalf("accepted owner context was not delivered: %+v", view)
	}
	text, ok := view["text"].(string)
	if !ok || !strings.Contains(text, acceptedCommunication) {
		t.Fatalf("accepted communication missing: %+v", view)
	}
	if view["reader"] != "session" || view["authority"] != "owner_profile_data_only" || len(view["generation"].(string)) != 64 {
		t.Fatalf("missing bounded context contract: %+v", view)
	}
	body, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{restrictedOwnerMarker, f.root, f.options.UserHome, f.options.WorkspacePath} {
		if strings.Contains(string(body), forbidden) {
			t.Fatal("owner context disclosed a private path or unauthorized facet")
		}
	}
}

func requireOwnerContextRefusal(t *testing.T, client *mcp.ClientSession, args map[string]any, code string) {
	t.Helper()
	value, failed := ownerContextQuery(t, client, args)
	if !failed || value["code"] != code || len(value) != 1 {
		t.Fatalf("unsafe refusal: %+v error=%v", value, failed)
	}
}

func TestOwnerContextRequiresCurrentFinalConfirmation(t *testing.T) {
	f := newOwnerFixture(t, false)
	client := f.connect(t, nil)
	requireOwnerContextRefusal(t, client, map[string]any{}, "owner_context_not_confirmed")
	if _, err := ownerctx.SelectOnboardingTrack(f.root, "quick"); err != nil {
		t.Fatal(err)
	}
	applyContextFixtureFacet(t, f.root, "owner-identity", "# Identity\n\nPartial synthetic answer.\n")
	requireOwnerContextRefusal(t, client, map[string]any{}, "owner_context_not_confirmed")
}

func TestOwnerContextPaginationAndFreshness(t *testing.T) {
	communication := "# Communication\n\n" + strings.Repeat("Prefer português conciso. ", 220) + "\n"
	f := acceptedContextFixture(t, communication)
	client := f.connect(t, nil)
	first, failed := ownerContextQuery(t, client, map[string]any{})
	if failed {
		t.Fatal(first)
	}
	if len(first["text"].(string)) > 4096 || first["complete"] != false {
		t.Fatalf("unbounded default page: %+v", first)
	}
	generation := first["generation"].(string)
	requireOwnerContextRefusal(t, client, map[string]any{"offset": 4}, "owner_context_generation_required")
	var joined strings.Builder
	offset := 0
	for n := 0; n < 64; n++ {
		page, bad := ownerContextQuery(t, client, map[string]any{"offset": offset, "max_bytes": 257, "generation": generation})
		if bad {
			t.Fatal(page)
		}
		text := page["text"].(string)
		if len(text) > 257 || !utf8.ValidString(text) || page["generation"] != generation {
			t.Fatal("unstable or invalid page")
		}
		joined.WriteString(text)
		next := int(page["next_offset"].(float64))
		if next == -1 {
			if page["complete"] != true || joined.Len() != int(page["total_bytes"].(float64)) {
				t.Fatal("incomplete terminal page")
			}
			break
		}
		if next <= offset {
			t.Fatal("nonadvancing cursor")
		}
		offset = next
	}
	if !strings.Contains(joined.String(), communication) || strings.Contains(joined.String(), restrictedOwnerMarker) {
		t.Fatal("accepted context was truncated or unauthorized text exported")
	}
	applyContextFixtureFacet(t, f.root, "communication-style", acceptedCommunication+"New preference.\n")
	requireOwnerContextRefusal(t, client, map[string]any{"offset": 4, "generation": generation}, "owner_context_not_confirmed")
	review, err := ownerctx.ReviewOnboarding(f.root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ownerctx.ConfirmOnboarding(f.root, review.ReviewDigest); err != nil {
		t.Fatal(err)
	}
	requireOwnerContextRefusal(t, client, map[string]any{"offset": 4, "generation": generation}, "owner_context_changed")
	current, bad := ownerContextQuery(t, client, map[string]any{})
	if bad || current["generation"] == generation {
		t.Fatalf("no fresh context: %+v", current)
	}
}

func TestOwnerContextRejectsClaimedReadersAndPrivateSelectors(t *testing.T) {
	f := acceptedContextFixture(t, acceptedCommunication)
	client := f.connect(t, nil)
	for _, field := range []string{"reader", "role", "path", "facet", "facets"} {
		requireOwnerContextRefusal(t, client, map[string]any{field: "yoda"}, "execution_rejected")
	}
	for _, maximum := range []int{3, 4097} {
		requireOwnerContextRefusal(t, client, map[string]any{"max_bytes": maximum}, "execution_rejected")
	}
}

func TestOwnerContextRefusesReaderPolicyTamperingBetweenPages(t *testing.T) {
	f := acceptedContextFixture(t, acceptedCommunication)
	client := f.connect(t, nil)
	first, bad := ownerContextQuery(t, client, map[string]any{"max_bytes": 64})
	if bad {
		t.Fatal(first)
	}
	path := filepath.Join(f.root, "owner", "registry.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var registry map[string]any
	if err = json.Unmarshal(body, &registry); err != nil {
		t.Fatal(err)
	}
	registry["facets"].(map[string]any)["communication-style"].(map[string]any)["readers"] = []string{"yoda"}
	changed, err := json.Marshal(registry)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	requireOwnerContextRefusal(t, client, map[string]any{"offset": 64, "generation": first["generation"]}, "owner_context_unavailable")
}
