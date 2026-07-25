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

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/boundary"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/decisionlog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/releasepack"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/skillmeta"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/hookpolicy"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
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
		{"product skills", func() error {
			return skillmeta.ValidateProductDir(filepath.Join(root, "bundles", "base", "skills"))
		}},
		{"skills index", func() error {
			return skillsindex.Validate(filepath.Join(root, "bundles", "base", "skills"))
		}},
		{"managed agents", func() error {
			return agentcatalog.ValidateDir(filepath.Join(root, "bundles", "base", "agents"))
		}},
		{"Claude skill projections", func() error {
			return skillmeta.ValidateClaudeProjections(
				filepath.Join(root, "dev", "skills"),
				filepath.Join(root, ".claude", "skills"),
			)
		}},
		{"Claude primary skill routing", func() error {
			return skillmeta.ValidateClaudeRouting(root)
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
			return releaseverify.ValidateAuthorityRegistrySchemaFile(
				filepath.Join(root, "schemas", "release-authority-registry.schema.json"),
			)
		}},
		{"distribution allowlist", func() error {
			_, err := releasepack.LoadAllowlist(filepath.Join(root, "bundles", "base", "distribution.json"))
			return err
		}},
		{"gofmt", func() error { return checkFormatting(root) }},
	}
	if full {
		checks = append(checks,
			check{"go vet", func() error { return runCommand(root, "go", "vet", "./...") }},
			check{"unit tests", func() error { return runCommand(root, "go", "test", "./...") }},
		)
	} else {
		checks = append(checks, check{"fast unit tests", func() error { return runCommand(root, "go", "test", "./internal/dev/...") }})
	}
	for _, current := range checks {
		if err := current.run(); err != nil {
			fmt.Fprintf(out, "[fail] %s\n", current.name)
			return fmt.Errorf("%s: %w", current.name, err)
		}
		fmt.Fprintf(out, "[ok] %s\n", current.name)
	}
	return nil
}

type check struct {
	name string
	run  func() error
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
	command := exec.Command("gofmt", append([]string{"-l"}, files...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("run gofmt: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if unformatted := strings.TrimSpace(string(output)); unformatted != "" {
		return fmt.Errorf("unformatted files:\n%s", unformatted)
	}
	return nil
}

func runCommand(root, name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Dir = root
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, strings.TrimSpace(output.String()))
	}
	return nil
}
