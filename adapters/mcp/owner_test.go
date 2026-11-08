package product_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ownerFixture struct {
	options      product.Options
	root, binary string
}

const ownerReviewFixtureSuffix = "\n\"Revisão autorizada\" <conteúdo> & colaboração.\n"

func newOwnerFixture(t *testing.T, stdio bool) ownerFixture {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("verified owner admission unavailable")
	}
	base := physicalTempDir(t)
	f := ownerFixture{options: product.Options{ProductRoot: filepath.Join(base, "product"), UserHome: filepath.Join(base, "user"), WorkspacePath: filepath.Join(base, "workspace"), LookupEnv: func(string) (string, bool) { return "", false }}}
	for _, p := range []string{f.options.ProductRoot, f.options.UserHome} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(f.options.ProductRoot, 0500); err != nil {
		t.Fatal(err)
	}
	if stdio {
		f.binary = filepath.Join(base, "maestro-mcp")
		build := exec.Command("go", "build", "-o", f.binary, "./cmd/maestro-mcp")
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build: %s %v", out, err)
		}
		f.cli(t, "init", nil, false)
	} else {
		h, err := datahome.InitializeFresh(datahome.Options{ProductRoot: f.options.ProductRoot, UserHome: f.options.UserHome, LookupEnv: f.options.LookupEnv})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = workspace.Initialize(workspace.Options{WorkspacePath: f.options.WorkspacePath, DataRoot: filepath.Join(h.Home, "state")}); err != nil {
			t.Fatal(err)
		}
		if _, err = ownerctx.Initialize(filepath.Join(h.Home, "brain")); err != nil {
			t.Fatal(err)
		}
	}
	h, err := datahome.Resolve(datahome.Options{ProductRoot: f.options.ProductRoot, UserHome: f.options.UserHome, LookupEnv: f.options.LookupEnv})
	if err != nil {
		t.Fatal(err)
	}
	f.root = filepath.Join(h.Home, "brain")
	return f
}
func (f ownerFixture) cli(t *testing.T, command string, extra []string, fail bool) string {
	t.Helper()
	args := append([]string{command, "--product-root", f.options.ProductRoot, "--user-home", f.options.UserHome, "--workspace", f.options.WorkspacePath}, extra...)
	out, err := exec.Command(f.binary, args...).CombinedOutput()
	if (err != nil) != fail {
		t.Fatalf("CLI %s: %s %v", command, out, err)
	}
	return string(out)
}
func (f ownerFixture) connect(t *testing.T, handler func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)) *mcp.ClientSession {
	return f.connectProtocol(t, handler, "")
}
func (f ownerFixture) connectProtocol(t *testing.T, handler func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error), protocol string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "owner-test", Version: "1"}, &mcp.ClientOptions{ElicitationHandler: handler})
	var transport mcp.Transport
	if f.binary != "" {
		cmd := exec.Command(f.binary, "serve", "--product-root", f.options.ProductRoot, "--user-home", f.options.UserHome, "--workspace", f.options.WorkspacePath)
		transport = &mcp.CommandTransport{Command: cmd, TerminateDuration: time.Second}
	} else {
		b, err := product.Open(f.options)
		if err != nil {
			t.Fatal(err)
		}
		a, z := mcp.NewInMemoryTransports()
		transport = z
		ss, err := b.Server().Connect(context.Background(), a, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ss.Close() })
	}
	cs, err := client.Connect(context.Background(), transport, &mcp.ClientSessionOptions{ProtocolVersion: protocol})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func TestOwnerConfirmationRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, action                string
		content                     map[string]any
		unsupported, transportError bool
	}{
		{name: "unsupported", unsupported: true},
		{name: "decline", action: "decline"},
		{name: "cancel", action: "cancel"},
		{name: "nil-content", action: "accept"},
		{name: "false", action: "accept", content: map[string]any{"approved": false}},
		{name: "missing", action: "accept", content: map[string]any{}},
		{name: "wrong-type", action: "accept", content: map[string]any{"approved": "true"}},
		{name: "transport-error", transportError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newOwnerFixture(t, false)
			path := filepath.Join(f.root, "owner/self/personal-context.md")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var handler func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)
			if !tc.unsupported {
				handler = func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
					if tc.transportError {
						return nil, errors.New("synthetic transport failure")
					}
					return &mcp.ElicitResult{Action: tc.action, Content: tc.content}, nil
				}
			}
			cs := f.connect(t, handler)
			p := ownerCall(t, cs, "owner.propose", map[string]any{"facet": "personal-context", "evidence": "synthetic", "proposed_body": "private-owner-marker declined"}, false)
			out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_apply", Arguments: map[string]any{"operation": "owner.apply", "proposal_id": p["id"]}})
			if err == nil && (out == nil || !out.IsError) {
				t.Fatalf("unapproved mutation accepted: %+v", out)
			}
			if err != nil && strings.Contains(err.Error(), "private-owner-marker") {
				t.Fatal("private body leaked in error")
			}
			if out != nil {
				body, _ := json.Marshal(out)
				if strings.Contains(string(body), "private-owner-marker") {
					t.Fatal("private body leaked in refusal")
				}
			}
			after, err := os.ReadFile(path)
			if err != nil || string(before) != string(after) {
				t.Fatal("refusal mutated canonical facet")
			}
		})
	}
}

func seedOwnerAnswers(t *testing.T, f ownerFixture) {
	t.Helper()
	if _, err := ownerctx.SelectOnboardingTrack(f.root, "quick"); err != nil {
		t.Fatal(err)
	}
	for _, step := range ownerctx.QuickStartInterview().Steps {
		p, err := ownerctx.SubmitRefinement(f.root, ownerctx.RefinementInput{Facet: step.Facet, Evidence: "synthetic fixture", ProposedBody: "# Profile\n\n## Current\n\nprivate-owner-marker " + step.Facet + "\n"})
		if err != nil {
			t.Fatal(err)
		}
		r, err := ownerctx.ReviewRefinement(f.root, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = ownerctx.ApplyReviewedRefinement(f.root, p.ID, r.ReviewDigest, true); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOwnerChangedDuringTrustedReview(t *testing.T) {
	for _, kind := range []string{"base", "proposal", "profile", "track", "binding"} {
		t.Run(kind, func(t *testing.T) {
			f := newOwnerFixture(t, false)
			seedOwnerAnswers(t, f)
			p, err := ownerctx.SubmitRefinement(f.root, ownerctx.RefinementInput{Facet: "personal-context", Evidence: "synthetic", ProposedBody: "private-owner-marker proposed"})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(f.root, "owner/self/personal-context.md")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			called := false
			cs := f.connect(t, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				called = true
				switch kind {
				case "base", "profile":
					if err := os.WriteFile(path, []byte("newer canonical profile"), 0600); err != nil {
						return nil, err
					}
				case "proposal":
					proposal := filepath.Join(f.root, "owner/refinement/proposals", p.ID+".json")
					body, err := os.ReadFile(proposal)
					if err != nil {
						return nil, err
					}
					var v map[string]any
					if err = json.Unmarshal(body, &v); err != nil {
						return nil, err
					}
					v["proposed_body"] = "newer proposed profile"
					body, err = json.Marshal(v)
					if err != nil {
						return nil, err
					}
					if err = os.WriteFile(proposal, body, 0600); err != nil {
						return nil, err
					}
				case "track":
					if _, err := ownerctx.SelectOnboardingTrack(f.root, "complete"); err != nil {
						return nil, err
					}
				case "binding":
					if err := os.Rename(f.options.WorkspacePath, f.options.WorkspacePath+"-moved"); err != nil {
						return nil, err
					}
				}
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			})
			op := "owner.apply"
			extra := map[string]any{"proposal_id": p.ID}
			if kind == "profile" || kind == "track" {
				op = "owner.confirm_onboarding"
				extra = nil
			}
			ownerCall(t, cs, op, extra, true)
			if !called {
				t.Fatal("trusted review callback not reached")
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "base" || kind == "profile" {
				if string(after) != "newer canonical profile" {
					t.Fatal("stale approval overwrote newer canonical body")
				}
			} else if string(after) != string(before) {
				t.Fatal("stale approval changed canonical facet")
			}
			status, err := ownerctx.Inspect(f.root)
			if err != nil {
				t.Fatal(err)
			}
			if status.Onboarding.State == "complete" {
				t.Fatal("stale final approval completed onboarding")
			}
		})
	}
}

func TestOwnerLegacyElicitation(t *testing.T) {
	f := newOwnerFixture(t, false)
	called := false
	cs := f.connectProtocol(t, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		called = strings.Contains(req.Params.Message, "private-owner-marker")
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	}, "2025-11-25")
	p := ownerCall(t, cs, "owner.propose", map[string]any{"facet": "personal-context", "evidence": "synthetic", "proposed_body": "private-owner-marker legacy"}, false)
	ownerCall(t, cs, "owner.apply", map[string]any{"proposal_id": p["id"]}, false)
	if !called {
		t.Fatal("legacy client did not receive exact private review")
	}
}

func TestOwnerLocalFallback(t *testing.T) {
	f := newOwnerFixture(t, true)
	f.cli(t, "owner-track", []string{"--track", "quick"}, true)
	f.cli(t, "owner-track", []string{"--track", "quick", "--confirm"}, false)
	cs := f.connect(t, nil)
	interview := ownerCall(t, cs, "owner.interview", map[string]any{"track": "quick"}, false)
	for _, step := range interview["steps"].([]any) {
		facet := step.(map[string]any)["facet"].(string)
		p := ownerCall(t, cs, "owner.propose", map[string]any{"facet": facet, "evidence": "explicit synthetic interview", "proposed_body": "private-owner-marker local " + facet}, false)
		ownerCall(t, cs, "owner.apply", map[string]any{"proposal_id": p["id"]}, true)
		raw := f.cli(t, "owner-review", []string{"--proposal-id", p["id"].(string)}, false)
		var r ownerctx.RefinementReview
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(r.ProposedBody, "private-owner-marker") {
			t.Fatal("operator review omitted private body")
		}
		args := []string{"--proposal-id", r.ID, "--review-digest", r.ReviewDigest}
		f.cli(t, "owner-apply", args, true)
		f.cli(t, "owner-apply", append(args, "--confirm"), false)
	}
	raw := f.cli(t, "owner-onboarding-review", nil, false)
	var r ownerctx.OnboardingReview
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatal(err)
	}
	f.cli(t, "owner-confirm-onboarding", []string{"--review-digest", r.ReviewDigest}, true)
	f.cli(t, "owner-confirm-onboarding", []string{"--review-digest", strings.Repeat("0", 64), "--confirm"}, true)
	f.cli(t, "owner-confirm-onboarding", []string{"--review-digest", r.ReviewDigest, "--confirm"}, false)
	status := ownerCall(t, f.connect(t, nil), "owner.status", nil, false)
	if status["onboarding"].(map[string]any)["state"] != "complete" {
		t.Fatal("local confirmation did not survive restart")
	}
	f.cli(t, "serve", []string{"--confirm"}, true)
}

func TestOwnerRejectsModelAuthority(t *testing.T) {
	f := newOwnerFixture(t, false)
	cs := f.connect(t, nil)
	for _, field := range []string{"root", "brain_root", "workspace", "producer_id", "capability", "key", "confirmed", "sanitized", "review_digest", "approved"} {
		extra := map[string]any{"facet": "personal-context", "evidence": "synthetic", "proposed_body": "private-owner-marker forged", field: true}
		ownerCall(t, cs, "owner.propose", extra, true)
	}
	ownerCall(t, cs, "owner.propose", map[string]any{"facet": "psychological-profile", "evidence": "synthetic", "proposed_body": "private-owner-marker unsupported"}, true)
	entries, err := os.ReadDir(filepath.Join(f.root, "owner/refinement/proposals"))
	if !os.IsNotExist(err) && err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatal("forged argument persisted proposal")
	}
}

func TestOwnerUnsafeBindingBeforeEffects(t *testing.T) {
	for _, kind := range []string{"workspace", "home"} {
		t.Run(kind, func(t *testing.T) {
			f := newOwnerFixture(t, false)
			cs := f.connect(t, nil)
			path := f.options.WorkspacePath
			if kind == "home" {
				path = filepath.Dir(f.root)
			}
			moved := path + "-moved"
			if err := os.Rename(path, moved); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(moved, path); err != nil {
				t.Fatal(err)
			}
			ownerCall(t, cs, "owner.propose", map[string]any{"facet": "personal-context", "evidence": "synthetic", "proposed_body": "private-owner-marker unsafe binding"}, true)
			root := f.root
			if kind == "home" {
				root = filepath.Join(moved, "brain")
			}
			entries, err := os.ReadDir(filepath.Join(root, "owner/refinement/proposals"))
			if !os.IsNotExist(err) && err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatal("unsafe binding persisted proposal")
			}
		})
	}
}

func TestOwnerQueryNeverInitializes(t *testing.T) {
	f := newOwnerFixture(t, false)
	if err := os.RemoveAll(filepath.Join(f.root, "owner")); err != nil {
		t.Fatal(err)
	}
	cs := f.connect(t, nil)
	status := ownerCall(t, cs, "owner.status", nil, false)
	if status["initialized"] != false {
		t.Fatal("missing owner state reported initialized")
	}
	ownerCall(t, cs, "owner.interview", map[string]any{"track": "complete"}, false)
	if _, err := os.Lstat(filepath.Join(f.root, "owner")); !os.IsNotExist(err) {
		t.Fatal("query bootstrapped owner state")
	}
}

func TestOwnerTrackReviewCannotOverwriteConcurrentSelection(t *testing.T) {
	for _, initial := range []string{"quick", "complete"} {
		t.Run(initial, func(t *testing.T) {
			f := newOwnerFixture(t, false)
			if _, err := ownerctx.SelectOnboardingTrack(f.root, initial); err != nil {
				t.Fatal(err)
			}
			approved, validations, changed := false, 0, false
			f.options.LookupEnv = func(key string) (string, bool) {
				if approved && key == "MAESTRO_HOME" {
					validations++
					if validations == 4 {
						if _, err := ownerctx.SelectOnboardingTrack(f.root, "complete"); err != nil {
							t.Error(err)
						}
						changed = true
					}
				}
				return "", false
			}
			cs := f.connectProtocol(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				approved = true
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			}, "2025-11-25")
			out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_apply", Arguments: map[string]any{"operation": "owner.track", "track": "quick"}})
			if err != nil {
				t.Fatal(err)
			}
			if !changed {
				t.Fatalf("probe missed post-review selection seam: validations=%d", validations)
			}
			status, err := ownerctx.Inspect(f.root)
			if err != nil {
				t.Fatal(err)
			}
			if !out.IsError || status.Onboarding.Track != "complete" {
				t.Fatalf("stale approved track overwrote committed selection: is_error=%v track=%s validations=%d", out.IsError, status.Onboarding.Track, validations)
			}
		})
	}
}

func TestOwnerTrackTrustedForms(t *testing.T) {
	for _, protocol := range []string{"", "2025-11-25"} {
		for _, tc := range []struct {
			name, action         string
			concurrent, rejected bool
		}{
			{name: "current", action: "accept"},
			{name: "decline", action: "decline", rejected: true},
			{name: "same-value-commit", action: "accept", concurrent: true, rejected: true},
		} {
			t.Run(protocol+"/"+tc.name, func(t *testing.T) {
				f := newOwnerFixture(t, false)
				if _, err := ownerctx.SelectOnboardingTrack(f.root, "complete"); err != nil {
					t.Fatal(err)
				}
				cs := f.connectProtocol(t, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
					var view map[string]any
					if err := json.Unmarshal([]byte(req.Params.Message[strings.Index(req.Params.Message, "{"):]), &view); err != nil {
						return nil, err
					}
					if view["Trilha atual"] != "Completa" || view["Trilha escolhida"] != "Curta" || strings.Contains(req.Params.Message, "track_revision") || strings.Contains(req.Params.Message, "review_digest") {
						return nil, errors.New("trusted form did not show the domain track choice")
					}
					if tc.concurrent {
						if _, err := ownerctx.SelectOnboardingTrack(f.root, "complete"); err != nil {
							return nil, err
						}
					}
					return &mcp.ElicitResult{Action: tc.action, Content: map[string]any{"approved": true}}, nil
				}, protocol)
				ownerCall(t, cs, "owner.track", map[string]any{"track": "quick"}, tc.rejected)
				status, err := ownerctx.Inspect(f.root)
				if err != nil {
					t.Fatal(err)
				}
				want := "quick"
				if tc.rejected {
					want = "complete"
				}
				if status.Onboarding.Track != want {
					t.Fatalf("track=%s want=%s", status.Onboarding.Track, want)
				}
			})
		}
	}
}
func ownerCall(t *testing.T, cs *mcp.ClientSession, op string, extra map[string]any, fail bool) map[string]any {
	t.Helper()
	args := map[string]any{"operation": op}
	for k, v := range extra {
		args[k] = v
	}
	tool := "maestro_query"
	if strings.Contains("|owner.track|owner.propose|owner.apply|owner.confirm_onboarding|", "|"+op+"|") {
		tool = "maestro_apply"
	}
	out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError != fail {
		t.Fatalf("%s: %+v %v", op, out, out.Content[0])
	}
	text := out.Content[0].(*mcp.TextContent).Text
	if strings.Contains(text, "private-owner-marker") || strings.Contains(text, "owner/self/") {
		t.Fatalf("private body/path leaked: %s", text)
	}
	var v map[string]any
	if err = json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatal(err)
	}
	return v
}
func TestOwnerPublicJourney(t *testing.T) {
	runOwnerJourney(t, "quick")
}
func TestOwnerCompletePublicJourney(t *testing.T) {
	runOwnerJourney(t, "complete")
}
func runOwnerJourney(t *testing.T, track string) {
	for _, stdio := range []bool{false, true} {
		t.Run(fmt.Sprint(stdio), func(t *testing.T) {
			f := newOwnerFixture(t, stdio)
			var messages []string
			approve := func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				messages = append(messages, req.Params.Message)
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			}
			cs := f.connect(t, approve)
			state := ownerCall(t, cs, "owner.status", nil, false)
			if state["initialized"] != true {
				t.Fatalf("explicit setup missing owner state: %v", state)
			}
			ownerCall(t, cs, "owner.track", map[string]any{"track": track}, false)
			interview := ownerCall(t, cs, "owner.interview", map[string]any{"track": track}, false)
			for _, step := range interview["steps"].([]any) {
				facet := step.(map[string]any)["facet"].(string)
				body := "# Synthetic\n\n## Current\n\nprivate-owner-marker " + facet + ownerReviewFixtureSuffix
				p := ownerCall(t, cs, "owner.propose", map[string]any{"facet": facet, "evidence": "explicit synthetic interview", "proposed_body": body}, false)
				review := ownerCall(t, cs, "owner.review", map[string]any{"proposal_id": p["id"]}, false)
				if len(review["review_digest"].(string)) != 64 {
					t.Fatal("missing exact review digest")
				}
				ownerCall(t, cs, "owner.apply", map[string]any{"proposal_id": p["id"]}, false)
				var view map[string]any
				message := messages[len(messages)-1]
				if err := json.Unmarshal([]byte(message[strings.Index(message, "{"):]), &view); err != nil || view["Texto proposto"] != body || view["Evidência"] != "explicit synthetic interview" || view["Texto atual"] == "" || view["Faceta"] == "" || view["Sensibilidade"] == "" || view["Regra de atualização"] == "" || view["Leitores autorizados"] == nil {
					t.Fatalf("UI did not receive exact private review: %s %v", message, err)
				}
				for _, field := range []string{"base_sha256", "proposal_sha256", "review_digest"} {
					if strings.Contains(message, field) || strings.Contains(message, review[field].(string)) {
						t.Fatalf("control proof appeared in human review: %s", field)
					}
				}
			}
			ownerCall(t, cs, "owner.onboarding_review", nil, false)
			ownerCall(t, cs, "owner.confirm_onboarding", nil, false)
			if !strings.Contains(messages[len(messages)-1], "private-owner-marker personal-context") {
				t.Fatal("final review omitted sensitive facet")
			}
			message := messages[len(messages)-1]
			var final map[string]any
			if err := json.Unmarshal([]byte(message[strings.Index(message, "{"):]), &final); err != nil {
				t.Fatal(err)
			}
			facets, ok := final["Facetas"].([]any)
			if !ok || len(facets) != len(interview["steps"].([]any)) || strings.Contains(message, "review_digest") {
				t.Fatal("final human review omitted facets or displayed control proof")
			}
			seen := map[string]bool{}
			for _, facet := range facets {
				seen[facet.(map[string]any)["Texto do perfil"].(string)] = true
			}
			for _, step := range interview["steps"].([]any) {
				body := "# Synthetic\n\n## Current\n\nprivate-owner-marker " + step.(map[string]any)["facet"].(string) + ownerReviewFixtureSuffix
				if !seen[body] {
					t.Fatal("final human review did not preserve a full exact body")
				}
			}
			restarted := f.connect(t, approve)
			status := ownerCall(t, restarted, "owner.status", nil, false)
			if status["onboarding"].(map[string]any)["state"] != "complete" {
				t.Fatalf("restart lost confirmation: %v", status)
			}
		})
	}
}
