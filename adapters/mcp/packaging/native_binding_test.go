package packaging

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// This compiled local program only serves the public inspection protocol. It
// never calls Code, a provider or a model, and cannot supply native hook proof.
func controlledInspectionHost(t *testing.T, in NativeOptions, size int64) string {
	t.Helper()
	root := filepath.Dir(in.Packet.Destination)
	profile := filepath.Join(root, "code-profile")
	if err := os.Mkdir(profile, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MAESTRO_CONTROLLED_PROFILE", profile)
	if err := os.WriteFile(filepath.Join(profile, "settings.json"), []byte(`{"autoMemoryEnabled":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "inspection-host.go")
	body := `package main
import("encoding/json";"fmt";"os")
func main(){
 if len(os.Args)==2 && os.Args[1]=="--version" {fmt.Println("2.1.295 (controlled inspection fixture)");return}
 if len(os.Args)==3 && os.Args[1]=="auth" && os.Args[2]=="status" {json.NewEncoder(os.Stdout).Encode(map[string]any{"loggedIn":true,"authMethod":"controlled-fixture","configDirectory":os.Getenv("MAESTRO_CONTROLLED_PROFILE")});return}
 os.Exit(93)
}`
	if err := os.WriteFile(source, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	host := filepath.Join(root, "inspection-host")
	if output, err := exec.Command("go", "build", "-o", host, source).CombinedOutput(); err != nil {
		t.Fatalf("build harmless inspection host: %s: %v", output, err)
	}
	if size > 0 {
		if err := os.Truncate(host, size); err != nil {
			t.Fatal(err)
		}
	}
	return host
}

func assertNoNativeEffects(t *testing.T, in NativeOptions) {
	t.Helper()
	for _, path := range []string{in.Packet.Destination, in.ChallengeDirectory} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("review rejection created %s", path)
		}
	}
	entries, err := os.ReadDir(in.ScratchRoot)
	if err != nil || len(entries) != 0 {
		t.Fatal("review rejection allocated scratch")
	}
}

func TestNativePublicReviewBindsChangedInstructionClosureBeforeEffects(t *testing.T) {
	for _, source := range []string{"local", "import", "rule", "user_rule", "nested_local", "new_local", "new_rule", "negative_local", "git_local"} {
		t.Run(source, func(t *testing.T) {
			in := nativeFixture(t)
			in.Executable = controlledInspectionHost(t, in, 0)
			ws := in.Packet.Binding.WorkspacePath
			path := filepath.Join(ws, "CLAUDE.local.md")
			switch source {
			case "import":
				path = filepath.Join(ws, "docs", "transitive.md")
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(ws, "CLAUDE.md"), []byte("@docs/first.md\n"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(ws, "docs", "first.md"), []byte("@transitive.md\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case "rule":
				path = filepath.Join(ws, ".claude", "rules", "nested", "read.md")
			case "user_rule":
				path = filepath.Join(os.Getenv("MAESTRO_CONTROLLED_PROFILE"), "rules", "read.md")
			case "nested_local":
				path = filepath.Join(ws, "nested", "CLAUDE.local.md")
				in.ChallengeDirectory = filepath.Join(ws, "nested", "new-challenge")
			case "new_rule":
				path = filepath.Join(ws, ".claude", "rules", "new.md")
			case "negative_local":
				public := filepath.Join(filepath.Dir(in.Packet.Destination), "public-server")
				if err := os.Mkdir(public, 0700); err != nil {
					t.Fatal(err)
				}
				body, err := os.ReadFile(in.Packet.Server)
				if err != nil {
					t.Fatal(err)
				}
				in.Packet.Server = filepath.Join(public, "maestro-mcp")
				if err := os.WriteFile(in.Packet.Server, body, 0700); err != nil {
					t.Fatal(err)
				}
				path = filepath.Join(public, "CLAUDE.local.md")
			case "git_local":
				project := filepath.Dir(ws)
				makeNativeGitFixture(t, project)
				path = filepath.Join(project, ".claude", "settings.local.json")
			}
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			body := "Reviewed instruction.\n"
			if source == "git_local" {
				body = `{"autoMemoryEnabled":false}`
			}
			if source != "new_local" && source != "new_rule" {
				if err := os.WriteFile(path, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			review, err := ReviewNative(context.Background(), in)
			if err != nil || review.Plan.State != "reviewable" {
				t.Fatalf("public review unavailable: %+v %v", review, err)
			}
			body = "Changed instruction.\n"
			if source == "git_local" {
				body = `{"enabledPlugins":{"external@fixture":true}}`
			}
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			_, err = QualifyAndBuildNative(context.Background(), in, review.ReviewDigest, true)
			if err == nil || !strings.Contains(err.Error(), "review changed") {
				t.Errorf("changed %s instruction accepted: %v", source, err)
			}
			assertNoNativeEffects(t, in)
		})
	}
}

func makeNativeGitFixture(t *testing.T, root string) {
	t.Helper()
	for _, name := range []string{"objects", "refs/heads"} {
		if err := os.MkdirAll(filepath.Join(root, ".git", name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range map[string]string{"HEAD": "ref: refs/heads/main\n", "config": "[core]\nrepositoryformatversion = 0\nbare = false\n"} {
		if err := os.WriteFile(filepath.Join(root, ".git", name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNativePublicReviewRefusesUnsupportedInstructionSources(t *testing.T) {
	for _, kind := range []string{"cycle", "depth", "file_size", "closure_size", "syntax", "rule_alias", "source_alias", "default_memory", "null_memory", "imported_settings_policy", "inactive_user_local", "plugin", "worktree", "bare_git", "root_plugin"} {
		t.Run(kind, func(t *testing.T) {
			in := nativeFixture(t)
			in.Executable = controlledInspectionHost(t, in, 0)
			ws := in.Packet.Binding.WorkspacePath
			write := func(path, body string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			memory := filepath.Join(ws, "CLAUDE.md")
			switch kind {
			case "cycle":
				write(memory, "@second.md\n")
				write(filepath.Join(ws, "second.md"), "@CLAUDE.md\n")
			case "depth":
				write(memory, "@hop0.md\n")
				for i := 0; i < 5; i++ {
					body := "end\n"
					if i < 4 {
						body = fmt.Sprintf("@hop%d.md\n", i+1)
					}
					write(filepath.Join(ws, fmt.Sprintf("hop%d.md", i)), body)
				}
			case "file_size":
				write(memory, strings.Repeat("x", (256<<10)+1))
			case "closure_size":
				imports := ""
				for i := 0; i < 9; i++ {
					imports += fmt.Sprintf("@part%d.md\n", i)
					write(filepath.Join(ws, fmt.Sprintf("part%d.md", i)), strings.Repeat("x", 256<<10))
				}
				write(memory, imports)
			case "syntax":
				write(memory, "@\"ambiguous path.md\"\n")
			case "rule_alias", "source_alias":
				target := filepath.Join(ws, "real.md")
				write(target, "normal\n")
				link := memory
				if kind == "rule_alias" {
					link = filepath.Join(ws, ".claude", "rules", "alias.md")
					if err := os.MkdirAll(filepath.Dir(link), 0700); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.Symlink(target, link); err != nil {
					t.Fatal(err)
				}
			case "default_memory":
				write(filepath.Join(os.Getenv("MAESTRO_CONTROLLED_PROFILE"), "settings.json"), "{}")
			case "null_memory":
				write(filepath.Join(os.Getenv("MAESTRO_CONTROLLED_PROFILE"), "settings.json"), `{"autoMemoryEnabled":null}`)
			case "inactive_user_local":
				write(filepath.Join(os.Getenv("MAESTRO_CONTROLLED_PROFILE"), "settings.json"), "{}")
				write(filepath.Join(os.Getenv("MAESTRO_CONTROLLED_PROFILE"), "settings.local.json"), `{"autoMemoryEnabled":false}`)
			case "imported_settings_policy":
				profile := os.Getenv("MAESTRO_CONTROLLED_PROFILE")
				write(filepath.Join(profile, "CLAUDE.md"), "@settings.json\n")
				write(filepath.Join(profile, "settings.json"), `{"autoMemoryEnabled":false,"enabledPlugins":{"external\u0040fixture":true}}`)
				write(filepath.Join(ws, ".claude", "settings.json"), `{"autoMemoryEnabled":false}`)
			case "plugin":
				write(filepath.Join(ws, ".claude", "settings.json"), `{"enabledPlugins":{"external@fixture":true}}`)
			case "worktree":
				write(filepath.Join(ws, ".git"), "gitdir: /unreviewed/main-checkout/.git/worktrees/fixture\n")
			case "bare_git":
				makeNativeGitFixture(t, filepath.Dir(ws))
				write(filepath.Join(filepath.Dir(ws), ".git", "config"), "[core]\nrepositoryformatversion = 0\nbare = true\n")
			case "root_plugin":
				makeNativeGitFixture(t, filepath.Dir(ws))
				write(filepath.Join(filepath.Dir(ws), ".claude", "settings.local.json"), `{"enabledPlugins":{"external@fixture":true}}`)
			}
			review, err := ReviewNative(context.Background(), in)
			if err != nil || review.Plan.State != "unavailable" || !strings.HasPrefix(review.Plan.Reason, "native_instruction_") {
				t.Fatalf("unsupported %s source admitted: %+v %v", kind, review, err)
			}
			assertNoNativeEffects(t, in)
		})
	}
}

func TestNativeHostInspectionPreservesPayloadAndIdentityRefusals(t *testing.T) {
	in := nativeFixture(t)
	in.Executable = controlledInspectionHost(t, in, 0)
	review, err := ReviewNative(context.Background(), in)
	if err != nil || review.Plan.State != "reviewable" {
		t.Fatalf("initial controlled review: %+v %v", review, err)
	}
	f, err := os.OpenFile(in.Executable, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := f.Write([]byte("changed-identity"))
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal("mutate controlled host")
	}
	if _, err := QualifyAndBuildNative(context.Background(), in, review.ReviewDigest, true); err == nil {
		t.Fatal("changed host consumed old review")
	}
	assertNoNativeEffects(t, in)
	for _, kind := range []string{"script", "invalid", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(filepath.Dir(in.Executable), kind)
			body := "not an executable"
			if kind == "script" {
				body = "#!/bin/sh\nexit 99\n"
			}
			if err := os.WriteFile(path, []byte(body), 0700); err != nil {
				t.Fatal(err)
			}
			if kind == "oversized" {
				// Keep a real compiled native header so format rejection cannot hide
				// a regression of the independent host size bound.
				native, err := os.ReadFile(in.Executable)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, native, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Truncate(path, (512<<20)+1); err != nil {
					t.Fatal(err)
				}
			}
			changed := in
			changed.Executable = path
			r, err := ReviewNative(context.Background(), changed)
			if err != nil || r.Plan.Reason != "native_executable_unavailable" {
				t.Fatalf("invalid host admitted: %+v %v", r, err)
			}
		})
	}
	if err := os.Truncate(in.Executable, 239695888); err != nil {
		t.Fatal(err)
	}
	packet := in.Packet
	packet.Server = in.Executable
	if err := BuildPlugin(packet); err == nil {
		t.Fatal("host-size exception weakened server payload bound")
	}
	assertNoNativeEffects(t, in)
}

func TestNativeControlInstructionAppearingStopsBeforeNextSession(t *testing.T) {
	in := nativeFixture(t)
	sessions := 0
	run := inspectionRunner(t, true, func(ctx context.Context, args []string, cwd string) ([]byte, error) {
		sessions++
		probe := probeFromArguments(t, in, args, cwd)
		stream := syntheticNativeStream(t, probe)
		if sessions == 1 {
			path := filepath.Join(probe.challenge, ".claude", "rules", "new.md")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("New instruction in the actual control ancestry.\n"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		return stream, nil
	})
	review, err := reviewNative(context.Background(), in, run)
	if err != nil || review.Plan.State != "reviewable" {
		t.Fatalf("initial engineering review: %+v %v", review, err)
	}
	result, err := qualifyAndBuildNative(context.Background(), in, review.ReviewDigest, true, run, "engineering_fixture")
	if err == nil || sessions != 1 || result.CallsAttempted != 1 {
		t.Fatalf("changed control ancestry did not stop before next session: sessions=%d result=%+v error=%v", sessions, result, err)
	}
	if _, err := os.Lstat(in.Packet.Destination); !os.IsNotExist(err) {
		t.Fatal("changed control ancestry minted a packet")
	}
}

func TestNativeUnsupportedProfileRetainsMCPOnlyAndRootAliasBinding(t *testing.T) {
	in := nativeFixture(t)
	in.Executable = controlledInspectionHost(t, in, 0)
	profile := os.Getenv("MAESTRO_CONTROLLED_PROFILE")
	real := profile + "-real"
	if err := os.Rename(profile, real); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, profile); err != nil {
		t.Fatal(err)
	}
	review, err := ReviewNative(context.Background(), in)
	if err != nil || review.Plan.State != "reviewable" {
		t.Fatalf("selected compatibility root unavailable: %+v %v", review, err)
	}
	if err := os.WriteFile(filepath.Join(real, "settings.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := QualifyAndBuildNative(context.Background(), in, review.ReviewDigest, true); err == nil {
		t.Fatal("changed resolved profile consumed old review")
	}
	assertNoNativeEffects(t, in)
	unsupported, err := ReviewNative(context.Background(), in)
	if err != nil || unsupported.Plan.Reason != "native_instruction_auto_memory_unsupported" {
		t.Fatalf("implicit memory not refused: %+v %v", unsupported, err)
	}
	result, err := QualifyAndBuildNative(context.Background(), in, unsupported.ReviewDigest, true)
	if err != nil || result.State != "unavailable" || result.CallsAttempted != 0 {
		t.Fatalf("unsupported profile lost MCP fallback: %+v %v", result, err)
	}
	for _, path := range []string{in.ChallengeDirectory, filepath.Join(in.Packet.Destination, "agents"), filepath.Join(in.Packet.Destination, "hooks")} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("unsupported profile activated %s", path)
		}
	}
	entries, err := os.ReadDir(in.ScratchRoot)
	if err != nil || len(entries) != 0 {
		t.Fatal("unsupported profile allocated native scratch")
	}
	if _, err := os.Stat(filepath.Join(in.Packet.Destination, "maestro-packet.json")); err != nil {
		t.Fatal("useful MCP-only packet missing")
	}
}

func TestNativePublicReviewInspectsSupportedSizeCompiledHost(t *testing.T) {
	in := nativeFixture(t)
	// Exact byte size independently observed in the first-party minimum-release
	// Darwin-arm64 artifact. Padding only enlarges our harmless local executable.
	in.Executable = controlledInspectionHost(t, in, 239695888)
	review, err := ReviewNative(context.Background(), in)
	if err != nil || review.Plan.State != "reviewable" || review.Plan.HostSHA256 == "" {
		t.Fatalf("supported-size compiled host refused: %+v %v", review, err)
	}
	assertNoNativeEffects(t, in)
}
