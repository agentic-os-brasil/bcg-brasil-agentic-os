//go:build !windows

package product

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/privatelock"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func scopeEffectFixture(t *testing.T) (*Bound, Options, *mcp.ClientSession) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	options := Options{ProductRoot: filepath.Join(base, "product"), UserHome: filepath.Join(base, "user"), WorkspacePath: filepath.Join(base, "workspace"), LookupEnv: func(string) (string, bool) { return "", false }}
	for _, path := range []string{options.ProductRoot, options.UserHome} {
		if err = os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err = os.Chmod(options.ProductRoot, 0500); err != nil {
		t.Fatal(err)
	}
	h, err := datahome.InitializeFresh(datahome.Options{ProductRoot: options.ProductRoot, UserHome: options.UserHome, LookupEnv: options.LookupEnv})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = workspace.Initialize(workspace.Options{WorkspacePath: options.WorkspacePath, DataRoot: filepath.Join(h.Home, "state")}); err != nil {
		t.Fatal(err)
	}
	if _, err = ownerctx.Initialize(filepath.Join(h.Home, "brain")); err != nil {
		t.Fatal(err)
	}
	bound, err := Open(options)
	if err != nil {
		t.Fatal(err)
	}
	a, z := mcp.NewInMemoryTransports()
	server, err := bound.Server().Connect(context.Background(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "held-scope-sdk-proof", Version: "1"}, &mcp.ClientOptions{ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	}})
	session, err := client.Connect(context.Background(), z, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return bound, options, session
}
func effectTool(t *testing.T, session *mcp.ClientSession, operation string, args map[string]any) map[string]any {
	t.Helper()
	args["operation"] = operation
	out, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_apply", Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatal(out.Content[0].(*mcp.TextContent).Text)
	}
	var value map[string]any
	if err = json.Unmarshal([]byte(out.Content[0].(*mcp.TextContent).Text), &value); err != nil {
		t.Fatal(err)
	}
	return value
}
func effectBody(workspace, page, title, scope string) string {
	sensitivity := "internal"
	if strings.HasPrefix(page, "accounts/") {
		sensitivity = "client-confidential"
	}
	return "---\nid: " + strings.TrimSuffix(page, ".md") + "\ntitle: " + title + "\nsummary: A synthetic reviewed endpoint.\ntype: project-brief\nscope: " + scope + "\nstatus: draft\nsensitivity: " + sensitivity + "\nupdated: 2026-11-06\n---\n# " + title + "\n\n## Related\n"
}
func effectJournal(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	result := map[string][32]byte{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			name, _ := filepath.Rel(root, path)
			result[name] = sha256.Sum256(body)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestPublicLinkRechecksTargetAfterHeldCorpusLock(t *testing.T) {
	for _, mode := range []string{"write", "replay"} {
		for _, change := range []string{"case-revoked", "target-replaced", "target-removed"} {
			t.Run(mode+"/"+change, func(t *testing.T) {
				bound, options, session := scopeEffectFixture(t)
				brain := filepath.Join(options.WorkspacePath, "brain")
				marker := filepath.Join(brain, "accounts", ".active")
				if err := os.MkdirAll(filepath.Dir(marker), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(marker, []byte("a/selected\n"), 0600); err != nil {
					t.Fatal(err)
				}
				const source, target = "projects/held-link.md", "accounts/a/cases/selected/target.md"
				sourceBody := effectBody(bound.workspaceID, source, "Source", "workspace/"+bound.workspaceID)
				targetBody := effectBody(bound.workspaceID, target, "Target", "account/a/case/selected")
				created := effectTool(t, session, "brain.create", map[string]any{"page": source, "text": sourceBody})
				effectTool(t, session, "brain.create", map[string]any{"page": target, "text": targetBody})
				args := map[string]any{"operation": "brain.link", "page": source, "section": "## Related", "target": target, "label": "Reviewed target", "expected_page_revision": created["revision"]}
				if mode == "replay" {
					effectTool(t, session, "brain.link", args)
				}
				sourcePath, targetPath := filepath.Join(brain, filepath.FromSlash(source)), filepath.Join(brain, filepath.FromSlash(target))
				beforeSource, err := os.ReadFile(sourcePath)
				if err != nil {
					t.Fatal(err)
				}
				journal := filepath.Join(brain, ".atlasops")
				release, err := privatelock.Acquire(filepath.Join(journal, "grants.lock"))
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					if release != nil {
						_ = release()
					}
				}()
				beforeJournal := effectJournal(t, journal)
				var armed, seen atomic.Bool
				reached := make(chan struct{})
				bound.observeWorkspaceTransition = func(stage, page string) {
					if stage == "reviewed" {
						armed.Store(true)
					}
					if stage == "admitted" && page == target && armed.Load() && seen.CompareAndSwap(false, true) {
						close(reached)
					}
				}
				type response struct {
					out *mcp.CallToolResult
					err error
				}
				finished := make(chan response, 1)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				go func() {
					out, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "maestro_apply", Arguments: args})
					finished <- response{out, err}
				}()
				select {
				case <-reached:
				case <-time.After(2 * time.Second):
					t.Fatal("link did not reach its final pre-lock target admission")
				}
				// The private observer has no authority or failure return. This real held
				// corpus lock prevents effects while the request advances past admission.
				select {
				case got := <-finished:
					t.Fatalf("link finished while the actual corpus lock was held: %v", got.err)
				case <-time.After(75 * time.Millisecond):
				}
				expectedTarget := targetBody
				switch change {
				case "case-revoked":
					if err = os.WriteFile(marker, []byte("a/other\n"), 0600); err != nil {
						t.Fatal(err)
					}
				case "target-replaced":
					expectedTarget = targetBody + "\nPreserve independently newer target bytes.\n"
					if err = os.WriteFile(targetPath, []byte(expectedTarget), 0600); err != nil {
						t.Fatal(err)
					}
				case "target-removed":
					if err = os.Remove(targetPath); err != nil {
						t.Fatal(err)
					}
				}
				if err = release(); err != nil {
					t.Fatal(err)
				}
				release = nil
				got := <-finished
				if got.err != nil {
					t.Fatal(got.err)
				}
				if !got.out.IsError {
					t.Fatalf("link accepted changed %s after waiting for the actual corpus lock: %s", change, got.out.Content[0].(*mcp.TextContent).Text)
				}
				afterSource, err := os.ReadFile(sourcePath)
				if err != nil || string(afterSource) != string(beforeSource) {
					t.Fatal("refused endpoint changed source", err)
				}
				if change == "target-removed" {
					if _, err = os.Stat(targetPath); !os.IsNotExist(err) {
						t.Fatal("removed target was recreated", err)
					}
				} else {
					afterTarget, err := os.ReadFile(targetPath)
					if err != nil || string(afterTarget) != expectedTarget {
						t.Fatal("refused endpoint changed target", err)
					}
				}
				if !reflect.DeepEqual(beforeJournal, effectJournal(t, journal)) {
					t.Fatal("refused endpoint finalized or changed journal")
				}
			})
		}
	}
}
