// Package paexpert owns the narrow boundary between a client/account or case
// agent and the centrally maintained PA Expert. It deliberately does not
// expose a client, workspace or filesystem context to the expert.
package paexpert

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/activationpolicy"
)

const SchemaVersion = 1

var (
	safeCode = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)
	semver   = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)
)

// Fact is intentionally code-only. The source agent must declassify into a
// stable vocabulary before a packet can cross into PA Expert scope.
type Fact struct {
	Code           string `json:"code"`
	Classification string `json:"classification"`
	ValueCode      string `json:"value_code"`
}

// Attestation is an explicit assertion made by the source boundary. The
// validator still checks the closed packet shape; this is not a prompt-level
// privacy promise.
type Attestation struct {
	ExporterID               string `json:"exporter_id"`
	NoClientIdentifiers      bool   `json:"no_client_identifiers"`
	NoStakeholderIdentifiers bool   `json:"no_stakeholder_identifiers"`
	NoRawExcerpts            bool   `json:"no_raw_excerpts"`
	NoScopedPointers         bool   `json:"no_scoped_pointers"`
}

type Request struct {
	SchemaVersion  int                       `json:"schema_version"`
	PacketID       string                    `json:"packet_id"`
	SourceSHA256   string                    `json:"source_sha256"`
	PlanSHA256     string                    `json:"plan_sha256"`
	Expert         activationpolicy.PAExpert `json:"expert"`
	QuestionCode   string                    `json:"question_code"`
	Classification string                    `json:"classification"`
	Facts          []Fact                    `json:"facts"`
	OutputSections []string                  `json:"output_sections"`
	Attestation    Attestation               `json:"attestation"`
}

// Packet is the only input accepted by a PA Expert. It contains opaque
// digests and closed codes, never source IDs, names, paths or excerpts.
type Packet struct {
	SchemaVersion  int                       `json:"schema_version"`
	PacketID       string                    `json:"packet_id"`
	SourceSHA256   string                    `json:"source_sha256"`
	PlanSHA256     string                    `json:"plan_sha256"`
	Expert         activationpolicy.PAExpert `json:"expert"`
	QuestionCode   string                    `json:"question_code"`
	Classification string                    `json:"classification"`
	Facts          []Fact                    `json:"facts"`
	OutputSections []string                  `json:"output_sections"`
	PacketSHA256   string                    `json:"packet_sha256"`
}

type Receipt struct {
	SchemaVersion int    `json:"schema_version"`
	PacketSHA256  string `json:"packet_sha256"`
	ExpertID      string `json:"expert_id"`
	ExpertVersion string `json:"expert_version"`
	CanonSHA256   string `json:"canon_sha256"`
	Outcome       string `json:"outcome"`
	MayExport     bool   `json:"may_export"`
	Evidence      string `json:"evidence_authority"`
}

type Response struct {
	SchemaVersion int      `json:"schema_version"`
	PacketSHA256  string   `json:"packet_sha256"`
	ExpertID      string   `json:"expert_id"`
	ExpertVersion string   `json:"expert_version"`
	CanonSHA256   string   `json:"canon_sha256"`
	Findings      []string `json:"findings"`
	Assumptions   []string `json:"assumptions"`
	Challenges    []string `json:"challenges"`
	Cautions      []string `json:"application_cautions"`
}

func Declassify(input Request) (Packet, Receipt, error) {
	if err := validateRequest(input); err != nil {
		return Packet{}, Receipt{}, err
	}
	packet := Packet{
		SchemaVersion: input.SchemaVersion, PacketID: input.PacketID,
		SourceSHA256: strings.ToLower(input.SourceSHA256), PlanSHA256: strings.ToLower(input.PlanSHA256),
		Expert: input.Expert, QuestionCode: input.QuestionCode,
		Classification: input.Classification, Facts: append([]Fact(nil), input.Facts...),
		OutputSections: SortOutputSections(input.OutputSections),
	}
	sort.Slice(packet.Facts, func(i, j int) bool {
		if packet.Facts[i].Code == packet.Facts[j].Code {
			return packet.Facts[i].ValueCode < packet.Facts[j].ValueCode
		}
		return packet.Facts[i].Code < packet.Facts[j].Code
	})
	packet.PacketSHA256 = digestPacket(packet)
	receipt := Receipt{
		SchemaVersion: SchemaVersion, PacketSHA256: packet.PacketSHA256,
		ExpertID: packet.Expert.ID, ExpertVersion: packet.Expert.Version,
		CanonSHA256: strings.ToLower(packet.Expert.CanonSHA256),
		Outcome:     "declassified_shadow", MayExport: false,
		Evidence: "content_free_shadow_receipt",
	}
	return packet, receipt, nil
}

// ValidateResponse checks a bounded answer and returns a receipt that can be
// attached to the accountable agent's route. It never grants export authority.
func ValidateResponse(packet Packet, response Response, receipt Receipt) (Receipt, error) {
	if err := validatePacket(packet); err != nil {
		return Receipt{}, err
	}
	if receipt.SchemaVersion != SchemaVersion || receipt.PacketSHA256 != packet.PacketSHA256 ||
		receipt.ExpertID != packet.Expert.ID || receipt.ExpertVersion != packet.Expert.Version ||
		receipt.CanonSHA256 != strings.ToLower(packet.Expert.CanonSHA256) ||
		receipt.Outcome != "declassified_shadow" || receipt.MayExport ||
		receipt.Evidence != "content_free_shadow_receipt" {
		return Receipt{}, errors.New("PA Expert receipt is stale, export-authorizing or mismatched")
	}
	if response.SchemaVersion != SchemaVersion || response.PacketSHA256 != packet.PacketSHA256 ||
		response.ExpertID != packet.Expert.ID || response.ExpertVersion != packet.Expert.Version ||
		response.CanonSHA256 != strings.ToLower(packet.Expert.CanonSHA256) {
		return Receipt{}, errors.New("PA Expert response does not match the declassified packet")
	}
	values := append(append(append(append([]string{}, response.Findings...), response.Assumptions...), response.Challenges...), response.Cautions...)
	if len(values) == 0 || len(values) > 24 {
		return Receipt{}, errors.New("PA Expert response exceeds its bounded contract")
	}
	for _, value := range values {
		if !validResponseValue(value) {
			return Receipt{}, errors.New("PA Expert response contains scoped or raw content")
		}
	}
	return receipt, nil
}

func validateRequest(input Request) error {
	if input.SchemaVersion != SchemaVersion || !validCode(input.PacketID) ||
		!validDigest(input.SourceSHA256) || !validDigest(input.PlanSHA256) ||
		!validExpert(input.Expert) || !validCode(input.QuestionCode) ||
		(input.Classification != "public" && input.Classification != "internal") {
		return errors.New("PA Expert packet identity or expert binding is invalid")
	}
	if !validCode(input.Attestation.ExporterID) || !input.Attestation.NoClientIdentifiers ||
		!input.Attestation.NoStakeholderIdentifiers || !input.Attestation.NoRawExcerpts ||
		!input.Attestation.NoScopedPointers {
		return errors.New("PA Expert packet lacks a complete declassification attestation")
	}
	if len(input.Facts) > 12 || len(input.OutputSections) == 0 || len(input.OutputSections) > 6 {
		return errors.New("PA Expert packet exceeds its bounded contract")
	}
	seen := map[string]bool{}
	for _, fact := range input.Facts {
		if !validCode(fact.Code) || !validCode(fact.ValueCode) ||
			(fact.Classification != "public" && fact.Classification != "internal") || seen[fact.Code] {
			return errors.New("PA Expert fact is not a unique declassified code")
		}
		seen[fact.Code] = true
	}
	for _, section := range input.OutputSections {
		if !allowedSection(section) || seen["section:"+section] {
			return errors.New("PA Expert output section is unsupported or duplicated")
		}
		seen["section:"+section] = true
	}
	return nil
}

func validatePacket(packet Packet) error {
	if err := validateRequest(Request{
		SchemaVersion: packet.SchemaVersion, PacketID: packet.PacketID,
		SourceSHA256: packet.SourceSHA256, PlanSHA256: packet.PlanSHA256,
		Expert: packet.Expert, QuestionCode: packet.QuestionCode,
		Classification: packet.Classification, Facts: packet.Facts,
		OutputSections: packet.OutputSections,
		Attestation: Attestation{ExporterID: "maestro", NoClientIdentifiers: true,
			NoStakeholderIdentifiers: true, NoRawExcerpts: true, NoScopedPointers: true},
	}); err != nil {
		return err
	}
	if packet.PacketSHA256 == "" || packet.PacketSHA256 != digestPacket(packet) {
		return errors.New("PA Expert packet digest is invalid")
	}
	return nil
}

func digestPacket(packet Packet) string {
	packet.PacketSHA256 = ""
	body, _ := json.Marshal(packet)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func validExpert(expert activationpolicy.PAExpert) bool {
	return validCode(expert.ID) && (expert.Kind == activationpolicy.ExpertFPA || expert.Kind == activationpolicy.ExpertIPA) &&
		semver.MatchString(expert.Version) && validDigest(expert.CanonSHA256) &&
		expert.Lifecycle == activationpolicy.Published
}

func validCode(value string) bool {
	if len(value) == 0 || len(value) > 80 || !safeCode.MatchString(value) {
		return false
	}
	lower := strings.ToLower(value)
	for _, forbidden := range []string{"client", "account", "case", "workspace", "stakeholder", "person", "raw", "file"} {
		if lower == forbidden || strings.HasPrefix(lower, forbidden+"-") || strings.Contains(lower, "-"+forbidden+"-") {
			return false
		}
	}
	return true
}

func validDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validResponseValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || len([]byte(trimmed)) > 500 || strings.ContainsAny(trimmed, "\r\n") {
		return false
	}
	lower := strings.ToLower(trimmed)
	for _, forbidden := range []string{"bcgos://", "workspace://", "account://", "case://", "file://", "/", "\\", "stakeholder", "client id", "workspace id", "case id"} {
		if strings.Contains(lower, forbidden) {
			return false
		}
	}
	return true
}

func allowedSection(value string) bool {
	for _, candidate := range []string{"findings", "assumptions", "challenges", "application_cautions"} {
		if value == candidate {
			return true
		}
	}
	return false
}

// SortOutputSections is useful to callers that want a canonical display while
// keeping packet construction deterministic.
func SortOutputSections(sections []string) []string {
	result := append([]string(nil), sections...)
	sort.Strings(result)
	return result
}
