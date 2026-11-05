//go:build windows

package product_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestWindowsOwnerReviewPilot(t *testing.T) {
	windowsPrivateOrdinaryUser(t)
	for _, track := range []string{"quick", "complete"} {
		t.Run(track, func(t *testing.T) {
			root := windowsPrivateRoot(t)
			product, home, workspace := filepath.Join(root, "product"), filepath.Join(root, "owner"), filepath.Join(root, "workspace")
			for _, path := range []string{product, home} {
				if e := os.Mkdir(path, 0700); e != nil {
					t.Fatal(e)
				}
			}
			server := os.Getenv("MAESTRO_WINDOWS_CORE_SERVER")
			args := []string{"--product-root", product, "--user-home", home, "--workspace", workspace}
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
			defer cancel()
			if out, e := exec.CommandContext(ctx, server, append([]string{"init"}, args...)...).CombinedOutput(); e != nil {
				t.Fatalf("public native owner setup: %s %v", out, e)
			}
			managed := windowsCoreTree(t, product)
			var approved atomic.Bool
			approved.Store(true)
			var reviewing, intervene string
			reviews := 0
			connect := func() *mcp.ClientSession {
				client := mcp.NewClient(&mcp.Implementation{Name: "windows-owner-native-proof", Version: "1"}, &mcp.ClientOptions{ElicitationHandler: func(_ context.Context, r *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
					reviews++
					if reviewing != "" && !strings.Contains(r.Params.Message, reviewing) {
						t.Error("trusted native host did not receive exact complete proposed text")
						return &mcp.ElicitResult{Action: "decline"}, nil
					}
					if intervene != "" {
						if e := scheduler.WritePrivateFileAtomic(filepath.Join(home, ".maestro", "brain", "owner", "self", "personal-context.md"), []byte(intervene)); e != nil {
							return nil, e
						}
						intervene = ""
					}
					if approved.Load() {
						return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
					}
					return &mcp.ElicitResult{Action: "decline"}, nil
				}})
				cs, e := client.Connect(ctx, &mcp.CommandTransport{Command: exec.CommandContext(ctx, server, append([]string{"serve"}, args...)...), TerminateDuration: time.Second}, nil)
				if e != nil {
					t.Fatal(e)
				}
				t.Cleanup(func() { cs.Close() })
				return cs
			}
			cs := connect()
			ownerCall(t, cs, "owner.track", map[string]any{"track": track}, false)
			interview := ownerCall(t, cs, "owner.interview", map[string]any{"track": track}, false)
			steps, ok := interview["steps"].([]any)
			if !ok || len(steps) < 2 {
				t.Fatal("native public interview omitted canonical facets")
			}
			for index, step := range steps {
				facet := step.(map[string]any)["facet"].(string)
				body := "# Synthetic\n\n## Current\n\nWINDOWS_OWNER_CURRENT " + track + " " + facet + "\n"
				p := ownerCall(t, cs, "owner.propose", map[string]any{"facet": facet, "evidence": "explicit synthetic native owner interview", "proposed_body": body}, false)
				reviewing = body
				if index == 0 {
					approved.Store(false)
					before := windowsCoreTree(t, home)
					ownerCall(t, cs, "owner.apply", map[string]any{"proposal_id": p["id"]}, true)
					if !reflect.DeepEqual(before, windowsCoreTree(t, home)) {
						t.Fatal("declined native owner review changed private state")
					}
					approved.Store(true)
				}
				ownerCall(t, cs, "owner.apply", map[string]any{"proposal_id": p["id"]}, false)
				reviewing = ""
			}
			ownerCall(t, cs, "owner.confirm_onboarding", nil, false)
			if e := cs.Close(); e != nil {
				t.Fatal(e)
			}
			cs = connect()
			state := ownerCall(t, cs, "owner.status", nil, false)
			if state["onboarding"].(map[string]any)["state"] != "complete" {
				t.Fatal("native reviewed onboarding did not survive restart", state)
			}
			// A real intervening private publication invalidates the already-shown review.
			p := ownerCall(t, cs, "owner.propose", map[string]any{"facet": "personal-context", "evidence": "explicit synthetic stale-review proof", "proposed_body": "WINDOWS_OWNER_STALE must never overwrite newer work"}, false)
			reviewing = "WINDOWS_OWNER_STALE must never overwrite newer work"
			later := "# Synthetic\n\n## Current\n\nWINDOWS_OWNER_LATER preserve this exact body\n"
			intervene = later
			ownerCall(t, cs, "owner.apply", map[string]any{"proposal_id": p["id"]}, true)
			actual, e := scheduler.ReadPrivateFile(filepath.Join(home, ".maestro", "brain", "owner", "self", "personal-context.md"), 12<<10)
			if e != nil || string(actual) != later {
				t.Fatal("stale native owner approval erased newer work", e)
			}
			if reviews < len(steps)+3 {
				t.Fatal("native trusted review interactions were bypassed")
			}
			if !reflect.DeepEqual(managed, windowsCoreTree(t, product)) {
				t.Fatal("native owner journey modified the managed product")
			}
			t.Logf("native_owner_review track=%s facets=%d restart=true declined_bytes_unchanged=true stale_preserves_newer=true", track, len(steps))
		})
	}
}
