package atlas

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReconcileManagedProducesDeterministicOKFBundle(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "specs/one.md", "# One\n\nThe first source.\n\n[Two](two.md)\n")
	writeTestFile(t, root, "specs/two.md", "# Two\n\nThe second source.\n")
	allowlist := filepath.Join(root, "allowlist.json")
	writeTestFile(t, root, "allowlist.json", `{
  "schema_version": 1,
  "okf_version": "0.2",
  "generator_version": "test",
  "policy_version": "1",
  "log_date": "2026-07-28",
  "sources": [
    {"id":"one","path":"specs/one.md","type":"Reference","title":"One","description":"The first source.","tags":["one"],"related":["two"]},
    {"id":"two","path":"specs/two.md","type":"Reference","title":"Two","description":"The second source.","tags":["two"],"related":["one"]}
  ]
}`)

	output := filepath.Join(root, "bundle")
	first, err := ReconcileManaged(root, allowlist, output)
	if err != nil {
		t.Fatal(err)
	}
	firstSnapshot := snapshotDirectory(t, output)
	second, err := ReconcileManaged(root, allowlist, output)
	if err != nil {
		t.Fatal(err)
	}
	secondSnapshot := snapshotDirectory(t, output)
	if first.Fingerprint == "" || first.Fingerprint != second.Fingerprint {
		t.Fatalf("fingerprints = %q and %q", first.Fingerprint, second.Fingerprint)
	}
	if firstSnapshot != secondSnapshot {
		t.Fatal("reconciliation is not deterministic")
	}
	if err := ValidateManagedBundle(output); err != nil {
		t.Fatal(err)
	}
	one, err := os.ReadFile(filepath.Join(output, "concepts", "one.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"type: Reference",
		"x-bcgos-scope: managed",
		"sources:",
		"/concepts/two.md",
		"[Two](/concepts/two.md)",
		"# Source snapshot",
	} {
		if !strings.Contains(string(one), expected) {
			t.Fatalf("one.md missing %q:\n%s", expected, one)
		}
	}
}

func TestReconcileManagedRewritesOpaqueAndRejectsBrokenMarkdownLinks(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "specs/one.md", "# One\n\n[Release gates](../docs/release-gates-checklist.md)\n")
	writeTestFile(t, root, "docs/release-gates-checklist.md", "# Gates\n")
	allowlist := filepath.Join(root, "allowlist.json")
	writeTestFile(t, root, "allowlist.json", `{"schema_version":1,"okf_version":"0.2","generator_version":"test/1","policy_version":"managed-product/1","log_date":"2026-07-28","sources":[{"id":"one","path":"specs/one.md","type":"Reference","title":"One"}]}`)
	output := filepath.Join(root, "bundle")
	if _, err := ReconcileManaged(root, allowlist, output); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(output, "concepts", "one.md"))
	if err != nil || !strings.Contains(string(body), "repo://docs/release-gates-checklist.md") {
		t.Fatalf("opaque link body=%q err=%v", body, err)
	}
	diagnostics, err := os.ReadFile(filepath.Join(output, "diagnostics.json"))
	if err != nil || !strings.Contains(string(diagnostics), "opaque_links") {
		t.Fatalf("link diagnostics=%q err=%v", diagnostics, err)
	}

	writeTestFile(t, root, "specs/one.md", "# One\n\n[Missing](missing.md)\n")
	if _, err := ReconcileManaged(root, allowlist, filepath.Join(root, "broken")); err == nil || !strings.Contains(err.Error(), "broken markdown link") {
		t.Fatalf("broken link error=%v", err)
	}
}

func TestReconcileManagedRejectsSourceOutsideManagedRoots(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "dev/private.md", "# private\n")
	allowlist := filepath.Join(root, "allowlist.json")
	writeTestFile(t, root, "allowlist.json", `{
  "schema_version": 1,
  "okf_version": "0.2",
  "generator_version": "test",
  "policy_version": "1",
  "log_date": "2026-07-28",
  "sources": [{"id":"private","path":"dev/private.md","type":"Reference","title":"Private"}]
}`)
	if _, err := ReconcileManaged(root, allowlist, filepath.Join(root, "bundle")); err == nil {
		t.Fatal("managed reconciliation accepted a development source")
	}
}

func TestReconcileManagedRejectsInvalidCalendarDate(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "README.md", "# Product\n")
	allowlist := `{
  "schema_version": 1,
  "okf_version": "0.2",
  "generator_version": "test/1",
  "policy_version": "managed-product/1",
  "log_date": "2026-02-31",
  "sources": [{"id":"readme","path":"README.md","type":"Product","title":"Product"}]
}`
	allowlistPath := filepath.Join(root, "allowlist.json")
	if err := os.WriteFile(allowlistPath, []byte(allowlist), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReconcileManaged(root, allowlistPath, filepath.Join(root, "atlas")); err == nil {
		t.Fatal("expected invalid calendar date to be rejected")
	}
}

func TestReconcileManagedRejectsSourceIDPathTraversal(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "README.md", "# Product\n")
	allowlist := `{
  "schema_version": 1,
  "okf_version": "0.2",
  "generator_version": "test/1",
  "policy_version": "managed-product/1",
  "log_date": "2026-07-28",
  "sources": [{"id":"../escape","path":"README.md","type":"Product","title":"Product"}]
}`
	allowlistPath := filepath.Join(root, "allowlist.json")
	if err := os.WriteFile(allowlistPath, []byte(allowlist), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReconcileManaged(root, allowlistPath, filepath.Join(root, "atlas")); err == nil {
		t.Fatal("expected source ID path traversal to be rejected")
	}
}

func TestReconcileManagedRejectsDuplicateAllowlistKeys(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "README.md", "# Product\n")
	allowlistPath := filepath.Join(root, "allowlist.json")
	writeTestFile(t, root, "allowlist.json", `{"schema_version":1,"schema_version":1,"okf_version":"0.2","generator_version":"test/1","policy_version":"managed-product/1","log_date":"2026-07-28","sources":[{"id":"readme","path":"README.md","type":"Product","title":"Product"}]}`)
	if _, err := ReconcileManaged(root, allowlistPath, filepath.Join(root, "atlas")); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate allowlist key rejection, got %v", err)
	}
}

func TestReconcileManagedRejectsMalformedAllowlist(t *testing.T) {
	root := t.TempDir()
	allowlistPath := filepath.Join(root, "allowlist.json")
	if err := os.WriteFile(allowlistPath, []byte(`{"schema_version":1,`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReconcileManaged(root, allowlistPath, filepath.Join(root, "atlas")); err == nil {
		t.Fatal("expected malformed allowlist rejection")
	}
}

func TestReconcileManagedRejectsSymlinkedSource(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "README-source.md")
	if err := os.WriteFile(target, []byte("# Product\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "README.md")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	allowlistPath := filepath.Join(root, "allowlist.json")
	writeTestFile(t, root, "allowlist.json", `{"schema_version":1,"okf_version":"0.2","generator_version":"test/1","policy_version":"managed-product/1","log_date":"2026-07-28","sources":[{"id":"readme","path":"README.md","type":"Product","title":"Product"}]}`)
	if _, err := ReconcileManaged(root, allowlistPath, filepath.Join(root, "atlas")); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlinked source rejection, got %v", err)
	}
}

func TestValidateManagedBundleRejectsMissingConceptType(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "index.md", "okf_version: \"0.2\"\n\n# Bundle\n")
	writeTestFile(t, root, "log.md", "# Directory Update Log\n")
	writeTestFile(t, root, "broken.md", "# Missing frontmatter\n")
	if err := ValidateManagedBundle(root); err == nil || !strings.Contains(err.Error(), "frontmatter") {
		t.Fatalf("ValidateManagedBundle() error = %v", err)
	}
}

func TestValidateManagedBundleRejectsBrokenMarkdownLink(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "index.md", "okf_version: \"0.2\"\n\n# Bundle\n")
	writeTestFile(t, root, "log.md", "# Directory Update Log\n")
	writeTestFile(t, root, "concepts/one.md", "---\ntype: Reference\nx-bcgos-profile-version: \"1\"\nx-bcgos-scope: managed\nx-bcgos-policy-version: \"1\"\nx-bcgos-generator-version: \"test\"\n---\n\n# One\n\n[Missing](missing.md)\n")
	if err := ValidateManagedBundle(root); err == nil || !strings.Contains(err.Error(), "broken generated markdown link") {
		t.Fatalf("expected broken link failure, got %v", err)
	}
}

func TestVerifyManagedUpToDateDetectsStaleGeneratedOutput(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "specs/one.md", "# One\n")
	allowlist := filepath.Join(root, "allowlist.json")
	writeTestFile(t, root, "allowlist.json", `{
  "schema_version": 1,
  "okf_version": "0.2",
  "generator_version": "test",
  "policy_version": "1",
  "log_date": "2026-07-28",
  "sources": [{"id":"one","path":"specs/one.md","type":"Reference","title":"One"}]
}`)
	output := filepath.Join(root, "bundle")
	if _, err := ReconcileManaged(root, allowlist, output); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "index.md"), []byte("okf_version: \"0.2\"\n\n# Stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyManagedUpToDate(root, allowlist, output); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("VerifyManagedUpToDate() error = %v", err)
	}
}

func TestVerifyManagedUpToDateDetectsChangedSource(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "specs/one.md", "# One\n")
	allowlist := filepath.Join(root, "allowlist.json")
	writeTestFile(t, root, "allowlist.json", `{"schema_version":1,"okf_version":"0.2","generator_version":"test","policy_version":"1","log_date":"2026-07-28","sources":[{"id":"one","path":"specs/one.md","type":"Reference","title":"One"}]}`)
	output := filepath.Join(root, "bundle")
	if _, err := ReconcileManaged(root, allowlist, output); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, "specs/one.md", "# One changed\n")
	if err := VerifyManagedUpToDate(root, allowlist, output); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("expected source freshness failure, got %v", err)
	}
}

func writeTestFile(t *testing.T, root, relative, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func snapshotDirectory(t *testing.T, root string) string {
	t.Helper()
	var builder strings.Builder
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		builder.WriteString(filepath.ToSlash(relative))
		builder.WriteByte('\n')
		builder.Write(body)
		builder.WriteString("\n---\n")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return builder.String()
}
