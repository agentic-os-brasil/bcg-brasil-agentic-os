package packaging

import (
	"context"
	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func foregroundConfigurationFixture(t *testing.T) (product.ForegroundRuntime, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	profile, work, managed := filepath.Join(root, "profile"), filepath.Join(root, "work"), filepath.Join(root, "managed")
	for _, p := range []string{profile, work, managed} {
		if e := os.Mkdir(p, 0700); e != nil {
			t.Fatal(e)
		}
	}
	if err = os.WriteFile(filepath.Join(profile, "settings.json"), []byte(`{"autoMemoryEnabled":false,"syncClaudeAiPlugins":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := observeWeeklyConfigurationAt(context.Background(), profile, work, managed)
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		if err == nil {
			t.Fatal("unsupported identity backend admitted")
		}
		return product.ForegroundRuntime{}, managed
	}
	if err != nil {
		t.Fatal(err)
	}
	return product.ForegroundRuntime{ProfileDirectory: profile, WorkingDirectory: work, Configuration: c, Environment: []string{"HOME=" + root, "CLAUDE_CONFIG_DIR=" + profile, "PATH=/usr/bin:/bin"}}, managed
}
func TestForegroundConfigurationReobservesManagedDropins(t *testing.T) {
	for _, initial := range []string{"absent", "empty"} {
		t.Run(initial, func(t *testing.T) {
			r, managed := foregroundConfigurationFixture(t)
			if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
				return
			}
			dropins := filepath.Join(managed, "managed-settings.d")
			if initial == "empty" {
				if err := os.Mkdir(dropins, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := verifyForegroundConfigurationAt(context.Background(), r, managed); err != nil {
				t.Fatal("stable selection unavailable", err)
			}
			if initial == "absent" {
				if err := os.Mkdir(dropins, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(dropins, "late-policy.json"), []byte(`{"hooks":{"SessionStart":[{"command":"untrusted"}]}}`), 0600); err != nil {
				t.Fatal(err)
			}
			if err := verifyForegroundConfigurationAt(context.Background(), r, managed); err == nil {
				t.Fatal("post-review managed drop-in escaped the foreground check")
			}
		})
	}
}

func TestForegroundLocalClosureUsesSelectedHomeAndTracksNewSources(t *testing.T) {
	r, managed := foregroundConfigurationFixture(t)
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		return
	}
	home := filepath.Dir(r.ProfileDirectory)
	r.Environment = []string{"HOME=" + home, "CLAUDE_CONFIG_DIR=" + r.ProfileDirectory, "PATH=/usr/bin:/bin"}
	// This reader uses the same original finite closure without invoking a CLI.
	initial, err := observeForegroundLocalClosureAt(context.Background(), r, managed)
	if err != nil {
		t.Fatal(err)
	}
	selectedImport := filepath.Join(home, "selected-import.md")
	if err = os.WriteFile(selectedImport, []byte("# Selected synthetic HOME\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(r.ProfileDirectory, "CLAUDE.md"), []byte("@~/selected-import.md\n"), 0600); err != nil {
		t.Fatal(err)
	}
	changed, err := observeForegroundLocalClosureAt(context.Background(), r, managed)
	if err != nil || changed == initial {
		t.Fatal("selected HOME/import membership was ignored", err)
	}
	if err = os.WriteFile(selectedImport, []byte("# Changed selected import\n"), 0600); err != nil {
		t.Fatal(err)
	}
	again, err := observeForegroundLocalClosureAt(context.Background(), r, managed)
	if err != nil || again == changed {
		t.Fatal("transitive instruction change was ignored", err)
	}
}
func TestForegroundDropinDriftStopsBeforePrivateInputClaim(t *testing.T) {
	r, managed := foregroundConfigurationFixture(t)
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		return
	}
	dropins := filepath.Join(managed, "managed-settings.d")
	if err := os.Mkdir(dropins, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dropins, "late-policy.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	q := &foregroundQualification{tupleSHA256: "controlled-tuple", inputSHA256: digest([]byte("PRIVATE")), sessionID: "controlled-probe"}
	b := &foregroundProcessBinding{
		identity: &WeeklyRuntimeIdentity{}, qualification: q, tupleSHA256: "controlled-tuple",
		current: func(ctx context.Context) error { return verifyForegroundConfigurationAt(ctx, r, managed) },
	}
	if err := b.beforeInput(context.Background(), []byte("PRIVATE")); err == nil {
		t.Fatal("post-review policy addition admitted stdin")
	}
	if q.inputClaimed {
		t.Fatal("changed sources consumed or released private input")
	}
}

func TestForegroundConfigurationIgnoresCoordinatorGitEnvironment(t *testing.T) {
	r, managed := foregroundConfigurationFixture(t)
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		return
	}
	r.Environment = []string{"HOME=" + filepath.Dir(r.ProfileDirectory), "CLAUDE_CONFIG_DIR=" + r.ProfileDirectory, "PATH=/usr/bin:/bin"}
	baseline, err := observeForegroundLocalClosureAt(context.Background(), r, managed)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"GIT_DIR", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, "coordinator-only-value")
			initial, err := observeForegroundConfigurationAt(context.Background(), r, managed)
			if err != nil || product.ForegroundDigest(initial) != product.ForegroundDigest(r.Configuration) {
				t.Fatal("initial foreground observation used coordinator environment", err)
			}
			if err := verifyForegroundConfigurationAt(context.Background(), r, managed); err != nil {
				t.Fatal("coordinator environment contaminated closed child observation", err)
			}
			current, err := observeForegroundLocalClosureAt(context.Background(), r, managed)
			if err != nil || current != baseline {
				t.Fatal("coordinator environment changed closed child closure", err)
			}
			if _, err := observeWeeklyConfigurationAt(context.Background(), r.ProfileDirectory, r.WorkingDirectory, managed); err == nil {
				t.Fatal("legacy observer stopped honoring its coordinator environment")
			}
		})
	}
}
