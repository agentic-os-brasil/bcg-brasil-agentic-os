package product

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func localReviewFixture(t *testing.T, channel string) *Bound {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	options := Options{ProductRoot: filepath.Join(root, "product"), UserHome: filepath.Join(root, "owner"), WorkspacePath: filepath.Join(root, "workspace"), OwnerReviewChannel: channel, LookupEnv: func(string) (string, bool) { return "", false }}
	for _, directory := range []string{options.ProductRoot, options.UserHome} {
		if err := os.Mkdir(directory, 0700); err != nil {
			t.Fatal(err)
		}
	}
	home, err := datahome.InitializeFresh(datahome.Options{ProductRoot: options.ProductRoot, UserHome: options.UserHome, LookupEnv: options.LookupEnv})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: options.WorkspacePath, DataRoot: filepath.Join(home.Home, "state")}); err != nil {
		t.Fatal(err)
	}
	if _, err := ownerctx.Initialize(filepath.Join(home.Home, "brain")); err != nil {
		t.Fatal(err)
	}
	b, err := Open(options)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func localReviewSession(t *testing.T, b *Bound, options *mcp.ClientOptions) *mcp.ClientSession {
	t.Helper()
	a, z := mcp.NewInMemoryTransports()
	server, err := b.Server().Connect(context.Background(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	client, err := mcp.NewClient(&mcp.Implementation{Name: "local-owner-review-test", Version: "1"}, options).Connect(context.Background(), z, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}
func localTrack(t *testing.T, client *mcp.ClientSession) *mcp.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "maestro_apply", Arguments: map[string]any{"operation": "owner.track", "track": "quick"}})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func ownerState(t *testing.T, b *Bound) string {
	t.Helper()
	state, err := ownerctx.Inspect(b.ownerRoot())
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
func TestLocalOwnerReviewRoutingAndCanonicalApply(t *testing.T) {
	for _, mode := range []string{"unconfigured", "fallback", "url_only", "decline", "opener_error", "native_first", "native_decline", "native_error", "stale"} {
		t.Run(mode, func(t *testing.T) {
			channel := "local-browser"
			if mode == "unconfigured" {
				channel = ""
			}
			b := localReviewFixture(t, channel)
			before := ownerState(t, b)
			localCalls, nativeCalls := 0, 0
			b.localOwnerReview = func(_ context.Context, message string, schema any) (bool, error) {
				localCalls++
				if !strings.Contains(message, "Trilha escolhida") || !strings.Contains(message, "Trilha atual") || schema == nil {
					t.Error("full current presentation/schema missing")
				}
				if mode == "opener_error" {
					return false, errors.New("private opener marker")
				}
				if mode == "stale" {
					if _, err := b.SelectOwnerTrack("complete", true); err != nil {
						t.Fatal(err)
					}
				}
				return mode != "decline", nil
			}
			options := &mcp.ClientOptions{}
			if mode == "url_only" {
				options.Capabilities = &mcp.ClientCapabilities{Elicitation: &mcp.ElicitationCapabilities{URL: &mcp.URLElicitationCapabilities{}}}
			}
			if strings.HasPrefix(mode, "native_") {
				options.ElicitationHandler = func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
					nativeCalls++
					if mode == "native_error" {
						return nil, errors.New("synthetic native failure")
					}
					action := "accept"
					if mode == "native_decline" {
						action = "decline"
					}
					return &mcp.ElicitResult{Action: action, Content: map[string]any{"approved": true}}, nil
				}
			}
			var diagnostic *os.File
			originalStderr := os.Stderr
			if mode == "opener_error" {
				var err error
				diagnostic, err = os.CreateTemp(t.TempDir(), "confirmation-log")
				if err != nil {
					t.Fatal(err)
				}
				os.Stderr = diagnostic
				defer func() { os.Stderr = originalStderr; diagnostic.Close() }()
			}
			client := localReviewSession(t, b, options)
			var out *mcp.CallToolResult
			if mode == "native_error" {
				// Modern MRTR propagates the host's failure before a second server round trip.
				_, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_apply", Arguments: map[string]any{"operation": "owner.track", "track": "quick"}})
				if err == nil || !strings.Contains(err.Error(), "synthetic native failure") {
					t.Fatalf("native error was not preserved: %v", err)
				}
				out = &mcp.CallToolResult{IsError: true}
			} else {
				out = localTrack(t, client)
			}
			success := mode == "fallback" || mode == "url_only" || mode == "native_first"
			if out.IsError == success {
				t.Fatalf("unexpected result %+v", out)
			}
			wantLocal := 1
			if mode == "unconfigured" || strings.HasPrefix(mode, "native_") {
				wantLocal = 0
			}
			if localCalls != wantLocal {
				t.Errorf("local calls %d, want %d", localCalls, wantLocal)
			}
			if strings.HasPrefix(mode, "native_") && nativeCalls != 1 {
				t.Errorf("native calls %d", nativeCalls)
			}
			if diagnostic != nil {
				os.Stderr = originalStderr
				data, err := os.ReadFile(diagnostic.Name())
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(data), "stage=local_browser_unavailable") || strings.Contains(string(data), "private") || strings.Contains(string(data), "127.0.0.1") || strings.Contains(string(data), "/review/") {
					t.Fatalf("unsafe diagnostic: %q", data)
				}
			}
			after := ownerState(t, b)
			if !success && mode != "stale" && before != after {
				t.Error("failed review changed canonical owner")
			}
			if success && before == after {
				t.Error("approved operation did not reach canonical owner")
			}
			if mode == "stale" && !strings.Contains(after, "complete") {
				t.Error("stale review overwrote newer selection")
			}
			body, _ := json.Marshal(out)
			if strings.Contains(string(body), "127.0.0.1") || strings.Contains(string(body), "private opener marker") {
				t.Error("private review channel leaked")
			}
		})
	}
}
func TestLocalOwnerReviewRequiresForegroundRequest(t *testing.T) {
	b := localReviewFixture(t, "local-browser")
	calls := 0
	b.localOwnerReview = func(context.Context, string, any) (bool, error) { calls++; return true, nil }
	requests := []*mcp.CallToolRequest{nil, {}, {Params: &mcp.CallToolParamsRaw{}}, {Params: &mcp.CallToolParamsRaw{Name: "maestro_apply"}}}
	for _, req := range requests {
		if err := b.confirm(context.Background(), req, "question", map[string]string{"private": "record"}, "presentation"); !errors.Is(err, errConfirmationUnavailable) {
			t.Errorf("background/nil request granted authority: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := b.confirm(ctx, nil, "question", nil, nil); !errors.Is(err, errConfirmationUnavailable) {
		t.Error(err)
	}
	if calls != 0 {
		t.Fatal("nonforeground request opened local review")
	}
	if _, err := Open(Options{OwnerReviewChannel: "model-approved"}); err == nil {
		t.Fatal("unknown channel admitted")
	}
}

func TestGammaNativeDeclineAfterCapabilityLossDoesNotFallback(t *testing.T) {
	b := localReviewFixture(t, "local-browser")
	localCalls := 0
	b.localOwnerReview = func(context.Context, string, any) (bool, error) { localCalls++; return false, nil }
	a, z := mcp.NewInMemoryTransports()
	ss, err := b.Server().Connect(context.Background(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "gamma-regression", Version: "1"}, &mcp.ClientOptions{}).Connect(context.Background(), z, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	question := "Exact native review"
	review := map[string]string{"test": "same reviewed record"}
	envelope, _ := json.MarshalIndent(review, "", "  ")
	key := fmt.Sprintf("owner-review-%x", sha256.Sum256([]byte(b.home+"\x00"+b.workspaceID+"\x00"+question+"\x00"+string(envelope))))
	req := &mcp.CallToolRequest{Session: ss, Params: &mcp.CallToolParamsRaw{Name: "maestro_apply", InputResponses: mcp.InputResponseMap{key: &mcp.ElicitResult{Action: "decline", Content: map[string]any{"approved": false}}}}}
	if form, _ := confirmationModes(req.ClientCapabilities()); form {
		t.Fatal("fixture unexpectedly declares a native form")
	}
	err = b.confirm(context.Background(), req, question, review, review)
	if localCalls != 0 {
		t.Fatalf("native decline switched to local browser confirmation: local_calls=%d, result=%v", localCalls, err)
	}
	if !errors.Is(err, errConfirmationDeclined) && !errors.Is(err, errConfirmationUnavailable) {
		t.Fatalf("continuation must fail closed: %v", err)
	}
}

func TestLocalOwnerReviewCapabilityLossContinuationCannotApply(t *testing.T) {
	for _, mode := range []string{"absent", "url_only"} {
		for _, action := range []string{"decline", "accept"} {
			t.Run(mode+"/"+action, func(t *testing.T) {
				b := localReviewFixture(t, "local-browser")
				before := ownerState(t, b)
				localCalls := 0
				b.localOwnerReview = func(context.Context, string, any) (bool, error) { localCalls++; return true, nil }
				client := localReviewSession(t, b, &mcp.ClientOptions{MultiRoundTrip: &mcp.MultiRoundTripOptions{Disabled: true}, ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
					t.Fatal("manual continuation unexpectedly invoked native handler")
					return nil, nil
				}})
				args := map[string]any{"operation": "owner.track", "track": "quick"}
				first, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_apply", Arguments: args})
				if err != nil || first.IsError || len(first.InputRequests) != 1 {
					t.Fatalf("native first round: %+v %v", first, err)
				}
				var key string
				for id := range first.InputRequests {
					key = id
				}
				capabilities := map[string]any{}
				if mode == "url_only" {
					capabilities["elicitation"] = map[string]any{"url": map[string]any{}}
				}
				out, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_apply", Arguments: args, InputResponses: mcp.InputResponseMap{key: &mcp.ElicitResult{Action: action, Content: map[string]any{"approved": action == "accept"}}}, Meta: mcp.Meta{mcp.MetaKeyClientCapabilities: capabilities}})
				if err != nil || out == nil || !out.IsError {
					t.Fatalf("withdrawn form continuation did not fail closed: %+v %v", out, err)
				}
				if localCalls != 0 {
					t.Errorf("native continuation opened local review %d times", localCalls)
				}
				if ownerState(t, b) != before {
					t.Error("withdrawn form continuation changed canonical owner")
				}
			})
		}
	}
}
