package memorybridge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	basememory "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
)

func TestInspectUsesOnlyFixedLegacyRootsAndReturnsOpaqueMetadata(t *testing.T) {
	service := testService(t)
	writeLegacy(t, service.DataRoot, "recent", "private-name.md", "recent sentinel")
	writeLegacy(t, service.DataRoot, "lifetime", "durable-name.md", "durable sentinel")
	writeLegacy(t, service.DataRoot, "recent", "ignored.txt", "ignored")
	if err := os.MkdirAll(filepath.Join(service.DataRoot, "memory", "recent", "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(service.DataRoot, "memory", "recent", "nested", "ignored.md"), "nested")
	writeFile(t, filepath.Join(service.DataRoot, "outside.md"), "outside")

	report, err := service.Inspect(context.Background(), "case-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Candidates) != 2 {
		t.Fatalf("candidates = %#v", report.Candidates)
	}
	body, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private-name", "durable-name", service.DataRoot, "recent sentinel", "durable sentinel"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("inspect leaked %q: %s", forbidden, body)
		}
	}
	for _, candidate := range report.Candidates {
		if len(candidate.ID) != 32 || len(candidate.SHA256) != 64 || candidate.Bytes == 0 {
			t.Fatalf("invalid metadata: %#v", candidate)
		}
	}
}

func TestPreviewIsBoundedAndRevalidatesContentBoundCandidate(t *testing.T) {
	service := testService(t)
	path := writeLegacy(t, service.DataRoot, "recent", "memory.md", strings.Repeat("a", PreviewBytes+100))
	report, err := service.Inspect(context.Background(), "case-a")
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.Preview(context.Background(), "case-a", report.Candidates[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len([]byte(preview.Content)) != PreviewBytes || !preview.Truncated {
		t.Fatalf("preview = %#v", preview)
	}
	writeFile(t, path, "changed")
	if _, err := service.Preview(context.Background(), "case-a", report.Candidates[0].ID); err == nil {
		t.Fatal("changed candidate remained valid")
	}
}

func TestDiscoveryFailsClosedForSymlinksInvalidUTF8AndBounds(t *testing.T) {
	for _, test := range []struct {
		name    string
		arrange func(*testing.T, *Service)
	}{
		{name: "symlink", arrange: func(t *testing.T, service *Service) {
			target := writeLegacy(t, service.DataRoot, "weekly", "target.md", "safe")
			if err := os.Symlink(target, filepath.Join(service.DataRoot, "memory", "weekly", "link.md")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "invalid utf8", arrange: func(t *testing.T, service *Service) {
			path := filepath.Join(service.DataRoot, "memory", "recent", "bad.md")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte{0xff}, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "file too large", arrange: func(t *testing.T, service *Service) {
			writeLegacy(t, service.DataRoot, "lifetime", "large.md", strings.Repeat("x", MaxCandidateBytes+1))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := testService(t)
			test.arrange(t, service)
			if _, err := service.Inspect(context.Background(), "case-a"); err == nil {
				t.Fatal("expected bounded discovery failure")
			} else if strings.Contains(err.Error(), service.DataRoot) {
				t.Fatalf("error leaked path: %v", err)
			}
		})
	}
}

func TestDiscoveryAndApplyEnforceAggregateLimits(t *testing.T) {
	for _, test := range []struct {
		name  string
		count int
		size  int
	}{
		{name: "directory entries", count: MaxDirectoryEntries + 1, size: 1},
		{name: "candidate count", count: MaxCandidates + 1, size: 1},
		{name: "discovered bytes", count: 17, size: MaxCandidateBytes},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := testService(t)
			for index := 0; index < test.count; index++ {
				writeLegacy(t, service.DataRoot, "recent", fmt.Sprintf("%03d.md", index), strings.Repeat("x", test.size))
			}
			if _, err := service.Inspect(context.Background(), "case-a"); err == nil {
				t.Fatal("expected aggregate limit failure")
			}
		})
	}

	service := testService(t)
	for index := 0; index < 3; index++ {
		writeLegacy(t, service.DataRoot, "recent", fmt.Sprintf("selected-%d.md", index), strings.Repeat("x", MaxCandidateBytes))
	}
	report, err := service.Inspect(context.Background(), "case-a")
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{report.Candidates[0].ID, report.Candidates[1].ID, report.Candidates[2].ID}
	if _, err := service.Apply(context.Background(), ApplyRequest{WorkspaceID: "case-a", CandidateIDs: ids, Confirmed: true, AttestedTargetScope: true}); err == nil {
		t.Fatal("selected content exceeded apply limit")
	}
}

func TestApplyRequiresAttestationPreservesLegacyBytesAndIsolatesTarget(t *testing.T) {
	service := testService(t)
	path := writeLegacy(t, service.DataRoot, "medium-term", "selected.md", "selected memory")
	writeLegacy(t, service.DataRoot, "recent", "unselected.md", "unselected memory")
	before, _ := os.ReadFile(path)
	report, err := service.Inspect(context.Background(), "case-a")
	if err != nil {
		t.Fatal(err)
	}
	selected := candidateForLayer(t, report, "medium-term")
	for _, request := range []ApplyRequest{
		{WorkspaceID: "case-a", CandidateIDs: []string{selected.ID}, Confirmed: true},
		{WorkspaceID: "case-a", CandidateIDs: []string{selected.ID}, AttestedTargetScope: true},
	} {
		if _, err := service.Apply(context.Background(), request); err == nil {
			t.Fatal("apply accepted missing owner gate")
		}
	}
	result, err := service.Apply(context.Background(), ApplyRequest{WorkspaceID: "case-a", CandidateIDs: []string{selected.ID}, Confirmed: true, AttestedTargetScope: true})
	if err != nil || result.State != "applied" {
		t.Fatalf("apply = %#v, %v", result, err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("legacy source changed")
	}
	bundle, err := service.Engine.AssembleContext("case-a")
	if err != nil || !contextContains(bundle, "selected memory") || contextContains(bundle, "unselected memory") {
		t.Fatalf("target context = %#v, %v", bundle, err)
	}
	other, err := service.Engine.AssembleContext("case-b")
	if err != nil {
		t.Fatal(err)
	}
	if contextContains(other, "selected memory") {
		t.Fatalf("import crossed workspace: %#v", other)
	}
}

func testService(t *testing.T) *Service {
	t.Helper()
	policy, err := basememory.Policy()
	if err != nil {
		t.Fatal(err)
	}
	runtimeConfig, err := basememory.Runtime()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	engine := &memory.Engine{Root: root, Policy: policy, Budgets: runtimeConfig.ContextBudgets(), Now: func() time.Time { return time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC) }}
	return &Service{DataRoot: root, Engine: engine}
}

func writeLegacy(t *testing.T, root, layer, name, content string) string {
	t.Helper()
	path := filepath.Join(root, "memory", layer, name)
	writeFile(t, path, content)
	return path
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func candidateForLayer(t *testing.T, report InspectReport, layer string) Candidate {
	t.Helper()
	for _, candidate := range report.Candidates {
		if candidate.LegacyLayer == layer {
			return candidate
		}
	}
	t.Fatalf("missing %s", layer)
	return Candidate{}
}

func contextContains(bundle memory.ContextBundle, value string) bool {
	for _, section := range bundle.Sections {
		if strings.Contains(section.Content, value) {
			return true
		}
	}
	return false
}
