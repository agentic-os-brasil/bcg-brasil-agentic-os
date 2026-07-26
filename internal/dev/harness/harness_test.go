package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindRootWalksUpward(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := FindRoot(nested)
	if err != nil {
		t.Fatalf("FindRoot() error = %v", err)
	}
	if got != root {
		t.Fatalf("FindRoot() = %q, want %q", got, root)
	}
}

func TestValidateActionPinsAcceptsImmutableAndLocalReferences(t *testing.T) {
	root := t.TempDir()
	writeActionFixture(t, root, ".github/workflows/validate.yml", `
jobs:
  validate:
    steps:
      - uses: actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803
      - uses: ./.github/actions/check-release
      - uses: docker://alpine@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
`)
	writeActionFixture(t, root, ".github/actions/check-release/action.yaml", `
runs:
  using: composite
  steps:
    - uses: acme/security-action/subtask@0123456789abcdef0123456789abcdef01234567
`)

	if err := validateActionPins(root); err != nil {
		t.Fatalf("validateActionPins() error = %v", err)
	}
}

func TestValidateActionPinsRejectsUnsafeReferencesAndMalformedYAML(t *testing.T) {
	const sha = "0123456789abcdef0123456789abcdef01234567"
	tests := []struct {
		name        string
		path        string
		body        string
		wantMessage string
	}{
		{
			name:        "tag reference",
			path:        ".github/workflows/tag.yml",
			body:        "jobs:\n  test:\n    steps:\n      - uses: actions/checkout@v6\n",
			wantMessage: "full 40-character commit SHA",
		},
		{
			name:        "block scalar reference",
			path:        ".github/workflows/block.yaml",
			body:        "jobs:\n  test:\n    steps:\n      - uses: |\n          actions/checkout@" + sha + "\n",
			wantMessage: "plain or quoted scalar",
		},
		{
			name:        "inline mapping tag reference",
			path:        ".github/workflows/inline.yml",
			body:        "jobs: {test: {steps: [{uses: actions/checkout@v6}]}}\n",
			wantMessage: "full 40-character commit SHA",
		},
		{
			name:        "composite action tag reference",
			path:        ".github/actions/example/action.yml",
			body:        "runs:\n  using: composite\n  steps:\n    - uses: actions/setup-go@v6\n",
			wantMessage: "full 40-character commit SHA",
		},
		{
			name:        "malformed workflow YAML",
			path:        ".github/workflows/malformed.yml",
			body:        "jobs:\n  test: [\n",
			wantMessage: "parse YAML",
		},
		{
			name:        "mutable docker tag",
			path:        ".github/workflows/docker.yml",
			body:        "jobs:\n  test:\n    steps:\n      - uses: docker://alpine:3.20\n",
			wantMessage: "sha256 digest",
		},
		{
			name:        "escaping local action",
			path:        ".github/workflows/local.yml",
			body:        "jobs:\n  test:\n    steps:\n      - uses: ./../outside\n",
			wantMessage: "safe repository-relative path",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeActionFixture(t, root, test.path, test.body)

			err := validateActionPins(root)
			if err == nil {
				t.Fatal("validateActionPins() error = nil, want rejection")
			}
			if !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf("validateActionPins() error = %q, want substring %q", err, test.wantMessage)
			}
		})
	}
}

func TestValidateActionPinsIgnoresOtherYAMLFiles(t *testing.T) {
	root := t.TempDir()
	writeActionFixture(t, root, "docs/example.yml", "uses: actions/checkout@v6\n")
	writeActionFixture(t, root, ".github/actions/example/metadata.yml", "uses: actions/checkout@v6\n")

	if err := validateActionPins(root); err != nil {
		t.Fatalf("validateActionPins() error = %v", err)
	}
}

func writeActionFixture(t *testing.T, root, relativePath, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimPrefix(body, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
}
