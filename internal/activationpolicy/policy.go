// Package activationpolicy computes deterministic, content-free activation
// plans. It authorizes routes; model planning may only provide non-authoritative
// expert suggestions.
package activationpolicy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

const PolicyVersion = "pxrt-v1"

type Posture string
type ConsequenceLevel string
type ReversibilityLevel string
type Classification string
type KnowledgeNeed string
type Route string
type Owner string
type ExpertKind string
type ExpertLifecycle string

const (
	Direct       Posture = "direct"
	Balanced     Posture = "balanced"
	Deliberative Posture = "deliberative"

	Low    ConsequenceLevel = "low"
	Medium ConsequenceLevel = "medium"
	High   ConsequenceLevel = "high"

	Reversible   ReversibilityLevel = "reversible"
	Limited      ReversibilityLevel = "limited"
	Irreversible ReversibilityLevel = "irreversible"

	Public       Classification = "public"
	Internal     Classification = "internal"
	Confidential Classification = "confidential"
	Restricted   Classification = "restricted"

	None       KnowledgeNeed = "none"
	Functional KnowledgeNeed = "functional"
	Industry   KnowledgeNeed = "industry"
	Both       KnowledgeNeed = "both"

	D0Direct   Route = "D0_DIRECT"
	D1Targeted Route = "D1_TARGETED"
	D2Governed Route = "D2_GOVERNED"
	Blocked    Route = "BLOCKED"

	OwnerAccount Owner = "client_account_agent"
	OwnerCase    Owner = "case_agent"

	ExpertFPA ExpertKind = "FPA"
	ExpertIPA ExpertKind = "IPA"

	Draft     ExpertLifecycle = "draft"
	Published ExpertLifecycle = "published"
	Retired   ExpertLifecycle = "retired"
)

type PlannerProposal struct {
	RequestedExperts []string `json:"requested_expert_ids,omitempty"`
}

type IntentEnvelope struct {
	SchemaVersion    int                `json:"schema_version"`
	EpisodeID        string             `json:"episode_id"`
	Owner            Owner              `json:"owner"`
	Posture          Posture            `json:"posture"`
	Consequence      ConsequenceLevel   `json:"consequence"`
	Reversibility    ReversibilityLevel `json:"reversibility"`
	Sensitivity      Classification     `json:"sensitivity"`
	KnowledgeNeed    KnowledgeNeed      `json:"knowledge_need"`
	Ambiguous        bool               `json:"ambiguous,omitempty"`
	CrossScope       bool               `json:"cross_scope,omitempty"`
	ExternalEffect   bool               `json:"external_effect,omitempty"`
	PrivilegedAction bool               `json:"privileged_action,omitempty"`
	PlannerProposal  PlannerProposal    `json:"planner_proposal,omitempty"`
}

type PXpert struct {
	ID          string          `json:"id"`
	Kind        ExpertKind      `json:"kind"`
	Version     string          `json:"version"`
	CanonSHA256 string          `json:"canon_sha256"`
	Lifecycle   ExpertLifecycle `json:"lifecycle"`
}

type SelectedPXpert struct {
	ID          string     `json:"id"`
	Kind        ExpertKind `json:"kind"`
	Version     string     `json:"version"`
	CanonSHA256 string     `json:"canon_sha256"`
}

type Budget struct {
	MaxCalls       int `json:"max_calls"`
	MaxExperts     int `json:"max_experts"`
	MaxTokenUnits  int `json:"max_token_units"`
	MaxDurationSec int `json:"max_duration_seconds"`
}

type RoutePlan struct {
	SchemaVersion             int              `json:"schema_version"`
	EpisodeID                 string           `json:"episode_id"`
	Owner                     Owner            `json:"owner"`
	Posture                   Posture          `json:"posture"`
	Route                     Route            `json:"route"`
	PolicyVersion             string           `json:"policy_version"`
	Shadow                    bool             `json:"shadow"`
	AuthorityState            string           `json:"authority_state"`
	MayAuthorizeDispatch      bool             `json:"may_authorize_dispatch"`
	RequiresAssurance         bool             `json:"requires_assurance"`
	AssuranceAgentID          string           `json:"assurance_agent_id,omitempty"`
	RequiresHumanConfirmation bool             `json:"requires_human_confirmation"`
	Experts                   []SelectedPXpert `json:"experts"`
	Budget                    Budget           `json:"budget"`
	ReasonCodes               []string         `json:"reason_codes"`
	StopConditions            []string         `json:"stop_conditions"`
	InputSHA256               string           `json:"input_sha256"`
	PlanSHA256                string           `json:"plan_sha256"`
}

var (
	safeID = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)
	semver = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)
)

func DecodeStrict(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("activation input contains multiple JSON values")
		}
		return err
	}
	return nil
}

func Plan(envelope IntentEnvelope, registry []PXpert) (RoutePlan, error) {
	normalized, err := normalizeEnvelope(envelope)
	if err != nil {
		return RoutePlan{}, err
	}
	inputBody, err := json.Marshal(normalized)
	if err != nil {
		return RoutePlan{}, err
	}
	route, reasons := decide(normalized)
	plan := RoutePlan{
		SchemaVersion: 1, EpisodeID: normalized.EpisodeID, Owner: normalized.Owner,
		Posture: normalized.Posture, Route: route,
		PolicyVersion: PolicyVersion, Shadow: true,
		AuthorityState: "caller_asserted_shadow", MayAuthorizeDispatch: false,
		Experts: []SelectedPXpert{}, ReasonCodes: reasons,
		StopConditions: []string{"budget_exhausted", "digest_drift", "missing_receipt", "scope_violation"},
		InputSHA256:    SHA256Hex(inputBody),
	}
	switch route {
	case D0Direct:
		plan.Budget = Budget{MaxCalls: 1, MaxExperts: 0, MaxTokenUnits: 4000, MaxDurationSec: 600}
	case D1Targeted:
		plan.Budget = Budget{MaxCalls: 3, MaxExperts: 1, MaxTokenUnits: 10000, MaxDurationSec: 1200}
	case D2Governed:
		plan.Budget = Budget{MaxCalls: 6, MaxExperts: 2, MaxTokenUnits: 24000, MaxDurationSec: 2700}
	case Blocked:
		plan.Budget = Budget{}
		plan.RequiresHumanConfirmation = true
	default:
		return RoutePlan{}, errors.New("activation policy produced an invalid route")
	}
	if route != Blocked {
		plan.Experts, err = selectExperts(normalized, registry, plan.Budget.MaxExperts)
		if err != nil {
			return RoutePlan{}, err
		}
	}
	if route == D2Governed || (route == D1Targeted && len(plan.Experts) == 0) {
		plan.RequiresAssurance = true
		plan.AssuranceAgentID = "walter"
	}
	plan.PlanSHA256 = PlanDigest(plan)
	return plan, nil
}

func normalizeEnvelope(input IntentEnvelope) (IntentEnvelope, error) {
	if input.SchemaVersion != 1 || !validID(input.EpisodeID) {
		return IntentEnvelope{}, errors.New("activation envelope has an invalid schema version or episode ID")
	}
	if input.Owner != OwnerAccount && input.Owner != OwnerCase {
		return IntentEnvelope{}, errors.New("activation envelope owner is unsupported")
	}
	if input.Posture == "" {
		input.Posture = Balanced
	}
	if input.Posture != Direct && input.Posture != Balanced && input.Posture != Deliberative {
		return IntentEnvelope{}, errors.New("activation posture is unsupported")
	}
	if input.Consequence != Low && input.Consequence != Medium && input.Consequence != High {
		return IntentEnvelope{}, errors.New("activation consequence is unsupported")
	}
	if input.Reversibility != Reversible && input.Reversibility != Limited && input.Reversibility != Irreversible {
		return IntentEnvelope{}, errors.New("activation reversibility is unsupported")
	}
	if input.Sensitivity != Public && input.Sensitivity != Internal && input.Sensitivity != Confidential && input.Sensitivity != Restricted {
		return IntentEnvelope{}, errors.New("activation sensitivity is unsupported")
	}
	if input.KnowledgeNeed != None && input.KnowledgeNeed != Functional && input.KnowledgeNeed != Industry && input.KnowledgeNeed != Both {
		return IntentEnvelope{}, errors.New("activation knowledge need is unsupported")
	}
	for _, id := range input.PlannerProposal.RequestedExperts {
		if !validID(id) {
			return IntentEnvelope{}, errors.New("planner proposed an invalid PXpert ID")
		}
	}
	input.PlannerProposal.RequestedExperts = append([]string(nil), input.PlannerProposal.RequestedExperts...)
	sort.Strings(input.PlannerProposal.RequestedExperts)
	input.PlannerProposal.RequestedExperts = compact(input.PlannerProposal.RequestedExperts)
	return input, nil
}

func decide(input IntentEnvelope) (Route, []string) {
	if input.PrivilegedAction {
		return Blocked, []string{"privileged_action_blocked"}
	}
	if input.Sensitivity == Restricted && input.ExternalEffect {
		return Blocked, []string{"restricted_external_effect_blocked"}
	}
	var reasons []string
	hardD2 := false
	if input.Consequence == High {
		hardD2 = true
		reasons = append(reasons, "high_consequence")
	}
	if input.Reversibility == Irreversible {
		hardD2 = true
		reasons = append(reasons, "irreversible_change")
	}
	if input.CrossScope {
		hardD2 = true
		reasons = append(reasons, "cross_scope")
	}
	if input.ExternalEffect {
		hardD2 = true
		reasons = append(reasons, "external_effect")
	}
	if input.KnowledgeNeed == Both {
		hardD2 = true
		reasons = append(reasons, "dual_practice_advice")
	}
	if hardD2 {
		sort.Strings(reasons)
		return D2Governed, reasons
	}
	needsAdvice := input.KnowledgeNeed == Functional || input.KnowledgeNeed == Industry
	switch input.Posture {
	case Direct:
		if needsAdvice {
			return D1Targeted, []string{"explicit_practice_advice"}
		}
		return D0Direct, []string{"direct_posture_safe_path"}
	case Deliberative:
		if needsAdvice || input.Ambiguous || input.Consequence == Medium || input.Reversibility == Limited {
			return D2Governed, []string{"deliberative_governed_loop"}
		}
		return D0Direct, []string{"deliberative_low_risk_direct"}
	default:
		if needsAdvice {
			return D1Targeted, []string{"balanced_practice_advice"}
		}
		if input.Ambiguous || input.Consequence == Medium || input.Reversibility == Limited {
			return D1Targeted, []string{"balanced_targeted_review"}
		}
		return D0Direct, []string{"balanced_low_risk_direct"}
	}
}

func selectExperts(input IntentEnvelope, registry []PXpert, limit int) ([]SelectedPXpert, error) {
	required := []ExpertKind{}
	switch input.KnowledgeNeed {
	case Functional:
		required = append(required, ExpertFPA)
	case Industry:
		required = append(required, ExpertIPA)
	case Both:
		required = append(required, ExpertFPA, ExpertIPA)
	}
	if len(required) == 0 {
		return []SelectedPXpert{}, nil
	}
	if len(required) > limit {
		return nil, errors.New("activation route expert budget cannot satisfy required knowledge kinds")
	}
	byID := make(map[string]PXpert, len(registry))
	var valid []PXpert
	for _, expert := range registry {
		if !validPXpertShape(expert) {
			return nil, errors.New("PXpert registry contains a malformed entry")
		}
		if _, exists := byID[expert.ID]; exists {
			return nil, errors.New("PXpert registry contains a duplicate immutable ID")
		}
		byID[expert.ID] = expert
		if expert.Lifecycle != Published {
			continue
		}
		valid = append(valid, expert)
	}
	sort.Slice(valid, func(i, j int) bool { return valid[i].ID < valid[j].ID })
	selected := make([]SelectedPXpert, 0, len(required))
	used := map[string]bool{}
	for _, kind := range required {
		var candidate PXpert
		found := false
		for _, id := range input.PlannerProposal.RequestedExperts {
			expert, ok := byID[id]
			if ok && expert.Lifecycle == Published &&
				expert.Kind == kind && !used[expert.ID] {
				candidate, found = expert, true
				break
			}
		}
		if !found {
			for _, expert := range valid {
				if expert.Kind == kind && !used[expert.ID] {
					candidate, found = expert, true
					break
				}
			}
		}
		if !found {
			return nil, fmt.Errorf("no published %s PXpert satisfies the deterministic route", kind)
		}
		used[candidate.ID] = true
		selected = append(selected, SelectedPXpert{
			ID: candidate.ID, Kind: candidate.Kind, Version: candidate.Version,
			CanonSHA256: strings.ToLower(candidate.CanonSHA256),
		})
	}
	return selected, nil
}

func validPXpert(expert PXpert) bool {
	return validPXpertShape(expert) && expert.Lifecycle == Published
}

func IsValidPublishedPXpert(expert PXpert) bool {
	return validPXpert(expert)
}

func validPXpertShape(expert PXpert) bool {
	return validID(expert.ID) && (expert.Kind == ExpertFPA || expert.Kind == ExpertIPA) &&
		semver.MatchString(expert.Version) && validSHA256(expert.CanonSHA256) &&
		(expert.Lifecycle == Draft || expert.Lifecycle == Published || expert.Lifecycle == Retired)
}

func PlanDigest(plan RoutePlan) string {
	plan.PlanSHA256 = ""
	body, err := json.Marshal(plan)
	if err != nil {
		return ""
	}
	return SHA256Hex(body)
}

func SHA256Hex(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func validID(value string) bool {
	return len(value) <= 80 && safeID.MatchString(value)
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func compact(values []string) []string {
	if len(values) < 2 {
		return values
	}
	output := values[:1]
	for _, value := range values[1:] {
		if value != output[len(output)-1] {
			output = append(output, value)
		}
	}
	return output
}
