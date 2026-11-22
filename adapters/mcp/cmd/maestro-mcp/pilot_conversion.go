package main

import (
	"errors"
	"fmt"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/pilotdocument"
)

const pilotDocumentName = pilotdocument.Name
const pilotDocumentSHA256 = pilotdocument.SHA256

// Diagnostic expectations for an attended synthetic test, never runtime trust,
// an installation profile, permission, qualification or an imported proof.
type pilotConversionExpectation struct {
	State          string `json:"state"`
	ManifestSHA256 string `json:"expected_manifest_sha256"`
	PackSHA256     string `json:"expected_pack_sha256"`
	Source         string `json:"source"`
	SourceSHA256   string `json:"source_sha256"`
}

func (c pilotConversionExpectation) validate() error {
	if c.State != "ready_for_participant_test" && c.State != "pending_runtime" && c.State != "blocked_identity_mismatch" {
		return errors.New("closed diagnostic conversion state required")
	}
	if !pilotKitID.MatchString(c.ManifestSHA256) || !pilotKitID.MatchString(c.PackSHA256) || c.Source != pilotDocumentName || c.SourceSHA256 != pilotDocumentSHA256 {
		return errors.New("exact Current and public test-document expectations required")
	}
	return nil
}

func installedConversionObservation(r *pilotReport, manifest, expectedPack, state, observedPack string) error {
	c := pilotConversionExpectation{State: "pending_runtime", ManifestSHA256: manifest, PackSHA256: expectedPack, Source: pilotDocumentName, SourceSHA256: pilotDocumentSHA256}
	if err := c.validate(); err != nil {
		return err
	}
	if state == "ready" {
		if observedPack == expectedPack {
			c.State = "ready_for_participant_test"
			r.add("document_runtime", "passed", "current_converter_available_no_conversion_performed", "")
		} else {
			c.State = "blocked_identity_mismatch"
			r.add("document_runtime", "failed", "current_pack_differs_from_closed_rehearsal_role", "Preserve os dados e compartilhe o diagnóstico; nenhum runtime foi instalado ou alterado.")
		}
	} else {
		r.add("document_runtime", "not_tested", "converter_unavailable_no_placement_or_repair_performed", "Conclua a etapa admitida do conversor e registre sua disponibilidade antes do teste real.")
	}
	r.Conversion = &c
	return nil
}

func installedConversionJourney(c pilotConversionExpectation) string {
	return fmt.Sprintf(" Nesta etapa, teste somente o conversor local para o Current autenticado deste ensaio. Reconfirme o mesmo workspace com maestro_context e consulte maestro_query content.status. Continue somente com state ready e pack_sha256 exatamente %s; divergência ou indisponibilidade encerra esta parte sem instalar ou reparar nada. Use somente o arquivo sintético %s preparado no workspace pela etapa de documento, com SHA256 %s. Execute maestro_apply com operation content.convert e source_path %s. Exija result.status usable, result.source_sha256 exatamente %s, provenance.network_denied true, provenance.pack_sha256 exatamente %s e provenance.manifest_sha256 exatamente %s; qualquer erro ou divergência encerra a conversão sem inferir sucesso. Guarde somente o result.artifact_ref realmente retornado e use esse valor como artifact_id em maestro_query content.read. Leia os formatos markdown e document separadamente, começando em offset 0 com max_bytes 4096; siga next_offset enquanto has_more for true, exigindo avanço real e paginação completa. Reconfira a mesma provenance de pack e manifest nas páginas; erro, identidade divergente ou paginação incompleta é falha. Verifique o marcador MAESTRO_DOCLING_OFFLINE_20261101 (escapes Markdown são permitidos) e a tabela sintética alpha/7. Mostre a rota, o artefato e o resultado observado, separando passou, falhou e não testado. Essa parte autoriza conversão e leitura de artefatos derivados; preserve o documento de origem, não altere o estado do trabalho, não capture memória, não salve conhecimento nem use outro conversor ou conector.", c.PackSHA256, c.Source, c.SourceSHA256, c.Source, c.SourceSHA256, c.PackSHA256, c.ManifestSHA256)
}
