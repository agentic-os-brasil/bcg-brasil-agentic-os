// Package sessionctx builds the bounded, runtime-neutral Session Context
// Packet. It returns pointers and availability only; adapters decide whether
// an authorized source may be read later.
package sessionctx

import (
	"errors"
	"sort"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/execution"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/profile"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

const skillsCatalogPointer = "bundles/base/skills/catalog.json"
const agentsCatalogPointer = "bundles/base/agents/catalog.json"

// sessionSafeFacets is intentionally an allowlist rather than an inference
// from a mutable local registry. Adding a facet to session context requires a
// reviewed change here and contract tests, even when its registry reader list
// says "session".
var sessionSafeFacets = map[string]struct{}{
	"professional-role":   {},
	"communication-style": {},
	"voice":               {},
	"preferences":         {},
	"decision-rules":      {},
	"working-boundaries":  {},
}

type Sources struct {
	Profile   profile.State
	Workspace workspace.Inspection
	Owner     ownerctx.Status
	Atlas     atlas.Status
	Execution execution.ActivePointer
}

type Pointer struct {
	Path      string `json:"path,omitempty"`
	Available bool   `json:"available"`
	State     string `json:"state"`
}

type InteractionProfile struct {
	ID      string `json:"id"`
	Source  string `json:"source"`
	Warning string `json:"warning,omitempty"`
}

type Workspace struct {
	ID            string `json:"id,omitempty"`
	State         string `json:"state"`
	BrainReadable bool   `json:"brain_readable"`
}

type Owner struct {
	Initialized    bool               `json:"initialized"`
	Facets         map[string]Pointer `json:"facets"`
	OperatingState Pointer            `json:"operating_state"`
	Tasks          Pointer            `json:"tasks"`
	Onboarding     Onboarding         `json:"onboarding"`
	OpenTasks      OpenTasks          `json:"open_tasks"`
}

type Onboarding struct {
	State        string `json:"state"`
	NextQuestion string `json:"next_question,omitempty"`
}

type OpenTasks struct {
	State string `json:"state"`
	Count int    `json:"count"`
}

type Atlas struct {
	Managed   Pointer `json:"managed"`
	Owner     Pointer `json:"owner"`
	Workspace Pointer `json:"workspace"`
}

type Skills struct {
	CatalogPointer string           `json:"catalog_pointer"`
	State          string           `json:"state"`
	Selected       []SkillSelection `json:"selected,omitempty"`
}

type SkillSelection struct {
	ID      string `json:"id"`
	Reason  string `json:"reason"`
	Pointer string `json:"pointer"`
}

type ActionConfirmation struct {
	State string `json:"state"`
}

type Agents struct {
	CatalogPointer   string `json:"catalog_pointer"`
	Hub              string `json:"hub"`
	DefinitionsState string `json:"definitions_state"`
	RuntimeState     string `json:"runtime_state"`
	Message          string `json:"message"`
}

type Memory struct {
	State   string `json:"state"`
	Message string `json:"message"`
}

type ExecutionContext struct {
	Active Pointer `json:"active"`
}

type Omission struct {
	Source string `json:"source"`
	Reason string `json:"reason"`
}

type Packet struct {
	SchemaVersion      int                 `json:"schema_version"`
	State              string              `json:"state"`
	InteractionProfile InteractionProfile  `json:"interaction_profile"`
	Workspace          Workspace           `json:"workspace"`
	Owner              Owner               `json:"owner"`
	Atlas              Atlas               `json:"atlas"`
	Execution          ExecutionContext    `json:"execution"`
	Skills             Skills              `json:"skills"`
	Agents             Agents              `json:"agents"`
	Memory             Memory              `json:"memory"`
	ActionConfirmation *ActionConfirmation `json:"action_confirmation,omitempty"`
	Omissions          []Omission          `json:"omissions"`
}

func Build(sources Sources) Packet {
	onboarding := sources.Owner.Onboarding
	if onboarding.State == "" {
		interview := ownerctx.ColdStartInterview()
		onboarding = ownerctx.OnboardingStatus{State: "required", Remaining: []string{"professional-role"}}
		if len(interview.Steps) > 0 {
			onboarding.NextQuestion = interview.Steps[0]
		}
	}
	openTasks := sources.Owner.OpenTasks
	if openTasks.State == "" {
		openTasks.State = "unavailable"
	}
	packet := Packet{
		SchemaVersion: 1,
		State:         "ready",
		InteractionProfile: InteractionProfile{
			ID: sources.Profile.Profile, Source: sources.Profile.Source, Warning: sources.Profile.Warning,
		},
		Workspace: Workspace{ID: sources.Workspace.WorkspaceID, State: sources.Workspace.State, BrainReadable: sources.Workspace.BrainReadable},
		Owner: Owner{
			Initialized:    sources.Owner.Initialized,
			Facets:         sessionFacets(sources.Owner.Facets),
			OperatingState: pointer(sources.Owner.OperatingState),
			Tasks:          pointer(sources.Owner.Tasks),
			Onboarding:     Onboarding{State: onboarding.State, NextQuestion: onboarding.NextQuestion.Question},
			OpenTasks:      OpenTasks{State: openTasks.State, Count: openTasks.Count},
		},
		Atlas: Atlas{
			Managed:   pointerAtlas("managed", sources.Atlas.Managed),
			Owner:     pointerAtlas("owner", sources.Atlas.Owner),
			Workspace: pointerAtlas("workspace", sources.Atlas.Workspace),
		},
		Execution: ExecutionContext{Active: executionPointer(sources.Execution)},
		Skills:    Skills{CatalogPointer: skillsCatalogPointer, State: "available"},
		Agents: Agents{
			CatalogPointer: agentsCatalogPointer, Hub: "maestro", DefinitionsState: "available", RuntimeState: "unavailable",
			Message: "native agent orchestration requires a runtime adapter with tool and delegation enforcement",
		},
		Memory: Memory{State: "unavailable", Message: "memory context injection requires a runtime adapter"},
	}
	if sources.Workspace.State != "ready" && sources.Workspace.State != "warning" {
		packet.Omissions = append(packet.Omissions, Omission{Source: "workspace", Reason: "workspace is not ready"})
	}
	if !sources.Owner.Initialized {
		packet.Omissions = append(packet.Omissions, Omission{Source: "owner", Reason: "owner context is not initialized"})
	}
	if !sources.Atlas.Owner.Available || !sources.Atlas.Workspace.Available {
		packet.Omissions = append(packet.Omissions, Omission{Source: "atlas", Reason: "human atlas bootstrap is incomplete"})
	}
	if packet.Execution.Active.State == execution.ActivePointerAmbiguous {
		packet.Omissions = append(packet.Omissions, Omission{Source: "execution", Reason: "multiple active execution items require explicit resolution"})
	}
	if len(packet.Omissions) > 0 {
		packet.State = "partial"
	}
	sort.Slice(packet.Omissions, func(left, right int) bool { return packet.Omissions[left].Source < packet.Omissions[right].Source })
	return packet
}

func (packet Packet) Validate() error {
	if packet.SchemaVersion != 1 || (packet.State != "ready" && packet.State != "partial") {
		return errors.New("invalid session context packet header")
	}
	if packet.InteractionProfile.ID == "" || packet.InteractionProfile.Source == "" || packet.Workspace.State == "" || packet.Owner.Onboarding.State == "" || packet.Skills.CatalogPointer != skillsCatalogPointer || packet.Skills.State != "available" || packet.Agents.CatalogPointer != agentsCatalogPointer || packet.Agents.Hub != "maestro" || packet.Agents.DefinitionsState != "available" || packet.Agents.RuntimeState != "unavailable" || packet.Agents.Message == "" || packet.Memory.State != "unavailable" || packet.Memory.Message == "" {
		return errors.New("session context packet is missing a required bounded source")
	}
	if packet.Owner.Onboarding.State != "required" && packet.Owner.Onboarding.State != "in_progress" && packet.Owner.Onboarding.State != "review_required" && packet.Owner.Onboarding.State != "complete" {
		return errors.New("session context packet has an invalid onboarding state")
	}
	if (packet.Owner.Onboarding.State == "required" || packet.Owner.Onboarding.State == "in_progress") && packet.Owner.Onboarding.NextQuestion == "" {
		return errors.New("pending onboarding is missing its next question")
	}
	if packet.Owner.OpenTasks.State != "unavailable" && packet.Owner.OpenTasks.State != "empty" && packet.Owner.OpenTasks.State != "available" {
		return errors.New("session context packet has an invalid open task state")
	}
	if len(packet.Skills.Selected) > 2 {
		return errors.New("session context packet selects too many skills")
	}
	for _, selected := range packet.Skills.Selected {
		if selected.ID == "" || selected.Reason == "" || selected.Pointer == "" {
			return errors.New("session context packet has an invalid selected skill pointer")
		}
	}
	if packet.ActionConfirmation != nil && packet.ActionConfirmation.State != "confirmed" {
		return errors.New("session context packet has an invalid action confirmation state")
	}
	for id, facet := range packet.Owner.Facets {
		if id == "" || facet.Path == "" || facet.State == "" {
			return errors.New("session context packet has an invalid owner facet pointer")
		}
	}
	active := packet.Execution.Active
	switch active.State {
	case execution.ActivePointerAvailable:
		if !active.Available || active.Path != execution.ActivePointerPath {
			return errors.New("session context packet has an invalid active execution pointer")
		}
	case execution.ActivePointerUnavailable, execution.ActivePointerAmbiguous:
		if active.Available || active.Path != "" {
			return errors.New("session context packet exposes an unresolved active execution pointer")
		}
	default:
		return errors.New("session context packet has an invalid active execution state")
	}
	if active.State == execution.ActivePointerAmbiguous && (packet.State != "partial" || !hasOmission(packet.Omissions, "execution")) {
		return errors.New("ambiguous active execution must be reported as a partial omission")
	}
	if packet.State == "ready" && len(packet.Omissions) != 0 {
		return errors.New("ready session context packet has omissions")
	}
	return nil
}

func hasOmission(omissions []Omission, source string) bool {
	for _, omission := range omissions {
		if omission.Source == source {
			return true
		}
	}
	return false
}

func executionPointer(value execution.ActivePointer) Pointer {
	if value.State == "" {
		return Pointer{State: execution.ActivePointerUnavailable}
	}
	return Pointer{Path: value.Path, Available: value.Available, State: value.State}
}

func sessionFacets(facets map[string]ownerctx.Facet) map[string]Pointer {
	result := make(map[string]Pointer)
	for id, facet := range facets {
		if _, safe := sessionSafeFacets[id]; safe && facet.Sensitivity != "sensitive" && hasReader(facet.Readers, "session") {
			result[id] = pointer(facet.Pointer)
		}
	}
	return result
}

func hasReader(readers []string, wanted string) bool {
	for _, reader := range readers {
		if reader == wanted {
			return true
		}
	}
	return false
}

func pointer(value ownerctx.Pointer) Pointer {
	return Pointer{Path: value.Path, Available: value.Available, State: value.State}
}

func pointerAtlas(scope string, value atlas.Pointer) Pointer {
	pointer := Pointer{Available: value.Available, State: value.State}
	if value.State != "unavailable" {
		pointer.Path = "bcgos://atlas/" + scope
	}
	return pointer
}
