package main

import (
	"bytes"
	"strings"
	"testing"
)

// Pure report/prompt projection. These fixtures never install a runtime, set an
// EngineeringRuntimeBinding, convert a document or grant a native qualification.
func TestInstalledPilotConversionJourneyBindsCurrentAndKeepsQuickPathsReadOnly(t *testing.T) {
	manifest, pack := strings.Repeat("a", 64), strings.Repeat("b", 64)
	r := pilotReport{SchemaVersion: 2, Status: "partial", WorkspaceID: strings.Repeat("c", 32), DesktopExtension: "maestro-connector-" + strings.Repeat("d", 32)}
	if err := installedConversionObservation(&r, manifest, pack, "ready", pack); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := finishPilotReport(&r, &output); err != nil {
		t.Fatal(err)
	}
	if r.Runtime != nil || r.Conversion == nil || r.Conversion.State != "ready_for_participant_test" {
		t.Fatal("diagnostic expectation coerced installed runtime into old pilot authority")
	}
	for _, prompt := range []string{r.CodePrompt, r.DesktopPrompt} {
		for _, literal := range []string{manifest, pack, pilotDocumentName, pilotDocumentSHA256, "MAESTRO_DOCLING_OFFLINE_20261101", "alpha/7", "result.status usable", "result.source_sha256", "provenance.manifest_sha256", "provenance.pack_sha256", "provenance.network_denied true", "result.artifact_ref", "artifact_id", "content.read", "markdown", "document", "max_bytes 4096", "next_offset", "has_more", "paginação completa", "não capture memória"} {
			if !strings.Contains(prompt, literal) {
				t.Fatalf("complete journey lacks real Current/source/provenance/readback requirement %q", literal)
			}
		}
	}
	for _, prompt := range []string{r.CodeReadyPrompt, r.DesktopReadyPrompt, r.CodeReadbackPrompt} {
		if strings.Contains(prompt, "content.convert") || strings.Contains(prompt, pilotDocumentName) {
			t.Fatal("read-only prompt acquired conversion/preparation")
		}
	}
	if !strings.Contains(r.Markdown, "sem autoridade de instalação") || !strings.Contains(r.Markdown, "não cria o documento no workspace") {
		t.Fatal("report lost preparation/authority boundary")
	}
}

func TestInstalledPilotConversionUnavailableAndIdentityMismatchNeverBecomePassed(t *testing.T) {
	manifest, pack := strings.Repeat("a", 64), strings.Repeat("b", 64)
	for _, state := range []string{"unavailable", "unqualified", "offline"} {
		t.Run(state, func(t *testing.T) {
			r := pilotReport{SchemaVersion: 2, Status: "partial", WorkspaceID: strings.Repeat("c", 32)}
			if err := installedConversionObservation(&r, manifest, pack, state, ""); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			if err := finishPilotReport(&r, &output); err != nil {
				t.Fatal(err)
			}
			if r.Conversion.State != "pending_runtime" || r.Checks[0].Status != "not_tested" || r.Runtime != nil {
				t.Fatal("unavailable runtime was promoted")
			}
			for _, prompt := range []string{r.CodePrompt, r.DesktopPrompt} {
				if !strings.Contains(prompt, "Conversão de documento: não testada") || strings.Contains(prompt, "Execute maestro_apply com operation content.convert") {
					t.Fatal("pending converter received an executable conversion journey")
				}
			}
		})
	}
	r := pilotReport{SchemaVersion: 2, Status: "partial", WorkspaceID: strings.Repeat("c", 32)}
	if err := installedConversionObservation(&r, manifest, pack, "ready", strings.Repeat("d", 64)); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := finishPilotReport(&r, &output); err == nil || r.Status != "blocked" || r.CodePrompt != "" || r.DesktopPrompt != "" {
		t.Fatal("ready but foreign runtime was promoted to a testable report")
	}
	if err := installedConversionObservation(&pilotReport{}, "not-a-current-manifest", pack, "ready", pack); err == nil {
		t.Fatal("unbound manifest became conversion expectation")
	}
}
