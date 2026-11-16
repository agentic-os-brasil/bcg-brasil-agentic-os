package docling_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
)

func TestDocumentProfilesPreserveLegacyInputsAndMetadata(t *testing.T) {
	for name, want := range map[string]string{
		"runtime/inputs.json":               "a4813105cbf27ca7689c15b9851375136d0ba07aa1e6776ddabdf31c243ff072",
		"runtime/pylock.toml":               "f69da0e247c73dc20870d51030344e1cbba9f5520b722c79a336b9d8b044639b",
		"runtime/bridge.py":                 "daddc15d07f4dcc2e232588269f6c3f27d0cea24ec981852b27d919f492e8855",
		"runtime/windows-amd64/inputs.json": "ead7b0bcda2f98ffd738aa40a51f7d8ac09afe4e0e8926aa1acac245ca0f7773",
		"runtime/windows-amd64/pylock.toml": "728aab0b59dd5011070dfc4e9159e3ed6180b1668539f84275f1b88e81f5be58",
		"runtime/windows-amd64/bridge.py":   "1627182352f08b2c327eacbf4f6c0e9ce93fee2398381ee405bde8a455d50975",
		"runtime/parser-licenses.json":      "19c15a90c1fab5a4f7a183fae802007b78eae8e77184223dd1d4b1eecfe1ecf1",
	} {
		body, err := os.ReadFile(name)
		if err != nil || fmt.Sprintf("%x", sha256.Sum256(body)) != want {
			t.Fatalf("preexisting profile bytes changed: %s: %v", name, err)
		}
	}
	for _, target := range []struct {
		os, arch          string
		legacy, documents int
		python            int64
	}{
		{"darwin", "arm64", 51, 55, 25172086},
		{"windows", "amd64", 52, 56, 47113364},
	} {
		legacy, err := docling.DescribeProfile(target.os, target.arch, "policy-formats-v1")
		if err != nil || len(legacy.Inputs) != target.legacy || legacy.Python.Size != target.python {
			t.Fatalf("legacy descriptor: %+v %v", legacy, err)
		}
		documents, err := docling.DescribeProfile(target.os, target.arch, "model-free-documents-v1")
		if err != nil || len(documents.Inputs) != target.documents || documents.LockSHA256 == legacy.LockSHA256 || documents.BridgeSHA256 == legacy.BridgeSHA256 {
			t.Fatalf("new closed profile: %+v %v", documents, err)
		}
		byName := map[string]docling.Input{}
		for _, input := range documents.Inputs {
			byName[input.Name] = input
		}
		for _, input := range legacy.Inputs {
			if byName[input.Name] != input {
				t.Fatalf("legacy dependency changed: %s", input.Name)
			}
		}
		for _, name := range []string{"python-pptx", "xlsxwriter", "pylatexenc", "pypdfium2"} {
			if byName[name].SHA256 == "" {
				t.Fatalf("missing exact parser input: %s", name)
			}
		}
		documents.Inputs[0].SHA256 = "mutated-copy"
		again, err := docling.DescribeProfile(target.os, target.arch, "model-free-documents-v1")
		if err != nil || again.Inputs[0].SHA256 == "mutated-copy" {
			t.Fatal("caller mutated shared descriptor")
		}
	}
}

func TestDocumentProfileSelectionIsClosedBeforeEffects(t *testing.T) {
	for _, selected := range []struct{ os, arch, profile string }{
		{"darwin", "amd64", "model-free-documents-v1"},
		{"windows", "arm64", "model-free-documents-v1"},
		{"darwin", "arm64", "unknown"},
		{"darwin", "arm64", ""},
	} {
		if _, err := docling.DescribeProfile(selected.os, selected.arch, selected.profile); err == nil {
			t.Fatal("unadmitted profile", selected)
		}
		output := filepath.Join(t.TempDir(), "new-pack")
		if _, err := docling.AssembleProfile("/missing-inputs", output, selected.os, selected.arch, selected.profile); err == nil {
			t.Fatal("unadmitted assembly", selected)
		}
		if _, err := os.Lstat(output); !os.IsNotExist(err) {
			t.Fatal("unadmitted profile created output", err)
		}
	}
}
