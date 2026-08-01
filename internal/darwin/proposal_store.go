package darwin

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const proposalArtifactSchemaVersion = 1

var ErrProposalReplayConflict = errors.New("Darwin proposal artifact conflicts with an existing record")

// AssessmentProposalArtifact is the durable, metadata-only owner of a deep
// review assessment. Its filename and digest are the same immutable identity
// carried by the maintenance receipt.
type AssessmentProposalArtifact struct {
	SchemaVersion    int        `json:"schema_version"`
	RecordType       string     `json:"record_type"`
	AgentID          string     `json:"agent_id"`
	JobID            string     `json:"job_id"`
	OccurrenceDigest string     `json:"occurrence_digest"`
	WindowID         string     `json:"window_id"`
	ProposalDigest   string     `json:"proposal_digest"`
	Assessment       Assessment `json:"assessment"`
	ScheduledFor     time.Time  `json:"scheduled_for"`
	RecordedAt       time.Time  `json:"recorded_at"`
}

type ProposalStore struct{ Root string }

func (artifact AssessmentProposalArtifact) Validate() error {
	if artifact.SchemaVersion != proposalArtifactSchemaVersion || artifact.RecordType != "assessment" || artifact.AgentID != AgentID || (artifact.JobID != "darwin-deep-weekly" && artifact.JobID != "darwin-structural-evolution-proposal") || !validEvolutionSHA(artifact.OccurrenceDigest) || !validEvolutionSHA(artifact.ProposalDigest) || artifact.ScheduledFor.IsZero() || artifact.RecordedAt.IsZero() || !artifact.RecordedAt.Equal(artifact.ScheduledFor.UTC()) {
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
	if err := artifact.Validate(); err != nil {
		return err
	}
	path := filepath.Join(store.Root, "proposals", artifact.ProposalDigest+".json")
	if err := ensureProposalDirectory(store.Root, filepath.Dir(path)); err != nil {
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

func (store ProposalStore) Read(proposalDigest string) (AssessmentProposalArtifact, error) {
	if !validEvolutionSHA(proposalDigest) {
		return AssessmentProposalArtifact{}, errors.New("Darwin proposal digest is invalid")
	}
	path := filepath.Join(store.Root, "proposals", proposalDigest+".json")
	if err := validateProposalDirectory(store.Root, filepath.Dir(path)); err != nil {
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

func ensureProposalDirectory(root, directory string) error {
	if err := ensurePrivateDirectory(root); err != nil {
		return err
	}
	rootAbs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return err
	}
	directoryAbs, err := filepath.Abs(filepath.Clean(directory))
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(rootAbs, directoryAbs)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("Darwin proposal path escapes its store root")
	}
	current := rootAbs
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		if err := os.Mkdir(current, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("Darwin proposal path cannot traverse symlinks or non-directories")
		}
		if info.Mode().Perm() != 0o700 {
			if err := os.Chmod(current, 0o700); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateProposalDirectory(root, directory string) error {
	rootAbs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return err
	}
	directoryAbs, err := filepath.Abs(filepath.Clean(directory))
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(rootAbs, directoryAbs)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("Darwin proposal path escapes its store root")
	}
	current := rootAbs
	for _, component := range append([]string{"."}, strings.Split(relative, string(filepath.Separator))...) {
		if component != "." && component != "" {
			current = filepath.Join(current, component)
		}
		info, statErr := os.Lstat(current)
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("Darwin proposal path cannot traverse symlinks or non-directories")
		}
	}
	return nil
}
