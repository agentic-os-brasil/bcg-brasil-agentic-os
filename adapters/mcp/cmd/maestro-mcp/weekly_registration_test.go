package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
)

func weeklyCLIRegistration(t *testing.T) maintenance.WeeklyModelRegistration {
	t.Helper()
	executable, err := filepath.Abs("synthetic-claude")
	if err != nil {
		t.Fatal(err)
	}
	r := maintenance.WeeklyModelRegistration{
		SchemaVersion: 1,
		Descriptor: maintenance.WeeklyModelDescriptor{
			SchemaVersion: 1, JobID: maintenance.YodaSelfReviewWeeklyJobID,
			Runtime: "claude_code_cli", RuntimeVersion: "synthetic-version",
			Executable: executable, ExecutableSHA256: strings.Repeat("a", 64),
			ConfigurationSHA256: strings.Repeat("b", 64), ManagedPolicySHA256: strings.Repeat("c", 64),
			ProfileReferenceSHA256: strings.Repeat("d", 64), TransportSHA256: strings.Repeat("e", 64),
			Provider: "synthetic-provider", Model: "synthetic-model", CredentialMode: "runtime_owned_session", ValidatorVersion: "weekly-self-v1",
			Limits: maintenance.WeeklyModelLimits{MaxInputBytes: 32768, MaxOutputBytes: 32768, MaxStderrBytes: 4096, MaxOutputTokens: 4096, TimeoutSeconds: 60, Attempts: 1, AgenticTurns: 1},
		},
		Policy: ownerctx.WeeklySelfOptions{OwnerID: "synthetic-owner", Facets: []string{"communication-style"}, Limits: ownerctx.DefaultWeeklySelfLimits(), WorkingLanguage: "en", TranslatorID: "identity", TranslatorVersion: "1"},
	}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestWeeklyCLIRegistrationExplicitClosedAndBounded(t *testing.T) {
	var absent product.Options
	if err := configureWeeklyRegistration(&absent, ""); err != nil {
		t.Fatal(err)
	}
	if absent.WeeklyCandidate != nil || absent.WeeklyNativeExecutor == nil {
		t.Fatal("ordinary startup must compose the fixed executor without selecting a tuple")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registration := weeklyCLIRegistration(t)
	body, err := json.Marshal(registration)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "registration.json")
	if err := os.WriteFile(file, body, 0600); err != nil {
		t.Fatal(err)
	}
	var selected product.Options
	if err := configureWeeklyRegistration(&selected, file); err != nil {
		t.Fatal(err)
	}
	if selected.WeeklyCandidate == nil || selected.WeeklyCandidate.Digest() != registration.Digest() || selected.WeeklyNativeExecutor == nil {
		t.Fatal("explicit declaration/fixed executor not composed")
	}

	for name, invalid := range map[string]string{
		"unknown":          strings.TrimSuffix(string(body), "}") + `,"credential":"forbidden"}`,
		"nested_unknown":   strings.Replace(string(body), `"runtime":"claude_code_cli"`, `"runtime":"claude_code_cli","other":true`, 1),
		"duplicate":        strings.Replace(string(body), `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1),
		"nested_duplicate": strings.Replace(string(body), `"runtime":"claude_code_cli"`, `"runtime":"claude_code_cli","runtime":"claude_code_cli"`, 1),
		"trailing":         string(body) + ` {}`,
		"invalid_tuple":    strings.Replace(string(body), `"credential_mode":"runtime_owned_session"`, `"credential_mode":"api_key"`, 1),
		"invalid_utf8":     strings.Replace(string(body), "synthetic-provider", string([]byte{0xff}), 1),
		"null":             `null`,
		"oversize":         strings.Repeat(" ", (256<<10)+1),
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(file, []byte(invalid), 0600); err != nil {
				t.Fatal(err)
			}
			var refused product.Options
			if err := configureWeeklyRegistration(&refused, file); err == nil {
				t.Fatal("invalid registration accepted")
			}
			if refused.WeeklyCandidate != nil || refused.WeeklyNativeExecutor != nil {
				t.Fatal("refused declaration changed native composition")
			}
		})
	}
	if err := os.WriteFile(file, body, 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias.json")
	if err := os.Symlink(file, alias); err == nil {
		var refused product.Options
		if err := configureWeeklyRegistration(&refused, alias); err == nil {
			t.Fatal("symlink alias accepted")
		}
	}
	hardlink := filepath.Join(root, "hardlink.json")
	if err := os.Link(file, hardlink); err != nil {
		t.Fatal(err)
	}
	var refused product.Options
	if err := configureWeeklyRegistration(&refused, hardlink); err == nil {
		t.Fatal("hardlink registration accepted")
	}
	if err := os.Remove(hardlink); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"registration.json", root, filepath.Join(root, "missing.json"), root + string(filepath.Separator) + "." + string(filepath.Separator) + "registration.json"} {
		if err := configureWeeklyRegistration(&refused, invalid); err == nil {
			t.Fatal("unsafe path accepted")
		}
	}
}

func TestWeeklyCLIRegistrationFlagIsStartupOnly(t *testing.T) {
	prior := os.Args
	defer func() { os.Args = prior }()
	for _, command := range []string{"serve", "maintenance-review", "maintenance-apply", "maintenance-status", "maintenance-run"} {
		t.Run(command, func(t *testing.T) {
			os.Args = []string{"maestro-mcp", command, "--weekly-registration-file", "relative.json"}
			if err := run(); err == nil || !strings.Contains(err.Error(), "weekly_registration_unavailable") {
				t.Fatalf("startup selection was not validated before binding: %v", err)
			}
		})
	}
	for _, command := range []string{"init", "inspect", "plugin", "mcpb", "hook", "owner-review"} {
		t.Run(command, func(t *testing.T) {
			os.Args = []string{"maestro-mcp", command, "--weekly-registration-file", "relative.json"}
			if err := run(); err == nil || !strings.Contains(err.Error(), "flag provided but not defined") {
				t.Fatalf("unrelated command admitted registration flag: %v", err)
			}
		})
	}
}
