package darwin

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
)

const proposalArtifactSchemaVersion = 1

var ErrProposalReplayConflict = errors.New("Darwin proposal artifact conflicts with an existing record")

// AssessmentProposalArtifact is the durable, metadata-only owner of a deep
// review assessment. ArtifactID is stable for the scheduled occurrence; the
// content ProposalDigest may change if a fresh assessment would have changed,
// so retries must recover this exact artifact instead of rebuilding it.
type AssessmentProposalArtifact struct {
	SchemaVersion    int        `json:"schema_version"`
	RecordType       string     `json:"record_type"`
	AgentID          string     `json:"agent_id"`
	JobID            string     `json:"job_id"`
	OccurrenceDigest string     `json:"occurrence_digest"`
	ArtifactID       string     `json:"artifact_id"`
	WindowID         string     `json:"window_id"`
	ProposalDigest   string     `json:"proposal_digest"`
	Assessment       Assessment `json:"assessment"`
	ScheduledFor     time.Time  `json:"scheduled_for"`
	RecordedAt       time.Time  `json:"recorded_at"`
}

type ProposalStore struct{ Root string }

func (store ProposalStore) ValidateReceipt(receipt maintenance.Receipt) error {
	if receipt.State != maintenance.ReceiptProposalEmitted || receipt.ProposalDigest == "" || receipt.ProposalArtifactID == "" {
		return errors.New("Darwin proposal receipt has no valid artifact binding")
	}
	artifact, err := store.Read(receipt.ProposalArtifactID)
	if err != nil {
		return err
	}
	if artifact.ArtifactID != receipt.ProposalArtifactID || artifact.OccurrenceDigest != receipt.OccurrenceDigest || artifact.JobID != receipt.JobID || artifact.ProposalDigest != receipt.ProposalDigest || len(artifact.Assessment.Proposals) != receipt.ProposalCount {
		return errors.New("Darwin proposal receipt does not match its durable artifact")
	}
	return nil
}

func (artifact AssessmentProposalArtifact) Validate() error {
	if artifact.SchemaVersion != proposalArtifactSchemaVersion || artifact.RecordType != "assessment" || artifact.AgentID != AgentID || (artifact.JobID != "darwin-deep-weekly" && artifact.JobID != "darwin-structural-evolution-proposal") || !validEvolutionSHA(artifact.OccurrenceDigest) || !validEvolutionSHA(artifact.ArtifactID) || !validEvolutionSHA(artifact.ProposalDigest) || artifact.ArtifactID != assessmentArtifactID(artifact.JobID, artifact.OccurrenceDigest) || artifact.ScheduledFor.IsZero() || artifact.RecordedAt.IsZero() || !artifact.RecordedAt.Equal(artifact.ScheduledFor.UTC()) {
		return errors.New("Darwin proposal artifact header is invalid")
	}
	if artifact.Assessment.SchemaVersion != SchemaVersion || artifact.Assessment.AgentID != AgentID || artifact.Assessment.DisplayName != DisplayName || artifact.Assessment.Emoji != Emoji || artifact.Assessment.WindowID != artifact.WindowID || artifact.Assessment.Mode != DeepReview || len(artifact.Assessment.Proposals) < 1 || len(artifact.Assessment.Proposals) > maxActions {
		return errors.New("Darwin proposal artifact assessment is invalid")
	}
	for _, proposal := range artifact.Assessment.Proposals {
		if !idPattern.MatchString(proposal.ID) || !validCodes[proposal.Finding] || proposal.Priority < 1 || proposal.Priority > maxActions || !validImpact(proposal.Impact) || !validEffort(proposal.Effort) || !validRisk(proposal.Risk) || !validAction(proposal.Action) || !validAction(proposal.Rollback) || !proposal.Reversible {
			return errors.New("Darwin proposal artifact contains invalid proposal metadata")
		}
	}
	if proposalDigest(artifact.OccurrenceDigest, artifact.Assessment) != artifact.ProposalDigest {
		return errors.New("Darwin proposal artifact digest is stale")
	}
	return nil
}

func validImpact(impact Impact) bool {
	switch impact {
	case ImpactReliability, ImpactRecovery, ImpactSafety, ImpactFriction:
		return true
	default:
		return false
	}
}

func validEffort(effort Effort) bool { return effort == EffortSmall || effort == EffortMedium }
func validRisk(risk Risk) bool       { return risk == RiskLow || risk == RiskMedium }

func (store ProposalStore) Append(artifact AssessmentProposalArtifact) error {
	if strings.TrimSpace(store.Root) == "" {
		return errors.New("Darwin proposal store root is required")
	}
	if artifact.ArtifactID == "" {
		artifact.ArtifactID = assessmentArtifactID(artifact.JobID, artifact.OccurrenceDigest)
	}
	if err := artifact.Validate(); err != nil {
		return err
	}
	root, err := canonicalProposalRoot(store.Root)
	if err != nil {
		return err
	}
	path := filepath.Join(root, "proposals", artifact.ArtifactID+".json")
	if err := ensureProposalDirectory(root, filepath.Dir(path)); err != nil {
		return err
	}
	body, err := json.Marshal(artifact)
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if info, statErr := os.Lstat(path); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("Darwin proposal artifact path is not a regular file")
		}
		existing, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if bytes.Equal(bytes.TrimSpace(existing), bytes.TrimSpace(body)) {
			return nil
		}
		return ErrProposalReplayConflict
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".proposal-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(body); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := publishEvolutionFile(temporaryPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			existing, readErr := os.ReadFile(path)
			if readErr == nil && bytes.Equal(bytes.TrimSpace(existing), bytes.TrimSpace(body)) {
				return nil
			}
			return ErrProposalReplayConflict
		}
		return err
	}
	return nil
}

func (store ProposalStore) Read(artifactID string) (AssessmentProposalArtifact, error) {
	if !validEvolutionSHA(artifactID) {
		return AssessmentProposalArtifact{}, errors.New("Darwin proposal artifact ID is invalid")
	}
	root, err := canonicalProposalRoot(store.Root)
	if err != nil {
		return AssessmentProposalArtifact{}, err
	}
	path := filepath.Join(root, "proposals", artifactID+".json")
	if err := validateProposalDirectory(root, filepath.Dir(path)); err != nil {
		return AssessmentProposalArtifact{}, err
	}
	var artifact AssessmentProposalArtifact
	if err := readEvolutionJSON(path, &artifact); err != nil {
		return AssessmentProposalArtifact{}, err
	}
	if err := artifact.Validate(); err != nil {
		return AssessmentProposalArtifact{}, err
	}
	return artifact, nil
}

func (store ProposalStore) ReadOccurrence(jobID, occurrenceDigest string) (AssessmentProposalArtifact, error) {
	if jobID != "darwin-deep-weekly" && jobID != "darwin-structural-evolution-proposal" {
		return AssessmentProposalArtifact{}, errors.New("Darwin proposal job is invalid")
	}
	if !validEvolutionSHA(occurrenceDigest) {
		return AssessmentProposalArtifact{}, errors.New("Darwin occurrence digest is invalid")
	}
	return store.Read(assessmentArtifactID(jobID, occurrenceDigest))
}

func assessmentArtifactID(jobID, occurrenceDigest string) string {
	sum := sha256.Sum256([]byte("darwin-assessment-artifact-v1\x00" + jobID + "\x00" + occurrenceDigest))
	return hex.EncodeToString(sum[:])
}

// canonicalProposalRoot keeps the OS-provided temporary-directory alias
// (notably /tmp -> /private/tmp and /var -> /private/var on macOS) from being
// mistaken for a user-controlled redirect. User-created symlinks below that
// physical prefix remain visible to rejectEvolutionSymlinkAncestors.
func canonicalProposalRoot(root string) (string, error) {
	absolute, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	temporary, err := filepath.Abs(filepath.Clean(os.TempDir()))
	if err != nil {
		return "", err
	}
	physicalTemporary, err := filepath.EvalSymlinks(temporary)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(temporary, absolute)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.Join(physicalTemporary, relative), nil
	}
	return absolute, nil
}

func ensureProposalDirectory(root, directory string) error {
	return ensureEvolutionDirectory(root, directory)
}

func validateProposalDirectory(root, directory string) error {
	return validateEvolutionPath(root, directory)
}
