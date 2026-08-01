// Package maestro contains the closed Maestro routing and quality-loop
// contracts. It is deliberately independent of runtime adapters: adapters
// execute a plan, but never choose authority from caller-provided role text.
package maestro

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
)

type IntentClass string

const (
	IntentDirectAnswer IntentClass = "direct_answer"
	IntentCase         IntentClass = "case_execution"
	IntentAccount      IntentClass = "account_context"
	IntentAdvisory     IntentClass = "practice_advisory"
	IntentReview       IntentClass = "material_review"
	IntentHealth       IntentClass = "health_governance"
	IntentErrand       IntentClass = "bounded_errand"
)

type Sensitivity string

const (
	SensitivityPublic       Sensitivity = "public"
	SensitivityInternal     Sensitivity = "internal"
	SensitivityConfidential Sensitivity = "confidential"
	SensitivityRestricted   Sensitivity = "restricted"
)

type Materiality string

const (
	MaterialityNone   Materiality = "none"
	MaterialityReview Materiality = "review"
)

type HealthIntent string

const (
	HealthNone       HealthIntent = "none"
	HealthSystem     HealthIntent = "system_health"
	HealthGovernance HealthIntent = "governance"
)

var validReviewTriggers = map[string]bool{
	"material_recommendation": true,
	"consequential_tradeoff":  true,
	"external_artifact":       true,
	"reputational_risk":       true,
	"hard_to_reverse":         true,
	"materiality_review":      true,
}

type Action string

const (
	ActionDirect   Action = "direct"
	ActionCase     Action = "open_case"
	ActionAccount  Action = "open_client_account"
	ActionPAExpert Action = "request_pa_expert"
	ActionWalter   Action = "invoke_walter"
	ActionDarwin   Action = "invoke_darwin"
	ActionErrand   Action = "bounded_errand"
)

type CaseEntry string

const (
	CaseEntryAccountFirst CaseEntry = "account_first"
	CaseEntryDirect       CaseEntry = "direct_case"
)

type RegisteredAgent struct {
	ID                  string `json:"id"`
	Role                string `json:"role"`
	ScopeKind           string `json:"scope_kind"`
	ScopeID             string `json:"scope_id"`
	ParentScopeKind     string `json:"parent_scope_kind,omitempty"`
	ParentScopeID       string `json:"parent_scope_id,omitempty"`
	AuthorizationDigest string `json:"authorization_digest"`
	CapabilityDigest    string `json:"capability_digest"`
	StateSnapshotDigest string `json:"state_snapshot_digest"`
	Available           bool   `json:"available"`
}

type Input struct {
	SchemaVersion          int               `json:"schema_version"`
	IntentClass            IntentClass       `json:"intent_class"`
	ScopeKind              string            `json:"scope_kind"`
	ScopeID                string            `json:"scope_id"`
	AccountScopeID         string            `json:"account_scope_id,omitempty"`
	Sensitivity            Sensitivity       `json:"sensitivity"`
	Materiality            Materiality       `json:"materiality"`
	ReviewTrigger          string            `json:"review_trigger"`
	HealthIntent           HealthIntent      `json:"health_governance_intent"`
	RequestedCapability    string            `json:"requested_capability"`
	CallerRole             string            `json:"caller_role,omitempty"`
	ClientImplication      bool              `json:"client_implication"`
	StakeholderImplication bool              `json:"stakeholder_implication"`
	StrategicImplication   bool              `json:"strategic_implication"`
	PromotionImplication   bool              `json:"promotion_implication"`
	CrossCaseContext       bool              `json:"cross_case_context"`
	ExecutionOnly          bool              `json:"execution_only"`
	SimpleReversible       bool              `json:"simple_reversible"`
	HighLeverage           bool              `json:"high_leverage"`
	ConsequentialDecision  bool              `json:"consequential_decision"`
	ExternalArtifact       bool              `json:"external_artifact"`
	ReputationalRisk       bool              `json:"reputational_risk"`
	HardToReverse          bool              `json:"hard_to_reverse"`
	AvailableAgents        []RegisteredAgent `json:"available_registered_agents"`
}

type AgentBinding struct {
	ID                  string `json:"id"`
	Role                string `json:"role"`
	ScopeKind           string `json:"scope_kind"`
	ScopeID             string `json:"scope_id"`
	ParentScopeKind     string `json:"parent_scope_kind,omitempty"`
	ParentScopeID       string `json:"parent_scope_id,omitempty"`
	AuthorizationDigest string `json:"authorization_digest"`
	CapabilityDigest    string `json:"capability_digest"`
	StateSnapshotDigest string `json:"state_snapshot_digest"`
}

type Plan struct {
	SchemaVersion               int            `json:"schema_version"`
	PlannerVersion              string         `json:"planner_version"`
	Action                      Action         `json:"action"`
	ReasonCode                  string         `json:"reason_code"`
	CaseEntry                   CaseEntry      `json:"case_entry,omitempty"`
	SkipPreAccount              bool           `json:"skip_pre_account"`
	SkipReasonCodes             []string       `json:"skip_reason_codes,omitempty"`
	SkipWalter                  bool           `json:"skip_walter"`
	WalterReasonCode            string         `json:"walter_reason_code,omitempty"`
	WalterSkipReasonCode        string         `json:"walter_skip_reason_code,omitempty"`
	WalterSkipEvidence          string         `json:"walter_skip_evidence,omitempty"`
	RequiresAccountFraming      bool           `json:"requires_account_framing"`
	RequiresAccountValidation   bool           `json:"requires_account_validation"`
	RequiresWalter              bool           `json:"requires_walter"`
	ScopeKind                   string         `json:"scope_kind"`
	ScopeID                     string         `json:"scope_id"`
	AccountScopeID              string         `json:"account_scope_id,omitempty"`
	RequestedCapability         string         `json:"requested_capability,omitempty"`
	AccountConsultationRequired bool           `json:"account_consultation_required"`
	Bindings                    []AgentBinding `json:"bindings,omitempty"`
	PlanDigest                  string         `json:"plan_digest"`
}

const PlannerVersion = "maestro-native-v2"

func PlanFor(input Input) (Plan, error) {
	if err := validateInput(input); err != nil {
		return Plan{}, err
	}
	agents := append([]RegisteredAgent(nil), input.AvailableAgents...)
	sort.Slice(agents, func(i, j int) bool { return agents[i].ID < agents[j].ID })
	find := func(role, kind, id string) (RegisteredAgent, bool) {
		for _, agent := range agents {
			if agent.Available && agent.Role == role && agent.ScopeKind == kind && agent.ScopeID == id {
				return agent, true
			}
		}
		return RegisteredAgent{}, false
	}
	plan := Plan{SchemaVersion: 1, PlannerVersion: PlannerVersion, ScopeKind: input.ScopeKind, ScopeID: input.ScopeID, AccountScopeID: input.AccountScopeID, RequestedCapability: input.RequestedCapability}
	var selected []RegisteredAgent
	add := func(agent RegisteredAgent) { selected = append(selected, agent) }
	clientLensSignal := input.ClientImplication || input.StakeholderImplication || input.StrategicImplication || input.PromotionImplication || input.CrossCaseContext
	accountConsultation := !input.ExecutionOnly || clientLensSignal
	walterRequired := input.HighLeverage || input.ConsequentialDecision || input.ExternalArtifact || input.ReputationalRisk || input.HardToReverse || input.Materiality == MaterialityReview || input.ReviewTrigger != ""
	switch input.IntentClass {
	case IntentDirectAnswer:
		plan.Action, plan.ReasonCode = ActionDirect, "direct_closed_intent"
	case IntentAccount:
		plan.Action, plan.ReasonCode = ActionAccount, "account_context_requested"
		account, ok := find("client_account_agent", "account", input.ScopeID)
		if !ok {
			return Plan{}, errors.New("client account routing is unavailable for the active scope")
		}
		add(account)
	case IntentAdvisory:
		plan.Action, plan.ReasonCode = ActionPAExpert, "pa_expert_advisory_requested"
		expert, ok := find("pa_expert", "practice", input.ScopeID)
		if !ok {
			return Plan{}, errors.New("PA Expert routing is unavailable for the requested registered canon")
		}
		add(expert)
	case IntentReview:
		plan.Action, plan.ReasonCode = ActionWalter, "material_review_requested"
		walter, ok := find("reviewer", "review", "review")
		if !ok {
			return Plan{}, errors.New("Walter routing is unavailable")
		}
		add(walter)
	case IntentHealth:
		if input.HealthIntent == HealthNone {
			return Plan{}, errors.New("health routing requires a closed health or governance intent")
		}
		plan.Action, plan.ReasonCode = ActionDarwin, "health_governance_requested"
		darwin, ok := find("governance_analyst", "health", input.ScopeID)
		if !ok {
			return Plan{}, errors.New("Darwin routing is unavailable for the health scope")
		}
		add(darwin)
	case IntentErrand:
		plan.Action, plan.ReasonCode = ActionErrand, "bounded_reversible_errand"
		err, ok := find("errand_helper", "errand", input.ScopeID)
		if !ok {
			return Plan{}, errors.New("bounded errand helper is unavailable")
		}
		add(err)
	case IntentCase:
		caseAgent, ok := find("case_agent", input.ScopeKind, input.ScopeID)
		if !ok {
			return Plan{}, errors.New("Case Agent routing is unavailable for the active scope")
		}
		if !accountConsultation {
			plan.Action, plan.ReasonCode = ActionCase, "direct_case_execution_only"
			plan.CaseEntry, plan.SkipPreAccount = CaseEntryDirect, true
			plan.AccountConsultationRequired = false
			plan.RequiresAccountValidation, plan.RequiresWalter = false, walterRequired
			plan.SkipReasonCodes = []string{"skip_pre_account_execution_only"}
			plan.WalterReasonCode = "walter_required_high_leverage"
			if !walterRequired {
				plan.SkipWalter, plan.WalterReasonCode, plan.WalterSkipReasonCode, plan.WalterSkipEvidence = true, "walter_skipped_low_leverage", "walter_skipped_low_leverage", "execution-only Case; no consequential decision, external artifact, reputational risk, hard-to-reverse action or review trigger"
			}
			add(caseAgent)
			if walterRequired {
				walter, ok := find("reviewer", "review", "review")
				if !ok {
					return Plan{}, errors.New("Walter routing is unavailable for the material Case")
				}
				add(walter)
			}
			break
		}
		accountScopeID, err := accountScopeID(input)
		if err != nil {
			return Plan{}, err
		}
		account, ok := find("client_account_agent", "account", accountScopeID)
		if !ok {
			return Plan{}, errors.New("account-first Case routing requires a registered Client Account Agent")
		}
		if !caseHasAccountBinding(input, accountScopeID) {
			return Plan{}, errors.New("account-first Case routing requires an explicit Case-to-Account binding")
		}
		plan.Action, plan.ReasonCode = ActionAccount, "client_strategic_lens_required"
		plan.CaseEntry = CaseEntryAccountFirst
		plan.AccountConsultationRequired = true
		plan.RequiresAccountFraming, plan.RequiresAccountValidation, plan.RequiresWalter = true, true, walterRequired
		plan.WalterReasonCode = "walter_required_high_leverage"
		add(account)
		add(caseAgent)
		if walterRequired {
			walter, ok := find("reviewer", "review", "review")
			if !ok {
				return Plan{}, errors.New("material Case routing requires a registered Walter reviewer")
			}
			add(walter)
		} else {
			plan.SkipWalter, plan.WalterReasonCode, plan.WalterSkipReasonCode, plan.WalterSkipEvidence = true, "walter_skipped_low_leverage", "walter_skipped_low_leverage", "low-leverage Case; no consequential decision, external artifact, reputational risk, hard-to-reverse action or review trigger"
		}
	default:
		return Plan{}, fmt.Errorf("unknown Maestro intent class %q", input.IntentClass)
	}
	for _, agent := range selected {
		plan.Bindings = append(plan.Bindings, AgentBinding{ID: agent.ID, Role: agent.Role, ScopeKind: agent.ScopeKind, ScopeID: agent.ScopeID, ParentScopeKind: agent.ParentScopeKind, ParentScopeID: agent.ParentScopeID, AuthorizationDigest: agent.AuthorizationDigest, CapabilityDigest: agent.CapabilityDigest, StateSnapshotDigest: agent.StateSnapshotDigest})
	}
	plan.PlanDigest = digestPlan(plan)
	return plan, nil
}

func accountScopeID(input Input) (string, error) {
	if input.ScopeKind == "account" {
		return input.ScopeID, nil
	}
	if input.AccountScopeID == "" {
		return "", errors.New("Case routing requires an explicit account scope binding")
	}
	return input.AccountScopeID, nil
}

func caseHasAccountBinding(input Input, accountScopeID string) bool {
	for _, agent := range input.AvailableAgents {
		if agent.Role == "case_agent" && agent.ScopeKind == "case" && agent.ScopeID == input.ScopeID && agent.ParentScopeKind == "account" && agent.ParentScopeID == accountScopeID {
			return true
		}
	}
	return false
}

func validateInput(input Input) error {
	if input.SchemaVersion != 1 {
		return fmt.Errorf("unsupported Maestro planner input schema %d", input.SchemaVersion)
	}
	if !validScopeKind(input.ScopeKind) || input.ScopeID == "" {
		return errors.New("Maestro requires a closed active scope kind and ID")
	}
	if input.ScopeID != "" && !agentcatalog.ValidAgentID(input.ScopeID) {
		return errors.New("Maestro scope ID is invalid")
	}
	validIntent := map[IntentClass]bool{IntentDirectAnswer: true, IntentCase: true, IntentAccount: true, IntentAdvisory: true, IntentReview: true, IntentHealth: true, IntentErrand: true}
	if !validIntent[input.IntentClass] {
		return fmt.Errorf("unknown Maestro intent class %q", input.IntentClass)
	}
	validSensitivity := map[Sensitivity]bool{SensitivityPublic: true, SensitivityInternal: true, SensitivityConfidential: true, SensitivityRestricted: true}
	if !validSensitivity[input.Sensitivity] {
		return fmt.Errorf("unknown sensitivity %q", input.Sensitivity)
	}
	if input.Materiality != MaterialityNone && input.Materiality != MaterialityReview {
		return fmt.Errorf("unknown materiality %q", input.Materiality)
	}
	if input.ReviewTrigger != "" && !validReviewTriggers[input.ReviewTrigger] {
		return fmt.Errorf("unknown review trigger %q", input.ReviewTrigger)
	}
	if input.HealthIntent != HealthNone && input.HealthIntent != HealthSystem && input.HealthIntent != HealthGovernance {
		return fmt.Errorf("unknown health intent %q", input.HealthIntent)
	}
	if input.RequestedCapability != "" && !agentcatalog.ValidAgentID(input.RequestedCapability) {
		return errors.New("requested capability is not a closed identifier")
	}
	seen := map[string]bool{}
	for _, agent := range input.AvailableAgents {
		if !agentcatalog.ValidAgentID(agent.ID) || seen[agent.ID] || !validRole(agent.Role) || !validScopeKind(agent.ScopeKind) || agent.ScopeID == "" || !agentcatalog.ValidAgentID(agent.ScopeID) || (agent.ParentScopeKind != "" && (agent.ParentScopeID == "" || !validScopeKind(agent.ParentScopeKind) || !agentcatalog.ValidAgentID(agent.ParentScopeID))) || (agent.ParentScopeKind == "" && agent.ParentScopeID != "") || !validDigest(agent.AuthorizationDigest) || !validDigest(agent.CapabilityDigest) || !validDigest(agent.StateSnapshotDigest) {
			return errors.New("available agent registry is incomplete, ambiguous or unbound")
		}
		seen[agent.ID] = true
	}
	return nil
}

func validScopeKind(kind string) bool {
	switch kind {
	case "control", "workspace", "case", "account", "practice", "review", "health", "errand":
		return true
	default:
		return false
	}
}

func validRole(role string) bool {
	switch role {
	case "case_agent", "client_account_agent", "pa_expert", "reviewer", "governance_analyst", "errand_helper":
		return true
	default:
		return false
	}
}

func validDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

func digestPlan(plan Plan) string {
	plan.PlanDigest = ""
	body, _ := json.Marshal(plan)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func (plan Plan) Validate() error {
	if plan.SchemaVersion != 1 || plan.PlannerVersion != PlannerVersion || plan.PlanDigest == "" || plan.PlanDigest != digestPlan(plan) {
		return errors.New("Maestro plan integrity is invalid")
	}
	if err := validateInput(Input{SchemaVersion: 1, IntentClass: IntentCase, ScopeKind: plan.ScopeKind, ScopeID: plan.ScopeID, Sensitivity: SensitivityInternal, Materiality: MaterialityNone, HealthIntent: HealthNone, RequestedCapability: plan.RequestedCapability}); err != nil {
		return err
	}
	if err := validatePlanBindings(plan); err != nil {
		return err
	}
	if plan.CaseEntry == CaseEntryAccountFirst {
		if plan.AccountScopeID == "" || !plan.RequiresAccountFraming || !plan.RequiresAccountValidation {
			return errors.New("account-first plan is missing its account binding")
		}
		accountBound, caseBound := false, false
		for _, binding := range plan.Bindings {
			if binding.Role == "client_account_agent" && binding.ScopeKind == "account" && binding.ScopeID == plan.AccountScopeID {
				accountBound = true
			}
			if binding.Role == "case_agent" && binding.ScopeKind == "case" && binding.ScopeID == plan.ScopeID && binding.ParentScopeKind == "account" && binding.ParentScopeID == plan.AccountScopeID {
				caseBound = true
			}
		}
		if !accountBound || !caseBound {
			return errors.New("account-first plan has no valid Case-to-Account parent binding")
		}
	}
	if plan.CaseEntry == CaseEntryDirect && (!plan.SkipPreAccount || plan.RequiresAccountValidation || len(plan.SkipReasonCodes) != 1 || plan.SkipReasonCodes[0] != "skip_pre_account_execution_only") {
		return errors.New("trivial Case may skip only the pre-brief")
	}
	if plan.CaseEntry == CaseEntryDirect && (!plan.SkipPreAccount || plan.RequiresAccountFraming || plan.RequiresAccountValidation) {
		return errors.New("direct Case must skip only pre-brief framing")
	}
	if plan.CaseEntry == CaseEntryAccountFirst && (plan.SkipPreAccount || !plan.RequiresAccountFraming || !plan.RequiresAccountValidation) {
		return errors.New("account-first Case must include pre-brief framing")
	}
	if plan.CaseEntry != "" && plan.AccountConsultationRequired != plan.RequiresAccountFraming {
		return errors.New("account consultation binding does not match pre-account framing")
	}
	if plan.CaseEntry != "" && len(plan.SkipReasonCodes) > 1 {
		return errors.New("Case plans may contain at most one pre-brief skip reason")
	}
	if plan.CaseEntry != "" && plan.RequiresWalter && (plan.SkipWalter || plan.WalterSkipReasonCode != "" || plan.WalterSkipEvidence != "") {
		return errors.New("Walter-required plans cannot carry a Walter skip")
	}
	if plan.CaseEntry != "" && !plan.RequiresWalter && (!plan.SkipWalter || plan.WalterSkipReasonCode == "" || plan.WalterSkipEvidence == "") {
		return errors.New("Walter skip requires an auditable low-materiality reason and evidence")
	}
	if plan.CaseEntry != "" && plan.WalterReasonCode == "" {
		return errors.New("Case plan must record the Walter leverage decision")
	}
	if plan.Action == ActionCase || plan.CaseEntry != "" {
		if plan.RequiresAccountValidation != plan.RequiresAccountFraming {
			return errors.New("Case account validation must exactly match pre-account framing")
		}
		if bindingID(plan, "case_agent") == "" {
			return errors.New("every Case plan must bind Case")
		}
		if plan.RequiresAccountValidation && bindingID(plan, "client_account_agent") == "" {
			return errors.New("account-assisted Case must bind Client Account framing and validation")
		}
		if plan.RequiresWalter != (bindingID(plan, "reviewer") != "") {
			return errors.New("Walter binding does not match the resolved materiality decision")
		}
	}
	if len(plan.Bindings) == 0 && plan.Action != ActionDirect {
		return errors.New("non-direct plan has no exact agent binding")
	}
	return nil
}

func validatePlanBindings(plan Plan) error {
	seenIDs := make(map[string]bool, len(plan.Bindings))
	seenRoles := make(map[string]bool, len(plan.Bindings))
	for _, binding := range plan.Bindings {
		if !agentcatalog.ValidAgentID(binding.ID) || seenIDs[binding.ID] || !validRole(binding.Role) || seenRoles[binding.Role] || !validScopeKind(binding.ScopeKind) || !agentcatalog.ValidAgentID(binding.ScopeID) || !validDigest(binding.AuthorizationDigest) || !validDigest(binding.CapabilityDigest) || !validDigest(binding.StateSnapshotDigest) {
			return errors.New("Maestro plan contains an invalid, duplicate or unbound agent binding")
		}
		if (binding.ParentScopeKind == "") != (binding.ParentScopeID == "") || (binding.ParentScopeKind != "" && (!validScopeKind(binding.ParentScopeKind) || !agentcatalog.ValidAgentID(binding.ParentScopeID))) {
			return errors.New("Maestro plan binding parent scope is incomplete or invalid")
		}
		seenIDs[binding.ID] = true
		seenRoles[binding.Role] = true
	}
	allowed := map[string]bool{}
	if plan.CaseEntry != "" {
		allowed["case_agent"] = true
		if plan.RequiresAccountFraming {
			allowed["client_account_agent"] = true
		}
		if plan.RequiresWalter {
			allowed["reviewer"] = true
		}
	} else {
		switch plan.Action {
		case ActionDirect:
		case ActionCase:
			allowed["case_agent"] = true
			if plan.RequiresWalter {
				allowed["reviewer"] = true
			}
		case ActionAccount:
			allowed["client_account_agent"] = true
		case ActionPAExpert:
			allowed["pa_expert"] = true
		case ActionWalter:
			allowed["reviewer"] = true
		case ActionDarwin:
			allowed["governance_analyst"] = true
		case ActionErrand:
			allowed["errand_helper"] = true
		}
	}
	for _, binding := range plan.Bindings {
		if !allowed[binding.Role] {
			return errors.New("Maestro plan contains an unreferenced agent binding")
		}
	}
	if plan.CaseEntry != "" {
		var caseBinding, accountBinding, walterBinding *AgentBinding
		for index := range plan.Bindings {
			binding := &plan.Bindings[index]
			switch binding.Role {
			case "case_agent":
				caseBinding = binding
			case "client_account_agent":
				accountBinding = binding
			case "reviewer":
				walterBinding = binding
			}
		}
		if caseBinding == nil || caseBinding.ScopeKind != plan.ScopeKind || caseBinding.ScopeID != plan.ScopeID {
			return errors.New("Case route binding does not match the active Case scope")
		}
		if plan.CaseEntry == CaseEntryAccountFirst {
			if accountBinding == nil || accountBinding.ScopeKind != "account" || accountBinding.ScopeID != plan.AccountScopeID || caseBinding.ParentScopeKind != "account" || caseBinding.ParentScopeID != plan.AccountScopeID {
				return errors.New("account-assisted Case route bindings are not parent-bound")
			}
		} else if plan.CaseEntry == CaseEntryDirect && plan.AccountScopeID != "" && (caseBinding.ParentScopeKind != "account" || caseBinding.ParentScopeID != plan.AccountScopeID) {
			return errors.New("direct Case binding does not match its declared account scope")
		}
		if plan.RequiresWalter && (walterBinding == nil || walterBinding.ScopeKind != "review" || walterBinding.ScopeID != "review") {
			return errors.New("Walter route binding does not match the review scope")
		}
	}
	if plan.Action == ActionDirect && len(plan.Bindings) != 0 {
		return errors.New("direct Maestro plan cannot contain agent bindings")
	}
	return nil
}

type Stage string

const (
	StageAccountFraming  Stage = "account_framing"
	StageCaseExecution   Stage = "case_execution"
	StageAccountValidate Stage = "account_validation"
	StageWalterReview    Stage = "walter_review"
	StageFinal           Stage = "final"
	StageFailed          Stage = "failed_closed"
)

type LoopPolicy struct {
	MaxAccountCycles int
	MaxWalterCycles  int
	MaxCaseAttempts  int
}

var DefaultLoopPolicy = LoopPolicy{MaxAccountCycles: 2, MaxWalterCycles: 2, MaxCaseAttempts: 3}

type ChainState struct {
	PlanDigest            string    `json:"plan_digest"`
	Stage                 Stage     `json:"stage"`
	ActiveAgentID         string    `json:"active_agent_id,omitempty"`
	AccountCycles         int       `json:"account_cycles"`
	WalterCycles          int       `json:"walter_cycles"`
	CaseAttempts          int       `json:"case_attempts"`
	ContentDigest         string    `json:"content_digest,omitempty"`
	AccountApprovalDigest string    `json:"account_approval_digest,omitempty"`
	WalterApprovalDigest  string    `json:"walter_approval_digest,omitempty"`
	Receipts              []Receipt `json:"receipts"`
}

type Receipt struct {
	Sequence                    int    `json:"sequence"`
	Stage                       Stage  `json:"stage"`
	AgentID                     string `json:"agent_id"`
	Decision                    string `json:"decision"`
	ContentDigest               string `json:"content_digest,omitempty"`
	ReasonCode                  string `json:"reason_code,omitempty"`
	AccountConsultationRequired bool   `json:"account_consultation_required"`
	WalterRequired              bool   `json:"walter_required"`
	WalterSkipped               bool   `json:"walter_skipped"`
	PlanDigest                  string `json:"plan_digest"`
	SelfSnapshotVersion         string `json:"self_snapshot_version,omitempty"`
	SelfSnapshotDigest          string `json:"self_snapshot_digest,omitempty"`
	PromptDigest                string `json:"prompt_digest,omitempty"`
	OutputDigest                string `json:"output_digest,omitempty"`
	IntentVerdict               string `json:"intent_verdict,omitempty"`
}

type Event struct {
	AgentID       string
	Decision      string
	ContentDigest string
	ReasonCode    string
	// Interaction is evaluated on every supplied loop event. It is not
	// persisted by Advance; only an explicit Maestro capture may append a
	// material, owner-attested observation to the local observation log.
	Interaction   *ownerctx.ObservationInput
	IntentReceipt *IntentReviewReceipt
}

func NewChain(plan Plan, policy LoopPolicy) (ChainState, error) {
	if err := plan.Validate(); err != nil {
		return ChainState{}, err
	}
	if policy.MaxAccountCycles < 1 || policy.MaxWalterCycles < 1 || policy.MaxCaseAttempts < 1 {
		return ChainState{}, errors.New("quality-loop budgets must be positive")
	}
	state := ChainState{PlanDigest: plan.PlanDigest, Receipts: []Receipt{}}
	if plan.CaseEntry == CaseEntryDirect {
		state.Stage, state.ActiveAgentID = StageCaseExecution, bindingID(plan, "case_agent")
		return state, nil
	}
	if plan.RequiresAccountFraming {
		state.Stage, state.ActiveAgentID = StageAccountFraming, bindingID(plan, "client_account_agent")
		return state, nil
	}
	state.Stage, state.ActiveAgentID = StageCaseExecution, bindingID(plan, "case_agent")
	return state, nil
}

func (state ChainState) Advance(plan Plan, policy LoopPolicy, actor string, event Event) (ChainState, Receipt, error) {
	if actor != "maestro" {
		return state, Receipt{}, errors.New("only Maestro may mediate quality-loop transitions")
	}
	if state.PlanDigest != plan.PlanDigest || plan.Validate() != nil {
		return state, Receipt{}, errors.New("quality-loop plan is stale or invalid")
	}
	if state.Stage == StageFinal || state.Stage == StageFailed {
		return state, Receipt{}, errors.New("quality-loop is already terminal")
	}
	if event.AgentID != state.ActiveAgentID {
		return state, Receipt{}, errors.New("event actor is not the active spoke")
	}
	if strings.TrimSpace(event.Decision) == "" {
		return state, Receipt{}, errors.New("quality-loop decision is required")
	}
	if state.Stage == StageCaseExecution {
		state.CaseAttempts++
		if state.CaseAttempts > policy.MaxCaseAttempts {
			return failState(state, "case_attempt_budget_exhausted")
		}
	}
	receipt := Receipt{
		Sequence: len(state.Receipts) + 1, Stage: state.Stage, AgentID: event.AgentID,
		Decision: event.Decision, ContentDigest: event.ContentDigest, ReasonCode: event.ReasonCode,
		AccountConsultationRequired: plan.AccountConsultationRequired,
		WalterRequired:              plan.RequiresWalter, WalterSkipped: plan.SkipWalter,
		PlanDigest: state.PlanDigest,
	}
	if event.IntentReceipt != nil {
		receipt.SelfSnapshotVersion = event.IntentReceipt.SelfSnapshotVersion
		receipt.SelfSnapshotDigest = event.IntentReceipt.SelfSnapshotDigest
		receipt.PromptDigest = event.IntentReceipt.PromptDigest
		receipt.OutputDigest = event.IntentReceipt.OutputDigest
		receipt.IntentVerdict = string(event.IntentReceipt.Verdict)
	}
	if event.Interaction != nil {
		if _, err := ownerctx.EvaluateInteraction(*event.Interaction); err != nil {
			return state, Receipt{}, err
		}
	}
	switch state.Stage {
	case StageAccountFraming:
		if event.Decision != "approve" && event.Decision != "refine" {
			return state, Receipt{}, errors.New("account framing decision must be approve or refine")
		}
		state.Stage, state.ActiveAgentID = StageCaseExecution, bindingID(plan, "case_agent")
	case StageCaseExecution:
		if event.Decision != "return" || !validDigest(event.ContentDigest) {
			return state, Receipt{}, errors.New("Case must return a content digest")
		}
		state.ContentDigest, state.AccountApprovalDigest, state.WalterApprovalDigest = event.ContentDigest, "", ""
		if plan.RequiresAccountValidation {
			state.Stage, state.ActiveAgentID = StageAccountValidate, bindingID(plan, "client_account_agent")
			break
		}
		if plan.RequiresWalter {
			state.Stage, state.ActiveAgentID = StageWalterReview, bindingID(plan, "reviewer")
			break
		}
		state.Stage, state.ActiveAgentID = StageFinal, ""
		break
	case StageAccountValidate:
		if event.Decision != "approve" && event.Decision != "refine" {
			return state, Receipt{}, errors.New("account validation decision must be approve or refine")
		}
		if event.Decision == "approve" {
			if event.ContentDigest != state.ContentDigest {
				return state, Receipt{}, errors.New("account approval digest does not match Case content")
			}
			state.AccountApprovalDigest = event.ContentDigest
			if plan.RequiresWalter {
				state.Stage, state.ActiveAgentID = StageWalterReview, bindingID(plan, "reviewer")
			} else {
				state.Stage, state.ActiveAgentID = StageFinal, ""
			}
			break
		}
		state.AccountCycles++
		if state.AccountCycles > policy.MaxAccountCycles {
			return failState(state, "account_cycle_budget_exhausted")
		}
		state.ContentDigest, state.AccountApprovalDigest, state.WalterApprovalDigest = "", "", ""
		state.Stage, state.ActiveAgentID = StageCaseExecution, bindingID(plan, "case_agent")
	case StageWalterReview:
		if event.Decision != "approve" && event.Decision != "refine" {
			return state, Receipt{}, errors.New("Walter decision must be approve or refine")
		}
		if event.ContentDigest != state.ContentDigest {
			return state, Receipt{}, errors.New("Walter received a stale content digest")
		}
		if plan.RequiresAccountValidation && state.AccountApprovalDigest != state.ContentDigest {
			return state, Receipt{}, errors.New("Walter received content without current Client Account validation")
		}
		if event.Decision == "approve" {
			state.WalterApprovalDigest = event.ContentDigest
			state.Stage, state.ActiveAgentID = StageFinal, ""
			break
		}
		state.WalterCycles++
		if state.WalterCycles > policy.MaxWalterCycles {
			return failState(state, "walter_cycle_budget_exhausted")
		}
		state.ContentDigest, state.AccountApprovalDigest, state.WalterApprovalDigest = "", "", ""
		state.Stage, state.ActiveAgentID = StageCaseExecution, bindingID(plan, "case_agent")
	default:
		return state, Receipt{}, errors.New("quality-loop stage is not executable")
	}
	state.Receipts = append(state.Receipts, receipt)
	return state, receipt, nil
}

func failState(state ChainState, reason string) (ChainState, Receipt, error) {
	state.Stage, state.ActiveAgentID = StageFailed, ""
	receipt := Receipt{Sequence: len(state.Receipts) + 1, Stage: state.Stage, Decision: "failed_closed", ReasonCode: reason, PlanDigest: state.PlanDigest}
	state.Receipts = append(state.Receipts, receipt)
	return state, receipt, errors.New(reason)
}

func bindingID(plan Plan, role string) string {
	for _, binding := range plan.Bindings {
		if binding.Role == role {
			return binding.ID
		}
	}
	return ""
}
