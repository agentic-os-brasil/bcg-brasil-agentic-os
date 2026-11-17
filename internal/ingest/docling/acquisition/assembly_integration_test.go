package acquisition

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

// This opt-in construction check consumes only already authenticated public
// inputs. It builds Windows bytes on the host and never executes those bytes.
func TestCanonicalWindowsDocumentsArchiveFromRetainedInputs(t *testing.T) {
	inputs := os.Getenv("MAESTRO_WINDOWS_ACQUISITION_INPUTS")
	output := os.Getenv("MAESTRO_WINDOWS_ACQUISITION_OUTPUT")
	if inputs == "" || output == "" {
		t.Skip("explicit retained public inputs and new output required; native Windows acquisition remains unattested")
	}
	d, err := docling.DescribeProfile("windows", "amd64", docling.DocumentsProfile)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(inputs)
	if err != nil || len(entries) != len(d.Inputs)+1 {
		t.Fatal("closed retained input count mismatch", err)
	}
	total := int64(0)
	for _, input := range append([]docling.Input{d.Python}, d.Inputs...) {
		body, err := readBounded(filepath.Join(inputs, input.Filename), input.Size)
		if err != nil || int64(len(body)) != input.Size || digest(body) != input.SHA256 {
			t.Fatal("retained input mismatch", input.Filename, err)
		}
		total += input.Size
	}
	if err = os.Mkdir(output, 0700); err != nil {
		t.Fatal("new construction output required", err)
	}
	first, err := docling.AssembleProfile(inputs, filepath.Join(output, "first"), d.OS, d.Arch, d.Profile)
	if err != nil {
		t.Fatal(err)
	}
	second, err := docling.AssembleProfile(inputs, filepath.Join(output, "second"), d.OS, d.Arch, d.Profile)
	if err != nil {
		t.Fatal(err)
	}
	one, err := readBounded(first.ArchivePath, d.MaximumArchiveBytes)
	if err != nil {
		t.Fatal(err)
	}
	two, err := readBounded(second.ArchivePath, d.MaximumArchiveBytes)
	if err != nil {
		t.Fatal(err)
	}
	if digest(one) != first.ArchiveSHA256 || digest(two) != second.ArchiveSHA256 || first.ArchiveSHA256 != second.ArchiveSHA256 || len(one) != len(two) || first.Files != second.Files || first.ExpandedBytes != second.ExpandedBytes {
		t.Fatal("canonical archive nondeterministic")
	}
	descriptor, _ := releasecontract.CanonicalJSON(d)
	receipt := map[string]any{"profile": d.Profile, "os": d.OS, "arch": d.Arch, "input_count": len(d.Inputs) + 1, "total_input_bytes": total, "descriptor_sha256": digest(descriptor), "archive_sha256": first.ArchiveSHA256, "archive_bytes": len(one), "files": first.Files, "expanded_bytes": first.ExpandedBytes, "maximum_archive_bytes": d.MaximumArchiveBytes, "maximum_expanded_bytes": d.MaximumExpandedBytes, "first": first.ArchivePath, "second": second.ArchivePath, "deterministic": true, "native_windows_execution": "UNATTESTED", "upstream_network": false}
	body, _ := json.MarshalIndent(receipt, "", "  ")
	if err = os.WriteFile(filepath.Join(output, "assembly-receipt.json"), append(body, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Log(string(body))
}
