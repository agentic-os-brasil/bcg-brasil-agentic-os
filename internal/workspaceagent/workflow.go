package workspaceagent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Evidence struct {
	WorkspaceID      string    `json:"workspace_id"`
	PlanID           string    `json:"plan_id"`
	Query            string    `json:"query"`
	SourceURL        string    `json:"source_url"`
	RetrievedAt      time.Time `json:"retrieved_at"`
	ValidUntil       time.Time `json:"valid_until"`
	Claim            string    `json:"claim"`
	EvidenceStrength string    `json:"evidence_strength"`
	Classification   string    `json:"classification"`
}

type QueryExecution struct {
	WorkspaceID string    `json:"workspace_id"`
	PlanID      string    `json:"plan_id"`
	Query       string    `json:"query"`
	ExecutedAt  time.Time `json:"executed_at"`
	Slot        int       `json:"slot"`
}

type Thesis struct {
	Statement           string   `json:"statement"`
	Evidence            []string `json:"evidence"`
	Assumptions         []string `json:"assumptions"`
	CounterEvidence     []string `json:"counter_evidence"`
	InvalidationSignals []string `json:"invalidation_signals"`
}

type Brief struct {
	BriefID           string    `json:"brief_id"`
	WorkspaceID       string    `json:"workspace_id"`
	CreatedAt         time.Time `json:"created_at"`
	ReviewedBy        string    `json:"reviewed_by"`
	Classification    string    `json:"classification"`
	Mandate           string    `json:"mandate"`
	Decision          string    `json:"decision,omitempty"`
	TimeHorizon       string    `json:"time_horizon,omitempty"`
	Objectives        []string  `json:"objectives"`
	Stakeholders      []string  `json:"stakeholders"`
	Materials         []string  `json:"materials,omitempty"`
	Constraints       []string  `json:"constraints"`
	SuccessSignals    []string  `json:"success_signals,omitempty"`
	OpenQuestions     []string  `json:"open_questions,omitempty"`
	Bullish           []Thesis  `json:"bullish"`
	Bearish           []Thesis  `json:"bearish"`
	ResearchQuestions []string  `json:"research_questions"`
}

type briefReference struct {
	BriefID   string    `json:"brief_id"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PublicSource struct {
	URL         string    `json:"url"`
	RetrievedAt time.Time `json:"retrieved_at"`
}

type PublicClaim struct {
	Statement      string   `json:"statement"`
	Classification string   `json:"classification"`
	SourceURLs     []string `json:"source_urls"`
}

type PublicAttestation struct {
	AttestedBy            string    `json:"attested_by"`
	AttestedAt            time.Time `json:"attested_at"`
	Origin                string    `json:"origin"`
	NoWorkspaceDerivation bool      `json:"no_workspace_derivation"`
}

type EconomicSnapshot struct {
	SnapshotID  string            `json:"snapshot_id"`
	AsOf        time.Time         `json:"as_of"`
	CreatedAt   time.Time         `json:"created_at"`
	Claims      []PublicClaim     `json:"claims"`
	Sources     []PublicSource    `json:"sources"`
	Attestation PublicAttestation `json:"attestation"`
}

type economicReference struct {
	SnapshotID string    `json:"snapshot_id"`
	AttachedAt time.Time `json:"attached_at"`
}

func SaveBrief(dataRoot string, brief Brief) (Brief, error) {
	if err := validateBrief(brief); err != nil {
		return Brief{}, err
	}
	if status, err := Inspect(dataRoot, brief.WorkspaceID); err != nil || !status.Initialized {
		if err != nil {
			return Brief{}, err
		}
		return Brief{}, errors.New("workspace agent is not initialized")
	}
	brief.CreatedAt = time.Now().UTC()
	brief.BriefID = identifier("workspace-brief", brief.WorkspaceID, brief.Mandate, brief.CreatedAt.String())
	root := workspaceAgentRoot(dataRoot, brief.WorkspaceID)
	if err := createImmutableJSON(filepath.Join(root, "dossier", "briefs", brief.BriefID+".json"), brief); err != nil {
		return Brief{}, err
	}
	if err := writeAtomicJSON(filepath.Join(root, "agent", "current-brief.json"), briefReference{BriefID: brief.BriefID, UpdatedAt: brief.CreatedAt}); err != nil {
		return Brief{}, err
	}
	if err := updateOperationalState(dataRoot, brief.WorkspaceID, func(state *OperationalState) {
		state.Lifecycle = "active"
		state.CurrentBriefID = brief.BriefID
		state.CurrentObjective = brief.Objectives[0]
		state.UpdatedAt = brief.CreatedAt
	}); err != nil {
		return Brief{}, err
	}
	return brief, nil
}

func CreateResearchPlan(dataRoot string, plan ResearchPlan) (ResearchPlan, error) {
	if err := validateDraftPlan(plan); err != nil {
		return ResearchPlan{}, err
	}
	if status, err := Inspect(dataRoot, plan.WorkspaceID); err != nil || !status.Initialized {
		if err == nil {
			err = errors.New("workspace agent is not initialized")
		}
		return ResearchPlan{}, err
	}
	plan.State = "proposed"
	plan.CreatedAt = time.Now().UTC()
	plan.PlanID = identifier("research-plan", plan.WorkspaceID, plan.Purpose, plan.CreatedAt.String())
	path := researchPlanPath(dataRoot, plan.WorkspaceID, plan.PlanID, "proposed")
	if err := createImmutableJSON(path, plan); err != nil {
		return ResearchPlan{}, err
	}
	if err := updateOperationalState(dataRoot, plan.WorkspaceID, func(state *OperationalState) {
		state.CurrentResearchPlanID = plan.PlanID
		state.UpdatedAt = plan.CreatedAt
	}); err != nil {
		return ResearchPlan{}, err
	}
	return plan, nil
}

func ApproveResearchPlan(dataRoot, workspaceID, planID string, approval Approval) (ResearchPlan, error) {
	plan, err := readResearchPlan(dataRoot, workspaceID, planID, "proposed")
	if err != nil {
		return ResearchPlan{}, err
	}
	plan.State = "approved"
	plan.Approval = approval
	if err := plan.Validate(); err != nil {
		return ResearchPlan{}, err
	}
	path := researchPlanPath(dataRoot, workspaceID, planID, "approved")
	if err := createImmutableJSON(path, plan); err != nil {
		return ResearchPlan{}, err
	}
	if err := updateOperationalState(dataRoot, workspaceID, func(state *OperationalState) {
		state.CurrentResearchPlanID = plan.PlanID
		state.UpdatedAt = approval.ApprovedAt
	}); err != nil {
		return ResearchPlan{}, err
	}
	return plan, nil
}

func ConsumeResearchQuery(dataRoot string, execution QueryExecution) (QueryExecution, error) {
	plan, err := readResearchPlan(dataRoot, execution.WorkspaceID, execution.PlanID, "approved")
	if errors.Is(err, os.ErrNotExist) {
		return QueryExecution{}, ErrResearchApprovalRequired
	}
	if err != nil {
		return QueryExecution{}, err
	}
	if err := plan.Validate(); err != nil {
		return QueryExecution{}, err
	}
	if !approvedQuery(plan, execution.Query) {
		return QueryExecution{}, errors.New("research query is outside the approved themes")
	}
	execution.ExecutedAt = time.Now().UTC()
	root := filepath.Join(workspaceAgentRoot(dataRoot, execution.WorkspaceID), "research", "executions", execution.PlanID)
	for slot := 1; slot <= plan.MaxQueries; slot++ {
		execution.Slot = slot
		path := filepath.Join(root, fmt.Sprintf("slot-%03d.json", slot))
		err := createImmutableJSON(path, execution)
		if err == nil {
			return execution, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return QueryExecution{}, err
		}
	}
	return QueryExecution{}, errors.New("approved research query budget is exhausted")
}

func RecordEvidence(dataRoot string, evidence Evidence) error {
	plan, err := readResearchPlan(dataRoot, evidence.WorkspaceID, evidence.PlanID, "approved")
	if errors.Is(err, os.ErrNotExist) {
		return ErrResearchApprovalRequired
	}
	if err != nil {
		return err
	}
	if err := validateEvidence(dataRoot, plan, evidence); err != nil {
		return err
	}
	id := identifier("evidence", evidence.Query, evidence.SourceURL, evidence.Claim, evidence.RetrievedAt.String())
	path := filepath.Join(workspaceAgentRoot(dataRoot, evidence.WorkspaceID), "dossier", "evidence", id+".json")
	return createImmutableJSON(path, evidence)
}

func SaveEconomicSnapshot(dataRoot string, snapshot EconomicSnapshot) (EconomicSnapshot, error) {
	attestation := snapshot.Attestation
	if snapshot.AsOf.IsZero() || len(snapshot.Claims) == 0 || len(snapshot.Sources) == 0 ||
		strings.TrimSpace(attestation.AttestedBy) == "" || attestation.AttestedAt.IsZero() ||
		attestation.Origin != "independent_public_sources" || !attestation.NoWorkspaceDerivation {
		return EconomicSnapshot{}, errors.New("economic snapshot requires an independent-public-source attestation, claims and sources")
	}
	sourceURLs := make(map[string]bool, len(snapshot.Sources))
	for _, source := range snapshot.Sources {
		if !validHTTPSURL(source.URL) || source.RetrievedAt.IsZero() {
			return EconomicSnapshot{}, errors.New("economic snapshot sources require HTTPS URL and retrieval time")
		}
		sourceURLs[source.URL] = true
	}
	for _, claim := range snapshot.Claims {
		if strings.TrimSpace(claim.Statement) == "" || claim.Classification != "public" || len(claim.SourceURLs) == 0 {
			return EconomicSnapshot{}, errors.New("economic snapshot claims require a public classification and source provenance")
		}
		for _, sourceURL := range claim.SourceURLs {
			if !sourceURLs[sourceURL] {
				return EconomicSnapshot{}, errors.New("economic snapshot claim references an undeclared source")
			}
		}
	}
	snapshot.CreatedAt = time.Now().UTC()
	claims, err := json.Marshal(snapshot.Claims)
	if err != nil {
		return EconomicSnapshot{}, err
	}
	snapshot.SnapshotID = identifier("economic-snapshot", snapshot.AsOf.String(), string(claims), snapshot.CreatedAt.String())
	path := filepath.Join(dataRoot, "economic", "public", "snapshots", snapshot.SnapshotID+".json")
	if err := createImmutableJSON(path, snapshot); err != nil {
		return EconomicSnapshot{}, err
	}
	return snapshot, nil
}

func AttachEconomicSnapshot(dataRoot, workspaceID, snapshotID string) error {
	if !workspaceIDPattern.MatchString(workspaceID) || !safeID(snapshotID) {
		return errors.New("invalid workspace or snapshot ID")
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "economic", "public", "snapshots", snapshotID+".json")); err != nil {
		return err
	}
	if status, err := Inspect(dataRoot, workspaceID); err != nil || !status.Initialized {
		if err != nil {
			return err
		}
		return errors.New("workspace agent is not initialized")
	}
	attachedAt := time.Now().UTC()
	path := filepath.Join(workspaceAgentRoot(dataRoot, workspaceID), "dossier", "economic-snapshot.json")
	if err := writeAtomicJSON(path, economicReference{SnapshotID: snapshotID, AttachedAt: attachedAt}); err != nil {
		return err
	}
	return updateOperationalState(dataRoot, workspaceID, func(state *OperationalState) {
		state.CurrentEconomicSnapshotID = snapshotID
		state.UpdatedAt = attachedAt
	})
}

func validateDraftPlan(plan ResearchPlan) error {
	if !workspaceIDPattern.MatchString(plan.WorkspaceID) || strings.TrimSpace(plan.Purpose) == "" || len(plan.QueryThemes) == 0 || len(plan.Sources) == 0 || plan.ValidUntil.IsZero() || plan.MaxQueries < len(plan.QueryThemes) || plan.MaxQueries > 20 {
		return errors.New("research plan requires workspace, purpose, query themes and approved source domains")
	}
	if !plan.ValidUntil.After(time.Now().UTC()) {
		return errors.New("research plan validity must be in the future")
	}
	for _, theme := range plan.QueryThemes {
		if strings.TrimSpace(theme) == "" {
			return errors.New("research query themes cannot be empty")
		}
	}
	for _, source := range plan.Sources {
		if strings.TrimSpace(source) == "" || strings.ContainsAny(source, "/: ") {
			return errors.New("research sources must be hostname allowlist entries")
		}
	}
	return nil
}

func validateBrief(brief Brief) error {
	if !workspaceIDPattern.MatchString(brief.WorkspaceID) || strings.TrimSpace(brief.ReviewedBy) == "" || strings.TrimSpace(brief.Mandate) == "" || len(brief.Objectives) == 0 || len(brief.Constraints) == 0 {
		return errors.New("brief requires workspace, reviewer, mandate, objectives and constraints")
	}
	if brief.Classification != "public" && brief.Classification != "internal" && brief.Classification != "confidential" {
		return errors.New("brief classification must be public, internal or confidential")
	}
	if len(brief.Bullish) == 0 || len(brief.Bearish) == 0 {
		return errors.New("brief requires at least one bullish and one bearish thesis")
	}
	for _, thesis := range append(append([]Thesis{}, brief.Bullish...), brief.Bearish...) {
		if strings.TrimSpace(thesis.Statement) == "" || len(thesis.Evidence) == 0 || len(thesis.Assumptions) == 0 || len(thesis.CounterEvidence) == 0 || len(thesis.InvalidationSignals) == 0 {
			return errors.New("each thesis requires statement, evidence, assumptions, counter-evidence and invalidation signals")
		}
	}
	return nil
}

func validateEvidence(dataRoot string, plan ResearchPlan, evidence Evidence) error {
	if evidence.WorkspaceID != plan.WorkspaceID || evidence.PlanID != plan.PlanID || evidence.RetrievedAt.IsZero() || evidence.ValidUntil.IsZero() || !evidence.ValidUntil.After(evidence.RetrievedAt) || strings.TrimSpace(evidence.Query) == "" || strings.TrimSpace(evidence.Claim) == "" {
		return errors.New("evidence does not match its approved research plan")
	}
	if time.Now().UTC().After(plan.ValidUntil) {
		return errors.New("approved research plan is expired")
	}
	if time.Now().UTC().After(evidence.ValidUntil) {
		return errors.New("research evidence is expired")
	}
	if !approvedQuery(plan, evidence.Query) {
		return errors.New("evidence query is outside the approved research themes")
	}
	if !queryWasConsumed(dataRoot, evidence.WorkspaceID, evidence.PlanID, evidence.Query) {
		return errors.New("evidence query was not consumed from the approved budget")
	}
	if evidence.Classification != "public" {
		return errors.New("external research evidence must be classified public")
	}
	if evidence.EvidenceStrength != "primary" && evidence.EvidenceStrength != "secondary" {
		return errors.New("evidence strength must be primary or secondary")
	}
	parsed, err := url.Parse(evidence.SourceURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return errors.New("evidence source must be an HTTPS URL")
	}
	host := strings.ToLower(parsed.Hostname())
	for _, allowed := range plan.Sources {
		allowed = strings.ToLower(allowed)
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return nil
		}
	}
	return errors.New("evidence source is outside the approved allowlist")
}

func approvedQuery(plan ResearchPlan, query string) bool {
	for _, approved := range plan.QueryThemes {
		if query == approved {
			return true
		}
	}
	return false
}

func queryWasConsumed(dataRoot, workspaceID, planID, query string) bool {
	root := filepath.Join(workspaceAgentRoot(dataRoot, workspaceID), "research", "executions", planID)
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		file, err := os.Open(filepath.Join(root, entry.Name()))
		if err != nil {
			continue
		}
		var execution QueryExecution
		decodeErr := json.NewDecoder(file).Decode(&execution)
		_ = file.Close()
		if decodeErr == nil && execution.WorkspaceID == workspaceID && execution.PlanID == planID && execution.Query == query {
			return true
		}
	}
	return false
}

func readResearchPlan(dataRoot, workspaceID, planID, state string) (ResearchPlan, error) {
	if !workspaceIDPattern.MatchString(workspaceID) || !safeID(planID) {
		return ResearchPlan{}, errors.New("invalid workspace or plan ID")
	}
	path := researchPlanPath(dataRoot, workspaceID, planID, state)
	file, err := os.Open(path)
	if err != nil {
		return ResearchPlan{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var plan ResearchPlan
	if err := decoder.Decode(&plan); err != nil {
		return ResearchPlan{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ResearchPlan{}, errors.New("research plan contains trailing JSON")
	}
	return plan, nil
}

func researchPlanPath(dataRoot, workspaceID, planID, state string) string {
	return filepath.Join(workspaceAgentRoot(dataRoot, workspaceID), "research", "plans", planID+"."+state+".json")
}

func workspaceAgentRoot(dataRoot, workspaceID string) string {
	return filepath.Join(dataRoot, "workspaces", workspaceID)
}

func updateOperationalState(dataRoot, workspaceID string, mutate func(*OperationalState)) error {
	path := filepath.Join(workspaceAgentRoot(dataRoot, workspaceID), "agent", "state.json")
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var state OperationalState
	decodeErr := decoder.Decode(&state)
	closeErr := file.Close()
	if decodeErr != nil {
		return decodeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if state.SchemaVersion != 1 || state.WorkspaceID != workspaceID {
		return errors.New("workspace operational state does not match workspace")
	}
	mutate(&state)
	return writeAtomicJSON(path, state)
}

func createImmutableJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	encodeErr := json.NewEncoder(file).Encode(value)
	closeErr := file.Close()
	if encodeErr != nil {
		return encodeErr
	}
	return closeErr
}

func writeAtomicJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".bcgos-workspace-agent-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := json.NewEncoder(temporary).Encode(value); err != nil {
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
	return durableReplace(temporaryPath, path)
}

func identifier(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:16])
}

func safeID(value string) bool {
	return workspaceIDPattern.MatchString(value)
}

func validHTTPSURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() != ""
}
