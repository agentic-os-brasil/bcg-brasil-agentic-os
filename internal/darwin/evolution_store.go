package darwin

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// EvolutionPersistenceSchemaVersion is independent from both the health
// receipt schema and the proposal schema. This keeps recovery changes from
// silently changing the operational surgeon contract.
const EvolutionPersistenceSchemaVersion = 1

var evolutionSemver = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

var (
	ErrEvolutionReplayConflict        = errors.New("Darwin evolution replay conflicts with an existing record")
	ErrNativeEvolutionPersistence     = errors.New("Darwin native evolution persistence is unavailable")
	ErrEvolutionRevisionConflict      = errors.New("Darwin evolution episode revision conflict")
	ErrEvolutionWindowVersionConflict = errors.New("Darwin evidence window version conflict")
)

type EvolutionEpisodeState string

const (
	EpisodeOpen        EvolutionEpisodeState = "open"
	EpisodeInterrupted EvolutionEpisodeState = "interrupted"
	EpisodeResumed     EvolutionEpisodeState = "resumed"
	EpisodeClosed      EvolutionEpisodeState = "closed"
)

type EvolutionEventState string

const (
	EventInterrupted EvolutionEventState = "interrupted"
	EventResumed     EvolutionEventState = "resumed"
	EventClosed      EvolutionEventState = "closed"
)

type PolicyPin struct {
	PolicyID      string `json:"policy_id"`
	PolicyVersion string `json:"policy_version"`
	PlanSHA256    string `json:"plan_sha256"`
}

// PortfolioExpert is an immutable copy of an approved PA Expert registry
// entry. It contains no canon body and can therefore be safely persisted as
// metadata-only evidence.
type PortfolioExpert struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Version     string `json:"version"`
	CanonSHA256 string `json:"canon_sha256"`
}

type PortfolioSnapshot struct {
	SchemaVersion     int               `json:"schema_version"`
	Authority         string            `json:"authority"`
	ApprovedBy        string            `json:"approved_by"`
	ApprovalRefSHA256 string            `json:"approval_ref_sha256"`
	Experts           []PortfolioExpert `json:"experts"`
	SnapshotSHA256    string            `json:"snapshot_sha256"`
}

type EvolutionEpisode struct {
	RecordType    string                `json:"record_type"`
	SchemaVersion int                   `json:"schema_version"`
	EpisodeID     string                `json:"episode_id"`
	WindowID      string                `json:"window_id"`
	Policy        PolicyPin             `json:"policy"`
	Portfolio     PortfolioSnapshot     `json:"portfolio"`
	State         EvolutionEpisodeState `json:"state"`
	Revision      int                   `json:"revision"`
	CreatedAt     time.Time             `json:"created_at"`
	UpdatedAt     time.Time             `json:"updated_at"`
}

// EpisodeEvent is an append-only lifecycle marker. It deliberately has no
// objective, checkpoint, prompt, response or tool payload.
type EpisodeEvent struct {
	RecordType    string              `json:"record_type"`
	SchemaVersion int                 `json:"schema_version"`
	EventID       string              `json:"event_id"`
	EpisodeID     string              `json:"episode_id"`
	Revision      int                 `json:"revision"`
	State         EvolutionEventState `json:"state"`
	RecordedAt    time.Time           `json:"recorded_at"`
	EventSHA256   string              `json:"event_sha256"`
}

type EpisodeObservation struct {
	SchemaVersion   int    `json:"schema_version"`
	EpisodeID       string `json:"episode_id"`
	PlanSHA256      string `json:"plan_sha256"`
	Route           string `json:"route"`
	Outcome         string `json:"outcome"`
	DurationSeconds int    `json:"duration_seconds"`
	BudgetExhausted bool   `json:"budget_exhausted"`
	MissingReceipt  bool   `json:"missing_receipt"`
	HumanOverride   bool   `json:"human_override"`
}

type EvidenceWindow struct {
	RecordType    string               `json:"record_type"`
	SchemaVersion int                  `json:"schema_version"`
	WindowID      string               `json:"window_id"`
	Version       int                  `json:"version"`
	Policy        PolicyPin            `json:"policy"`
	Portfolio     PortfolioSnapshot    `json:"portfolio"`
	Observations  []EpisodeObservation `json:"observations"`
	WindowSHA256  string               `json:"window_sha256"`
	RecordedAt    time.Time            `json:"recorded_at"`
}

type EvolutionProposalArtifact struct {
	RecordType           string             `json:"record_type"`
	SchemaVersion        int                `json:"schema_version"`
	Proposal             StructuralProposal `json:"proposal"`
	Policy               PolicyPin          `json:"policy"`
	Portfolio            PortfolioSnapshot  `json:"portfolio"`
	EvidenceWindowID     string             `json:"evidence_window_id"`
	EvidenceWindowSHA256 string             `json:"evidence_window_sha256"`
	ProposalSHA256       string             `json:"proposal_sha256"`
	CreatedAt            time.Time          `json:"created_at"`
}

type EvolutionDecisionReceipt struct {
	RecordType        string    `json:"record_type"`
	SchemaVersion     int       `json:"schema_version"`
	ReceiptID         string    `json:"receipt_id"`
	ProposalID        string    `json:"proposal_id"`
	ProposalSHA256    string    `json:"proposal_sha256"`
	Decision          string    `json:"decision"`
	ClaimedApproverID string    `json:"claimed_approver_id"`
	AuthorityState    string    `json:"authority_state"`
	MayAuthorize      bool      `json:"may_authorize"`
	RecordedAt        time.Time `json:"recorded_at"`
	ReceiptSHA256     string    `json:"receipt_sha256"`
}

const callerAssertedDecisionAuthority = "caller_asserted_shadow"

type PersistenceCapabilityReport struct {
	SchemaVersion int    `json:"schema_version"`
	LocalState    string `json:"local_state"`
	NativeState   string `json:"native_state"`
	NativeReason  string `json:"native_reason"`
}

type EvolutionStore struct {
	Root string
	Now  func() time.Time
}

func (pin PolicyPin) Validate() error {
	if !validOpaqueEvolutionID(pin.PolicyID) || !validOpaqueEvolutionID(pin.PolicyVersion) || !validEvolutionSHA(pin.PlanSHA256) {
		return errors.New("Darwin evolution policy pin is invalid")
	}
	return nil
}

func (portfolio PortfolioSnapshot) Validate() error {
	if portfolio.SchemaVersion != EvolutionPersistenceSchemaVersion || !validOpaqueEvolutionID(portfolio.Authority) ||
		!validOpaqueEvolutionID(portfolio.ApprovedBy) || strings.EqualFold(portfolio.ApprovedBy, AgentID) ||
		!validEvolutionSHA(portfolio.ApprovalRefSHA256) || !validEvolutionSHA(portfolio.SnapshotSHA256) || len(portfolio.Experts) > 16 {
		return errors.New("Darwin PA Expert portfolio snapshot is invalid")
	}
	copyValue := portfolio
	copyValue.SnapshotSHA256 = ""
	if digestJSON(copyValue) != portfolio.SnapshotSHA256 {
		return errors.New("Darwin PA Expert portfolio snapshot digest is stale")
	}
	previous := ""
	for _, expert := range portfolio.Experts {
		if !idPattern.MatchString(expert.ID) || expert.ID <= previous ||
			(expert.Kind != "FPA" && expert.Kind != "IPA") || !evolutionSemver.MatchString(expert.Version) ||
			!validEvolutionSHA(expert.CanonSHA256) {
			return errors.New("Darwin PA Expert portfolio contains an invalid or unsorted entry")
		}
		previous = expert.ID
	}
	return nil
}

func (episode EvolutionEpisode) Validate() error {
	if episode.RecordType != "episode" || episode.SchemaVersion != EvolutionPersistenceSchemaVersion ||
		!idPattern.MatchString(episode.EpisodeID) || !idPattern.MatchString(episode.WindowID) ||
		episode.State != EpisodeOpen || episode.Revision != 0 || episode.CreatedAt.IsZero() ||
		!episode.UpdatedAt.Equal(episode.CreatedAt) {
		return errors.New("Darwin evolution episode is invalid")
	}
	if err := episode.Policy.Validate(); err != nil {
		return err
	}
	return episode.Portfolio.Validate()
}

func (event EpisodeEvent) Validate() error {
	if event.RecordType != "episode_event" || event.SchemaVersion != EvolutionPersistenceSchemaVersion ||
		!idPattern.MatchString(event.EventID) || !idPattern.MatchString(event.EpisodeID) || event.Revision < 1 ||
		!validEventState(event.State) || event.RecordedAt.IsZero() || !validEvolutionSHA(event.EventSHA256) {
		return errors.New("Darwin evolution episode event is invalid")
	}
	copyValue := event
	copyValue.EventSHA256 = ""
	if digestJSON(copyValue) != event.EventSHA256 {
		return errors.New("Darwin evolution episode event digest is stale")
	}
	return nil
}

func (observation EpisodeObservation) Validate() error {
	if observation.SchemaVersion != EvolutionPersistenceSchemaVersion || !idPattern.MatchString(observation.EpisodeID) ||
		!validEvolutionSHA(observation.PlanSHA256) || !validEvolutionRoute(observation.Route) ||
		!validEvolutionOutcome(observation.Outcome) || observation.DurationSeconds < 0 || observation.DurationSeconds > 86400 {
		return errors.New("Darwin evolution observation is invalid")
	}
	return nil
}

func (window EvidenceWindow) Validate() error {
	if window.RecordType != "evidence_window" || window.SchemaVersion != EvolutionPersistenceSchemaVersion ||
		!idPattern.MatchString(window.WindowID) || window.Version < 1 || window.RecordedAt.IsZero() ||
		!validEvolutionSHA(window.WindowSHA256) || len(window.Observations) > 1000 {
		return errors.New("Darwin evidence window is invalid")
	}
	if err := window.Policy.Validate(); err != nil {
		return err
	}
	if err := window.Portfolio.Validate(); err != nil {
		return err
	}
	copyValue := window
	copyValue.WindowSHA256 = ""
	if digestJSON(copyValue) != window.WindowSHA256 {
		return errors.New("Darwin evidence window digest is stale")
	}
	seen := map[string]bool{}
	for _, observation := range window.Observations {
		if err := observation.Validate(); err != nil || seen[observation.EpisodeID] ||
			observation.PlanSHA256 != window.Policy.PlanSHA256 {
			return errors.New("Darwin evidence window contains an invalid or duplicate episode")
		}
		seen[observation.EpisodeID] = true
	}
	return nil
}

func (proposal EvolutionProposalArtifact) Validate() error {
	if proposal.RecordType != "proposal" || proposal.SchemaVersion != EvolutionPersistenceSchemaVersion || proposal.CreatedAt.IsZero() ||
		!validEvolutionSHA(proposal.EvidenceWindowSHA256) || !validEvolutionSHA(proposal.ProposalSHA256) || !idPattern.MatchString(proposal.EvidenceWindowID) {
		return errors.New("Darwin evolution proposal artifact is invalid")
	}
	if err := proposal.Proposal.Validate(); err != nil {
		return err
	}
	if err := proposal.Policy.Validate(); err != nil {
		return err
	}
	if proposal.Proposal.PolicyVersion != proposal.Policy.PolicyVersion ||
		proposal.Proposal.EvidenceWindow != proposal.EvidenceWindowID {
		return errors.New("Darwin evolution proposal policy version is not pinned")
	}
	if err := proposal.Portfolio.Validate(); err != nil {
		return err
	}
	copyValue := proposal
	copyValue.ProposalSHA256 = ""
	if digestJSON(copyValue) != proposal.ProposalSHA256 {
		return errors.New("Darwin evolution proposal digest is stale")
	}
	return nil
}

func (receipt EvolutionDecisionReceipt) Validate() error {
	if receipt.RecordType != "decision_receipt" || receipt.SchemaVersion != EvolutionPersistenceSchemaVersion ||
		!idPattern.MatchString(receipt.ReceiptID) || !idPattern.MatchString(receipt.ProposalID) ||
		!validEvolutionSHA(receipt.ProposalSHA256) || receipt.Decision == "" ||
		(receipt.Decision != "approved" && receipt.Decision != "rejected") ||
		receipt.ClaimedApproverID != "walter" || receipt.AuthorityState != callerAssertedDecisionAuthority ||
		receipt.MayAuthorize || receipt.RecordedAt.IsZero() || !validEvolutionSHA(receipt.ReceiptSHA256) {
		return errors.New("Darwin evolution decision receipt is invalid or claims unavailable authority")
	}
	copyValue := receipt
	copyValue.ReceiptSHA256 = ""
	if digestJSON(copyValue) != receipt.ReceiptSHA256 {
		return errors.New("Darwin evolution decision receipt digest is stale")
	}
	return nil
}

func (store EvolutionStore) Capability() PersistenceCapabilityReport {
	return PersistenceCapabilityReport{
		SchemaVersion: EvolutionPersistenceSchemaVersion,
		LocalState:    "available",
		NativeState:   "unavailable",
		NativeReason:  "native_persistence_not_qualified",
	}
}

func NativeEvolutionPersistence() error { return ErrNativeEvolutionPersistence }

func (store EvolutionStore) AppendEpisode(episode EvolutionEpisode) error {
	if episode.RecordType == "" {
		episode.RecordType = "episode"
	}
	if episode.SchemaVersion == 0 {
		episode.SchemaVersion = EvolutionPersistenceSchemaVersion
	}
	if err := episode.Validate(); err != nil {
		return err
	}
	return store.appendJSON(filepath.Join(store.episodeRoot(episode.EpisodeID), "episode.json"), episode)
}

func (store EvolutionStore) AppendEpisodeEvent(event EpisodeEvent) error {
	if event.RecordType == "" {
		event.RecordType = "episode_event"
	}
	if event.SchemaVersion == 0 {
		event.SchemaVersion = EvolutionPersistenceSchemaVersion
	}
	if event.EventSHA256 == "" {
		copyValue := event
		copyValue.EventSHA256 = ""
		event.EventSHA256 = digestJSON(copyValue)
	}
	if err := event.Validate(); err != nil {
		return err
	}
	path := filepath.Join(store.episodeRoot(event.EpisodeID), "events", fmt.Sprintf("%06d.json", event.Revision))
	if _, err := os.Stat(path); err == nil {
		return store.appendJSON(path, event)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	current, _, err := store.RecoverEpisode(event.EpisodeID)
	if err != nil {
		return err
	}
	if event.Revision != current.Revision+1 || !validEpisodeTransition(current.State, event.State) {
		return ErrEvolutionRevisionConflict
	}
	return store.appendJSON(path, event)
}

func (store EvolutionStore) RecoverEpisode(episodeID string) (EvolutionEpisode, []EpisodeEvent, error) {
	if !idPattern.MatchString(episodeID) {
		return EvolutionEpisode{}, nil, errors.New("invalid Darwin evolution episode ID")
	}
	path := filepath.Join(store.episodeRoot(episodeID), "episode.json")
	if err := validateEvolutionPath(store.Root, path); err != nil {
		return EvolutionEpisode{}, nil, err
	}
	var episode EvolutionEpisode
	if err := readEvolutionJSON(path, &episode); err != nil {
		return EvolutionEpisode{}, nil, err
	}
	if err := episode.Validate(); err != nil {
		return EvolutionEpisode{}, nil, err
	}
	eventsRoot := filepath.Join(store.episodeRoot(episodeID), "events")
	if err := validateEvolutionPath(store.Root, eventsRoot); err != nil {
		return EvolutionEpisode{}, nil, err
	}
	entries, err := os.ReadDir(eventsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return episode, nil, nil
	}
	if err != nil {
		return EvolutionEpisode{}, nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	events := make([]EpisodeEvent, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return EvolutionEpisode{}, nil, errors.New("Darwin evolution episode recovery found a symlinked revision")
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		expectedName := fmt.Sprintf("%06d.json", len(events)+1)
		if entry.Name() != expectedName {
			return EvolutionEpisode{}, nil, errors.New("Darwin evolution episode recovery found a non-canonical revision filename")
		}
		var event EpisodeEvent
		if err := readEvolutionJSON(filepath.Join(store.episodeRoot(episodeID), "events", entry.Name()), &event); err != nil {
			return EvolutionEpisode{}, nil, err
		}
		if err := event.Validate(); err != nil || event.EpisodeID != episodeID || event.Revision != len(events)+1 || !validEpisodeTransition(episode.State, event.State) {
			return EvolutionEpisode{}, nil, errors.New("Darwin evolution episode recovery found an invalid revision")
		}
		episode.State = EvolutionEpisodeState(event.State)
		episode.Revision = event.Revision
		episode.UpdatedAt = event.RecordedAt
		events = append(events, event)
	}
	return episode, events, nil
}

func (store EvolutionStore) AppendWindow(window EvidenceWindow) error {
	if window.RecordType == "" {
		window.RecordType = "evidence_window"
	}
	if window.SchemaVersion == 0 {
		window.SchemaVersion = EvolutionPersistenceSchemaVersion
	}
	if window.WindowSHA256 == "" {
		copyValue := window
		copyValue.WindowSHA256 = ""
		window.WindowSHA256 = digestJSON(copyValue)
	}
	if err := window.Validate(); err != nil {
		return err
	}
	for _, observation := range window.Observations {
		episode, _, err := store.RecoverEpisode(observation.EpisodeID)
		if err != nil {
			return errors.New("Darwin evidence window references an unavailable or invalid episode")
		}
		if episode.WindowID != window.WindowID || episode.Policy != window.Policy ||
			episode.Portfolio.SnapshotSHA256 != window.Portfolio.SnapshotSHA256 {
			return errors.New("Darwin evidence window changed its episode policy, portfolio or window binding")
		}
	}
	directory := filepath.Join(store.evolutionRoot(), "windows", window.WindowID)
	path := filepath.Join(directory, fmt.Sprintf("v%06d.json", window.Version))
	if _, err := os.Stat(path); err == nil {
		return store.appendJSON(path, window)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	windows, err := store.RecoverWindow(window.WindowID)
	if err != nil {
		return err
	}
	if len(windows) > 0 && window.Version != windows[len(windows)-1].Version+1 {
		return ErrEvolutionWindowVersionConflict
	}
	if len(windows) == 0 && window.Version != 1 {
		return ErrEvolutionWindowVersionConflict
	}
	if len(windows) > 0 && (window.Policy != windows[0].Policy ||
		window.Portfolio.SnapshotSHA256 != windows[0].Portfolio.SnapshotSHA256) {
		return errors.New("Darwin evidence window versions cannot change policy or portfolio pins")
	}
	return store.appendJSON(path, window)
}

func (store EvolutionStore) RecoverWindow(windowID string) ([]EvidenceWindow, error) {
	if !idPattern.MatchString(windowID) {
		return nil, errors.New("invalid Darwin evidence window ID")
	}
	directory := filepath.Join(store.evolutionRoot(), "windows", windowID)
	if err := validateEvolutionPath(store.Root, directory); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	windows := make([]EvidenceWindow, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, errors.New("Darwin evidence window recovery found a symlinked version")
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		expectedName := fmt.Sprintf("v%06d.json", len(windows)+1)
		if entry.Name() != expectedName {
			return nil, errors.New("Darwin evidence window recovery found a non-canonical version filename")
		}
		var window EvidenceWindow
		if err := readEvolutionJSON(filepath.Join(store.evolutionRoot(), "windows", windowID, entry.Name()), &window); err != nil {
			return nil, err
		}
		if err := window.Validate(); err != nil || window.WindowID != windowID || window.Version != len(windows)+1 {
			return nil, errors.New("Darwin evidence window recovery found an invalid version")
		}
		if len(windows) > 0 && (window.Policy != windows[0].Policy || window.Portfolio.SnapshotSHA256 != windows[0].Portfolio.SnapshotSHA256) {
			return nil, errors.New("Darwin evidence window versions changed their policy or portfolio pin")
		}
		windows = append(windows, window)
	}
	return windows, nil
}

func (store EvolutionStore) AppendProposal(proposal EvolutionProposalArtifact) error {
	if proposal.RecordType == "" {
		proposal.RecordType = "proposal"
	}
	if proposal.SchemaVersion == 0 {
		proposal.SchemaVersion = EvolutionPersistenceSchemaVersion
	}
	if proposal.ProposalSHA256 == "" {
		copyValue := proposal
		copyValue.ProposalSHA256 = ""
		proposal.ProposalSHA256 = digestJSON(copyValue)
	}
	if err := proposal.Validate(); err != nil {
		return err
	}
	windows, err := store.RecoverWindow(proposal.EvidenceWindowID)
	if err != nil {
		return err
	}
	if len(windows) == 0 {
		return errors.New("Darwin evolution proposal references an unavailable evidence window")
	}
	window := windows[len(windows)-1]
	if window.WindowSHA256 != proposal.EvidenceWindowSHA256 || window.Policy != proposal.Policy || window.Portfolio.SnapshotSHA256 != proposal.Portfolio.SnapshotSHA256 {
		return errors.New("Darwin evolution proposal is not pinned to its evidence window")
	}
	return store.appendJSON(filepath.Join(store.evolutionRoot(), "proposals", proposal.Proposal.ProposalID+".json"), proposal)
}

func (store EvolutionStore) AppendDecision(receipt EvolutionDecisionReceipt) error {
	if receipt.RecordType == "" {
		receipt.RecordType = "decision_receipt"
	}
	if receipt.SchemaVersion == 0 {
		receipt.SchemaVersion = EvolutionPersistenceSchemaVersion
	}
	if receipt.AuthorityState == "" {
		receipt.AuthorityState = callerAssertedDecisionAuthority
	}
	if receipt.ReceiptSHA256 == "" {
		copyValue := receipt
		copyValue.ReceiptSHA256 = ""
		receipt.ReceiptSHA256 = digestJSON(copyValue)
	}
	if err := receipt.Validate(); err != nil {
		return err
	}
	proposalPath := filepath.Join(store.evolutionRoot(), "proposals", receipt.ProposalID+".json")
	if err := validateEvolutionPath(store.Root, proposalPath); err != nil {
		return err
	}
	var proposal EvolutionProposalArtifact
	if err := readEvolutionJSON(proposalPath, &proposal); err != nil {
		return errors.New("Darwin evolution decision references an unavailable proposal")
	}
	if err := proposal.Validate(); err != nil || proposal.ProposalSHA256 != receipt.ProposalSHA256 ||
		proposal.Proposal.ProposalID != receipt.ProposalID {
		return errors.New("Darwin evolution decision is not bound to the proposal digest")
	}
	// The proposal ID, rather than the caller-selected receipt ID, is the
	// no-clobber identity. This atomically fences concurrent approve/reject
	// claims for the same proposal.
	return store.appendJSON(filepath.Join(store.evolutionRoot(), "decisions", receipt.ProposalID+".json"), receipt)
}

func (store EvolutionStore) appendJSON(path string, value any) error {
	if strings.TrimSpace(store.Root) == "" {
		return errors.New("Darwin evolution store root is required")
	}
	if err := ensureEvolutionDirectory(store.Root, filepath.Dir(path)); err != nil {
		return err
	}
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("Darwin evolution record path cannot be a symlink")
		}
		existing, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if bytes.Equal(bytes.TrimSpace(existing), bytes.TrimSpace(body)) {
			return syncEvolutionDirectory(filepath.Dir(path))
		}
		return ErrEvolutionReplayConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".evolution-*.tmp")
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
				return syncEvolutionDirectory(filepath.Dir(path))
			}
			return ErrEvolutionReplayConflict
		}
		return err
	}
	return nil
}

func (store EvolutionStore) windowVersions(windowID string) ([]int, error) {
	entries, err := os.ReadDir(filepath.Join(store.evolutionRoot(), "windows", windowID))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	versions := []int{}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var version int
		if _, err := fmt.Sscanf(entry.Name(), "v%06d.json", &version); err != nil || version < 1 {
			return nil, errors.New("Darwin evidence window contains an invalid version filename")
		}
		versions = append(versions, version)
	}
	sort.Ints(versions)
	return versions, nil
}

func (store EvolutionStore) evolutionRoot() string { return filepath.Join(store.Root, "evolution") }

func (store EvolutionStore) episodeRoot(episodeID string) string {
	return filepath.Join(store.evolutionRoot(), "episodes", episodeID)
}

func (store EvolutionStore) now() time.Time {
	if store.Now != nil {
		return store.Now().UTC()
	}
	return time.Now().UTC()
}

func readEvolutionJSON(path string, target any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Darwin evolution record cannot be a symlink")
	}
	return readStrictJSON(path, target)
}

func ensureEvolutionDirectory(root, directory string) error {
	rootAbs, directoryAbs, err := boundedEvolutionPaths(root, directory)
	if err != nil {
		return err
	}
	if err := rejectEvolutionSymlinkAncestors(filepath.Dir(rootAbs)); err != nil {
		return err
	}
	_, rootStatErr := os.Lstat(rootAbs)
	rootWasMissing := errors.Is(rootStatErr, os.ErrNotExist)
	if rootStatErr != nil && !rootWasMissing {
		return rootStatErr
	}
	if err := os.MkdirAll(rootAbs, 0o700); err != nil {
		return err
	}
	if err := rejectEvolutionSymlinkAncestors(rootAbs); err != nil {
		return err
	}
	if rootWasMissing {
		if err := syncEvolutionDirectory(filepath.Dir(rootAbs)); err != nil {
			return err
		}
	}
	rootInfo, err := os.Lstat(rootAbs)
	if err != nil {
		return err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return errors.New("Darwin evolution store root cannot be a symlink or non-directory")
	}
	if rootInfo.Mode().Perm() != 0o700 {
		if err := os.Chmod(rootAbs, 0o700); err != nil {
			return err
		}
	}
	relative, _ := filepath.Rel(rootAbs, directoryAbs)
	current := rootAbs
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		created := false
		if err := os.Mkdir(current, 0o700); err != nil {
			if !errors.Is(err, os.ErrExist) {
				return err
			}
		} else {
			created = true
		}
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("Darwin evolution store path cannot traverse symlinks or non-directories")
		}
		if info.Mode().Perm() != 0o700 {
			if err := os.Chmod(current, 0o700); err != nil {
				return err
			}
		}
		if created {
			if err := syncEvolutionDirectory(filepath.Dir(current)); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateEvolutionPath(root, target string) error {
	rootAbs, targetAbs, err := boundedEvolutionPaths(root, target)
	if err != nil {
		return err
	}
	if err := rejectEvolutionSymlinkAncestors(targetAbs); err != nil {
		return err
	}
	if _, err := os.Lstat(rootAbs); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	rootInfo, err := os.Lstat(rootAbs)
	if err != nil {
		return err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return errors.New("Darwin evolution store root cannot be a symlink or non-directory")
	}
	relative, _ := filepath.Rel(rootAbs, targetAbs)
	current := rootAbs
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("Darwin evolution store path cannot traverse symlinks")
		}
	}
	return nil
}

func boundedEvolutionPaths(root, target string) (string, string, error) {
	if strings.TrimSpace(root) == "" {
		return "", "", errors.New("Darwin evolution store root is required")
	}
	rootAbs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", "", err
	}
	targetAbs, err := filepath.Abs(filepath.Clean(target))
	if err != nil {
		return "", "", err
	}
	relative, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", errors.New("Darwin evolution path escapes its store root")
	}
	return rootAbs, targetAbs, nil
}

func rejectEvolutionSymlinkAncestors(path string) error {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return err
	}
	volume := filepath.VolumeName(absolute)
	remainder := strings.TrimPrefix(absolute, volume)
	remainder = strings.TrimPrefix(remainder, string(filepath.Separator))
	current := volume + string(filepath.Separator)
	for _, component := range strings.Split(remainder, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("Darwin evolution store path cannot traverse symlinked ancestors")
		}
	}
	return nil
}

func digestJSON(value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func validEvolutionSHA(value string) bool { return evolutionDigest.MatchString(value) }

func validOpaqueEvolutionID(value string) bool { return idPattern.MatchString(value) }

func validEpisodeState(value EvolutionEpisodeState) bool {
	return value == EpisodeOpen || value == EpisodeInterrupted || value == EpisodeResumed || value == EpisodeClosed
}

func validEventState(value EvolutionEventState) bool {
	return value == EventInterrupted || value == EventResumed || value == EventClosed
}

func validEpisodeTransition(current EvolutionEpisodeState, next EvolutionEventState) bool {
	switch current {
	case EpisodeOpen:
		return next == EventInterrupted || next == EventClosed
	case EpisodeInterrupted:
		return next == EventResumed || next == EventClosed
	case EpisodeResumed:
		return next == EventInterrupted || next == EventClosed
	default:
		return false
	}
}

func validEvolutionRoute(value string) bool {
	switch value {
	case "D0_DIRECT", "D1_TARGETED", "D2_GOVERNED", "BLOCKED":
		return true
	default:
		return false
	}
}

func validEvolutionOutcome(value string) bool {
	switch value {
	case "completed", "failed", "blocked":
		return true
	default:
		return false
	}
}
