package harness

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/capabilitybundle"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/boundary"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/decisionlog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/releasepack"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/skillmeta"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/topologygate"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/hookpolicy"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseprovider"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillpolicy"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillsindex"
)

// FindRoot walks upward until it finds go.mod.
func FindRoot(start string) (string, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("repository root not found from %s", start)
		}
		current = parent
	}
}

// Validate executes the development-only gate used locally and in CI.
func Validate(root string, full bool, out io.Writer) error {
	checks := []check{
		{"decision log", func() error {
			_, err := decisionlog.ParseFile(filepath.Join(root, "docs", "decisions", "decision-log.md"))
			return err
		}},
		{"development skills", func() error {
			return skillmeta.ValidateDir(filepath.Join(root, "dev", "skills"))
		}},
		{"capability bundles", func() error {
			catalog, err := capabilitybundle.LoadFile(filepath.Join(root, "bundles", "catalog", "catalog.json"))
			if err != nil {
				return err
			}
			for _, bundle := range catalog.Bundles {
				skillsRoot := filepath.Join(root, filepath.FromSlash(filepath.Dir(bundle.CatalogPointer)))
				if err := skillmeta.ValidateProductDir(skillsRoot); err != nil {
					return fmt.Errorf("validate product skills for bundle %s: %w", bundle.ID, err)
				}
				if err := skillsindex.Validate(skillsRoot); err != nil {
					return fmt.Errorf("validate skills index for bundle %s: %w", bundle.ID, err)
				}
			}
			return nil
		}},
		{"agent skill policy", func() error {
			skillsRoot := filepath.Join(root, "bundles", "base", "skills")
			policy, err := skillpolicy.ParseFile(filepath.Join(skillsRoot, "agent-skill-policy.json"))
			if err != nil {
				return err
			}
			skills, err := skillsindex.Build(skillsRoot)
			if err != nil {
				return err
			}
			agents, err := agentcatalog.ParseFile(filepath.Join(root, "bundles", "base", "agents", "catalog.json"))
			if err != nil {
				return err
			}
			_, err = skillpolicy.Compile(policy, skills, agents)
			return err
		}},
		{"managed agents", func() error {
			return agentcatalog.ValidateDir(filepath.Join(root, "bundles", "base", "agents"))
		}},
		{"canonical topology retired-role gate", func() error { return topologygate.Validate(root) }},
		{"Claude skill projections", func() error {
			return skillmeta.ValidateClaudeProjections(
				filepath.Join(root, "dev", "skills"),
				filepath.Join(root, ".claude", "skills"),
			)
		}},
		{"Claude primary skill routing", func() error {
			return skillmeta.ValidateClaudeRouting(root)
		}},
		{"GitHub Actions trust policy", func() error {
			return validateActionPins(root)
		}},
		{"development boundary", func() error { return boundary.Validate(root) }},
		{"memory contract", func() error {
			if err := memory.ValidateSchemaFile(filepath.Join(root, "schemas", "memory-policy.schema.json")); err != nil {
				return err
			}
			if err := memory.ValidateArtifactSchemaFile(filepath.Join(root, "schemas", "memory-artifact.schema.json")); err != nil {
				return err
			}
			if err := memory.ValidateCommitSchemaFile(filepath.Join(root, "schemas", "memory-commit.schema.json")); err != nil {
				return err
			}
			policy, err := memory.LoadFile(filepath.Join(root, "bundles", "base", "memory", "policy.json"))
			if err != nil {
				return err
			}
			return policy.Validate()
		}},
		{"product hook execution policy", func() error {
			file, err := os.Open(filepath.Join(root, "bundles", "base", "runtime", "hook-policy.json"))
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = hookpolicy.Parse(file)
			return err
		}},
		{"release contract", func() error {
			if err := releasecontract.ValidateSchemaFile(filepath.Join(root, "schemas", "release-manifest.schema.json")); err != nil {
				return err
			}
			if err := releaseverify.ValidateAuthorityRegistrySchemaFile(
				filepath.Join(root, "schemas", "release-authority-registry.schema.json"),
			); err != nil {
				return err
			}
			schema, err := os.Open(filepath.Join(root, "schemas", "release-provider.schema.json"))
			if err != nil {
				return err
			}
			if err := releaseprovider.ValidateProviderConfigSchema(schema); err != nil {
				schema.Close()
				return err
			}
			if err := schema.Close(); err != nil {
				return err
			}
			config, err := os.Open(filepath.Join(root, "bundles", "base", "release", "provider.json"))
			if err != nil {
				return err
			}
			_, parseErr := releaseprovider.ParseConfig(config)
			closeErr := config.Close()
			if parseErr != nil {
				return parseErr
			}
			return closeErr
		}},
		{"maintenance contract", func() error {
			return maintenance.ValidateSchemaAndCatalog(
				filepath.Join(root, "schemas", "maintenance-jobs.schema.json"),
				filepath.Join(root, "bundles", "base", "runtime", "maintenance.json"),
			)
		}},
		{"distribution allowlist", func() error {
			_, err := releasepack.LoadAllowlist(filepath.Join(root, "bundles", "base", "distribution.json"))
			return err
		}},
		{"managed OKF wiki", func() error {
			return atlas.VerifyManagedUpToDate(
				root,
				filepath.Join(root, "dev", "wiki", "managed-allowlist.json"),
				filepath.Join(root, "bundles", "base", "atlas", "managed"),
			)
		}},
		{"gofmt", func() error { return checkFormatting(root) }},
	}
	if full {
		testArgs := []string{"test", "./..."}
		// Some install-readiness and release-pack tests verify OS-specific
		// portable artifacts (macOS/Windows). They assert that the running
		// CLI platform matches the artifact target, so they fail under a
		// single-OS runner even when the code is correct. When
		// HARNESS_SKIP_CROSS_OS_TESTS is set (e.g. validate-lite on Ubuntu),
		// skip that subset via a -skip regex. The full 3-OS matrix workflow
		// still exercises them on their native runners.
		if os.Getenv("HARNESS_SKIP_CROSS_OS_TESTS") != "" {
			testArgs = append(testArgs,
				"-skip",
				`TestBuildMacOSPortableProducesVerifiedClaudeReadyArchive|TestVerifyAcceptsOnlyCanonicalConfiguredCodexInstall|TestVerifyAcceptsOnlyCanonicalConfiguredClaudeInstall|TestVerifyRejectsMissingManagedClaudeNativeAgent|TestVerifyRejectsMissingAndTamperedSurfaces`,
			)
		}
		checks = append(checks,
			check{"go vet (offline)", func() error { return runCommand(root, "go", "vet", "./...") }},
			check{"unit tests (offline)", func() error { return runCommand(root, "go", append([]string(nil), testArgs...)...) }},
		)
	} else {
		checks = append(checks, check{"fast unit tests (offline)", func() error { return runCommand(root, "go", "test", "./internal/dev/...") }})
	}
	for _, current := range checks {
		started := time.Now()
		fmt.Fprintf(out, "[run] %s\n", current.name)
		if err := current.run(); err != nil {
			elapsed := time.Since(started).Round(time.Millisecond)
			fmt.Fprintf(out, "[fail] %s (%s)\n", current.name, elapsed)
			return fmt.Errorf("%s (%s): %w", current.name, elapsed, err)
		}
		fmt.Fprintf(out, "[ok] %s (%s)\n", current.name, time.Since(started).Round(time.Millisecond))
	}
	return nil
}

type check struct {
	name string
	run  func() error
}

// maximumCommandLineLength keeps one invocation well inside the smallest limit
// any supported platform imposes. Windows caps a CreateProcess command line at
// 32,767 characters, and the repository's own file list passes that on a
// checkout with a long path: 408 files at absolute paths measured 79,089
// characters, so gofmt never ran and every check after it was unreachable.
const maximumCommandLineLength = 24000

// batchByCommandLineLength splits the file list so no single invocation can
// exceed the platform limit. A file whose own path is longer than the budget
// still gets its own batch rather than being silently dropped: skipping a file
// would make the check quietly stop covering it.
func batchByCommandLineLength(files []string) [][]string {
	var batches [][]string
	var current []string
	length := 0
	for _, file := range files {
		cost := len(file) + 1
		if len(current) > 0 && length+cost > maximumCommandLineLength {
			batches = append(batches, current)
			current, length = nil, 0
		}
		current = append(current, file)
		length += cost
	}
	if len(current) > 0 {
		batches = append(batches, current)
	}
	return batches
}

func checkFormatting(root string) error {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(files)
	if len(files) == 0 {
		return fmt.Errorf("no Go files found")
	}
	var unformatted []string
	for _, batch := range batchByCommandLineLength(files) {
		output, err := exec.Command("gofmt", append([]string{"-l"}, batch...)...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("run gofmt: %w: %s", err, strings.TrimSpace(string(output)))
		}
		if listed := strings.TrimSpace(string(output)); listed != "" {
			unformatted = append(unformatted, listed)
		}
	}
	if len(unformatted) > 0 {
		return fmt.Errorf("unformatted files:\n%s", strings.Join(unformatted, "\n"))
	}
	return nil
}

func runCommand(root, name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Dir = root
	if name == "go" {
		// Local validation must never spend credits or depend on network access.
		// The module and checksum caches remain the only allowed dependency source.
		command.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off")
	}
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, strings.TrimSpace(output.String()))
	}
	return nil
}
