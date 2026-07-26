package workspaceagent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// PlanAction is one accountable, observable action derived from a reviewed brief.
type PlanAction struct {
	Outcome             string `json:"outcome"`
	Owner               string `json:"owner"`
	CompletionCriterion string `json:"completion_criterion"`
}
type FirstValueSubmission struct {
	Brief         Brief        `json:"brief"`
	Plan          []PlanAction `json:"plan"`
	ArtifactTitle string       `json:"artifact_title"`
	NextStep      string       `json:"next_step"`
	NextOwner     string       `json:"next_owner"`
}
type FirstValueMetrics struct {
	StartedAt               time.Time `json:"started_at"`
	BriefReadyAt            time.Time `json:"brief_ready_at"`
	ArtifactReadyAt         time.Time `json:"artifact_ready_at"`
	TimeToFirstValueSeconds int64     `json:"time_to_first_value_seconds"`
	ManualInterventions     int       `json:"manual_interventions"`
}
type FirstValueArtifact struct {
	ArtifactID string `json:"artifact_id"`
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
}
type FirstValueHandoff struct {
	HandoffID         string    `json:"handoff_id"`
	RunID             string    `json:"run_id"`
	BriefID           string    `json:"brief_id"`
	PlanID            string    `json:"plan_id"`
	ArtifactID        string    `json:"artifact_id"`
	NextStep          string    `json:"next_step"`
	NextOwner         string    `json:"next_owner"`
	OpenQuestionCount int       `json:"open_question_count"`
	CreatedAt         time.Time `json:"created_at"`
}
type FirstValueReceipt struct {
	RunID    string             `json:"run_id"`
	Status   string             `json:"status"`
	Brief    Brief              `json:"brief,omitempty"`
	PlanID   string             `json:"plan_id,omitempty"`
	Artifact FirstValueArtifact `json:"artifact,omitempty"`
	Handoff  FirstValueHandoff  `json:"handoff,omitempty"`
	Metrics  FirstValueMetrics  `json:"metrics"`
}
type FirstValueState struct {
	Available bool              `json:"available"`
	RunID     string            `json:"run_id,omitempty"`
	Status    string            `json:"status,omitempty"`
	Handoff   FirstValueHandoff `json:"handoff,omitempty"`
	Metrics   FirstValueMetrics `json:"metrics,omitempty"`
}
type firstValueRun struct {
	RunID               string    `json:"run_id"`
	WorkspaceID         string    `json:"workspace_id"`
	Status              string    `json:"status"`
	StartedAt           time.Time `json:"started_at"`
	BriefReadyAt        time.Time `json:"brief_ready_at,omitempty"`
	ArtifactReadyAt     time.Time `json:"artifact_ready_at,omitempty"`
	ManualInterventions []string  `json:"manual_interventions,omitempty"`
	BriefID             string    `json:"brief_id,omitempty"`
	PlanID              string    `json:"plan_id,omitempty"`
	ArtifactID          string    `json:"artifact_id,omitempty"`
}

func StartFirstValue(root, workspaceID string) (FirstValueReceipt, error) {
	status, err := Inspect(root, workspaceID)
	if err != nil {
		return FirstValueReceipt{}, err
	}
	if !status.Initialized {
		return FirstValueReceipt{}, errors.New("workspace agent is not initialized")
	}
	runs, err := firstValueRuns(root, workspaceID)
	if err != nil {
		return FirstValueReceipt{}, err
	}
	for _, prior := range runs {
		if prior.Status == "started" {
			return FirstValueReceipt{}, errors.New("a first-value run is already active")
		}
	}
	now := time.Now().UTC()
	run := firstValueRun{RunID: identifier("first-value", workspaceID, now.String()), WorkspaceID: workspaceID, Status: "started", StartedAt: now}
	if err := createImmutableJSON(firstValueRunPath(root, workspaceID, run.RunID), run); err != nil {
		return FirstValueReceipt{}, err
	}
	return FirstValueReceipt{RunID: run.RunID, Status: run.Status, Metrics: metrics(run)}, nil
}

func RecordFirstValueIntervention(root, workspaceID, runID, kind string) error {
	if kind != "brief_correction" && kind != "plan_correction" && kind != "artifact_revision" {
		return errors.New("manual intervention kind is not recognized")
	}
	run, err := readFirstValueRun(root, workspaceID, runID)
	if err != nil {
		return err
	}
	if run.Status != "started" {
		return errors.New("first-value run is no longer open")
	}
	run.ManualInterventions = append(run.ManualInterventions, kind)
	return writeAtomicJSON(firstValueRunPath(root, workspaceID, runID), run)
}

func CompleteFirstValue(root, workspaceID, runID, deliverables string, submission FirstValueSubmission) (FirstValueReceipt, error) {
	run, err := readFirstValueRun(root, workspaceID, runID)
	if err != nil {
		return FirstValueReceipt{}, err
	}
	if run.Status != "started" {
		return FirstValueReceipt{}, errors.New("first-value run is no longer open")
	}
	if err := validateFirstValue(submission); err != nil {
		return FirstValueReceipt{}, err
	}
	submission.Brief.WorkspaceID, submission.Brief.BriefID, submission.Brief.CreatedAt = workspaceID, "", time.Time{}
	brief, err := SaveBrief(root, submission.Brief)
	if err != nil {
		return FirstValueReceipt{}, err
	}
	now := time.Now().UTC()
	planID := identifier("first-value-plan", workspaceID, brief.BriefID, now.String())
	plan := struct {
		PlanID      string       `json:"plan_id"`
		WorkspaceID string       `json:"workspace_id"`
		BriefID     string       `json:"brief_id"`
		Actions     []PlanAction `json:"actions"`
		CreatedAt   time.Time    `json:"created_at"`
	}{planID, workspaceID, brief.BriefID, submission.Plan, now}
	if err := createImmutableJSON(filepath.Join(workspaceAgentRoot(root, workspaceID), "dossier", "plans", planID+".json"), plan); err != nil {
		return FirstValueReceipt{}, err
	}
	artifactID := identifier("decision-brief", workspaceID, brief.BriefID, planID)
	artifactPath := filepath.Join(deliverables, artifactID+".md")
	body := renderDecisionBrief(submission.ArtifactTitle, brief, submission.Plan, submission.NextStep, submission.NextOwner)
	if err := createImmutableText(artifactPath, body); err != nil {
		return FirstValueReceipt{}, err
	}
	digest := sha256.Sum256([]byte(body))
	artifact := FirstValueArtifact{ArtifactID: artifactID, Path: artifactPath, SHA256: hex.EncodeToString(digest[:])}
	if err := createImmutableJSON(filepath.Join(workspaceAgentRoot(root, workspaceID), "dossier", "artifacts", artifactID+".json"), artifact); err != nil {
		return FirstValueReceipt{}, err
	}
	handoff := FirstValueHandoff{HandoffID: identifier("first-value-handoff", runID, artifactID), RunID: runID, BriefID: brief.BriefID, PlanID: planID, ArtifactID: artifactID, NextStep: submission.NextStep, NextOwner: submission.NextOwner, OpenQuestionCount: len(brief.OpenQuestions), CreatedAt: now}
	if err := writeAtomicJSON(filepath.Join(workspaceAgentRoot(root, workspaceID), "agent", "first-value-handoff.json"), handoff); err != nil {
		return FirstValueReceipt{}, err
	}
	run.Status, run.BriefReadyAt, run.ArtifactReadyAt, run.BriefID, run.PlanID, run.ArtifactID = "complete", now, now, brief.BriefID, planID, artifactID
	if err := writeAtomicJSON(firstValueRunPath(root, workspaceID, runID), run); err != nil {
		return FirstValueReceipt{}, err
	}
	if err := updateOperationalState(root, workspaceID, func(state *OperationalState) {
		state.CurrentPlanID, state.CurrentArtifactID, state.CurrentHandoffID, state.CurrentNextStep, state.UpdatedAt = planID, artifactID, handoff.HandoffID, submission.NextStep, now
	}); err != nil {
		return FirstValueReceipt{}, err
	}
	return FirstValueReceipt{RunID: runID, Status: run.Status, Brief: brief, PlanID: planID, Artifact: artifact, Handoff: handoff, Metrics: metrics(run)}, nil
}

func FirstValueStatus(root, workspaceID string) (FirstValueState, error) {
	runs, err := firstValueRuns(root, workspaceID)
	if err != nil {
		return FirstValueState{}, err
	}
	if len(runs) == 0 {
		return FirstValueState{}, nil
	}
	run := runs[len(runs)-1]
	result := FirstValueState{Available: true, RunID: run.RunID, Status: run.Status, Metrics: metrics(run)}
	handoffPath := filepath.Join(workspaceAgentRoot(root, workspaceID), "agent", "first-value-handoff.json")
	if err := readJSON(handoffPath, &result.Handoff); err != nil && !errors.Is(err, os.ErrNotExist) {
		return FirstValueState{}, err
	}
	if result.Handoff.RunID != run.RunID {
		result.Handoff = FirstValueHandoff{}
	}
	if result.Handoff.ArtifactID != "" {
		var artifact FirstValueArtifact
		if err := readJSON(filepath.Join(workspaceAgentRoot(root, workspaceID), "dossier", "artifacts", result.Handoff.ArtifactID+".json"), &artifact); err != nil {
			return FirstValueState{}, err
		}
		bytes, err := os.ReadFile(artifact.Path)
		if err != nil {
			return FirstValueState{}, err
		}
		digest := sha256.Sum256(bytes)
		if hex.EncodeToString(digest[:]) != artifact.SHA256 {
			return FirstValueState{}, errors.New("governed first-value artifact no longer matches its receipt")
		}
	}
	return result, nil
}

func validateFirstValue(s FirstValueSubmission) error {
	b := s.Brief
	if strings.TrimSpace(b.Decision) == "" || strings.TrimSpace(b.TimeHorizon) == "" || len(b.Materials) == 0 || len(b.SuccessSignals) == 0 || len(b.OpenQuestions) == 0 || len(b.Stakeholders) == 0 || strings.TrimSpace(s.ArtifactTitle) == "" || strings.TrimSpace(s.NextStep) == "" || strings.TrimSpace(s.NextOwner) == "" {
		return errors.New("first-value brief is incomplete")
	}
	b.WorkspaceID = "first-value"
	if err := validateBrief(b); err != nil {
		return fmt.Errorf("first-value brief: %w", err)
	}
	if len(s.Plan) < 1 || len(s.Plan) > 3 {
		return errors.New("first-value plan requires one to three actions")
	}
	for _, action := range s.Plan {
		if strings.TrimSpace(action.Outcome) == "" || strings.TrimSpace(action.Owner) == "" || strings.TrimSpace(action.CompletionCriterion) == "" {
			return errors.New("each first-value plan action requires outcome, owner and completion criterion")
		}
	}
	return nil
}
func firstValueRunPath(root, workspaceID, runID string) string {
	return filepath.Join(workspaceAgentRoot(root, workspaceID), "first-value", "runs", runID+".json")
}
func readFirstValueRun(root, workspaceID, runID string) (firstValueRun, error) {
	if !workspaceIDPattern.MatchString(workspaceID) || !safeID(runID) {
		return firstValueRun{}, errors.New("invalid workspace or first-value run")
	}
	var run firstValueRun
	if err := readJSON(firstValueRunPath(root, workspaceID, runID), &run); err != nil {
		return firstValueRun{}, err
	}
	if run.RunID != runID || run.WorkspaceID != workspaceID || run.StartedAt.IsZero() {
		return firstValueRun{}, errors.New("first-value run does not match workspace")
	}
	return run, nil
}
func firstValueRuns(root, workspaceID string) ([]firstValueRun, error) {
	entries, err := os.ReadDir(filepath.Join(workspaceAgentRoot(root, workspaceID), "first-value", "runs"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	runs := []firstValueRun{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		run, err := readFirstValueRun(root, workspaceID, strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].StartedAt.Before(runs[j].StartedAt) })
	return runs, nil
}
func metrics(run firstValueRun) FirstValueMetrics {
	value := FirstValueMetrics{StartedAt: run.StartedAt, BriefReadyAt: run.BriefReadyAt, ArtifactReadyAt: run.ArtifactReadyAt, ManualInterventions: len(run.ManualInterventions)}
	if !run.ArtifactReadyAt.IsZero() {
		value.TimeToFirstValueSeconds = int64(run.ArtifactReadyAt.Sub(run.StartedAt).Seconds())
	}
	return value
}
func createImmutableText(path, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.WriteString(body)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}
func readJSON(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("JSON contains trailing data")
	}
	return nil
}
func renderDecisionBrief(title string, b Brief, plan []PlanAction, nextStep, nextOwner string) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# %s\n\n**Classificação:** %s  \n**Horizonte:** %s\n\n## Mandato\n\n%s\n\n## Decisão a apoiar\n\n%s\n\n", title, b.Classification, b.TimeHorizon, b.Mandate, b.Decision)
	section := func(name string, values []string) {
		fmt.Fprintf(&out, "## %s\n\n", name)
		for _, v := range values {
			fmt.Fprintf(&out, "- %s\n", v)
		}
		out.WriteString("\n")
	}
	section("Objetivos", b.Objectives)
	section("Sinais de sucesso", b.SuccessSignals)
	out.WriteString("## Hipóteses a testar\n\n### Upside\n\n")
	for _, t := range b.Bullish {
		fmt.Fprintf(&out, "- **%s** — evidência: %s; pressupostos: %s; contraevidência: %s; sinal de invalidação: %s\n", t.Statement, strings.Join(t.Evidence, "; "), strings.Join(t.Assumptions, "; "), strings.Join(t.CounterEvidence, "; "), strings.Join(t.InvalidationSignals, "; "))
	}
	out.WriteString("\n### Downside\n\n")
	for _, t := range b.Bearish {
		fmt.Fprintf(&out, "- **%s** — evidência: %s; pressupostos: %s; contraevidência: %s; sinal de invalidação: %s\n", t.Statement, strings.Join(t.Evidence, "; "), strings.Join(t.Assumptions, "; "), strings.Join(t.CounterEvidence, "; "), strings.Join(t.InvalidationSignals, "; "))
	}
	out.WriteString("\n## Próximos passos\n\n")
	for i, a := range plan {
		fmt.Fprintf(&out, "%d. **%s** — responsável: %s; concluído quando: %s\n", i+1, a.Outcome, a.Owner, a.CompletionCriterion)
	}
	section("Perguntas em aberto", b.OpenQuestions)
	fmt.Fprintf(&out, "## Handoff\n\nPróximo passo: %s  \nResponsável: %s\n", nextStep, nextOwner)
	return out.String()
}
