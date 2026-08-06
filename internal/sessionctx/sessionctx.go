// Package sessionctx builds the bounded, runtime-neutral Session Context
// Packet. It returns pointers and availability only; adapters decide whether
// an authorized source may be read later.
package sessionctx

import (
	"errors"
	"path"
	"sort"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/continuoususe"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/execution"
	basememory "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/priorwork"
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
	"motivations":         {},
	"quality-bar":         {},
	"decision-rules":      {},
	"working-boundaries":  {},
}

type Sources struct {
	Profile   profile.State
	Workspace workspace.Inspection
	Owner     ownerctx.Status
	// OwnerContextRoot is a local directive anchor only. Owner context is
	// intentionally stored in the private data root, never inferred from a
	// workspace-local owner/ directory.
	OwnerContextRoot string
	Atlas            atlas.Status
	Execution        execution.ActivePointer
	Memory           MemorySource
	SharePointSource priorwork.SourceSelectionStatus
	ContinuousUse    continuoususe.Status
}

// MemorySource is assembled by the local runtime boundary. Build never reads
// local memory files itself, and the serialized packet receives only portable
// layer pointers. Sections remain ephemeral input for SessionStart rendering.
type MemorySource struct {
	State  string
	Bundle basememory.ContextBundle
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
	SelfIndex      Pointer            `json:"self_index"`
	Expansion      SelfExpansion      `json:"expansion"`
	OpenTasks      OpenTasks          `json:"open_tasks"`
}

type SelfExpansion struct {
	State       string `json:"state"`
	Total       int    `json:"total"`
	Current     int    `json:"current"`
	Unknown     int    `json:"unknown"`
	Stale       int    `json:"stale"`
	NextFacet   string `json:"next_facet,omitempty"`
	ReviewCount int    `json:"review_count"`
}

type Onboarding struct {
	State        string `json:"state"`
	Track        string `json:"track"`
	NextQuestion string `json:"next_question,omitempty"`
	ReviewDigest string `json:"review_digest,omitempty"`
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
	State    string                      `json:"state"`
	Message  string                      `json:"message"`
	Layers   []MemoryLayer               `json:"layers,omitempty"`
	Sections []basememory.ContextSection `json:"-"`
}

type MemoryLayer struct {
	Layer     string `json:"layer"`
	Pointer   string `json:"pointer"`
	Truncated bool   `json:"truncated"`
}

type ExecutionContext struct {
	Active Pointer `json:"active"`
}

// SharePointSource contains only bounded setup state. Exact URLs remain behind
// Pointer in private local storage and collection authority remains separate.
type SharePointSource struct {
	State                string `json:"state"`
	Pointer              string `json:"pointer,omitempty"`
	FolderCount          int    `json:"folder_count"`
	SourceAuthority      string `json:"source_authority"`
	LocalProjection      string `json:"local_projection"`
	AuthorizationState   string `json:"authorization_state"`
	CollectionRuntime    string `json:"collection_runtime"`
	CollectionState      string `json:"collection_state"`
	CodexCollectionState string `json:"codex_collection_state"`
}

type Omission struct {
	Source string `json:"source"`
	Reason string `json:"reason"`
}

type Packet struct {
	SchemaVersion      int                  `json:"schema_version"`
	State              string               `json:"state"`
	InteractionProfile InteractionProfile   `json:"interaction_profile"`
	Workspace          Workspace            `json:"workspace"`
	Owner              Owner                `json:"owner"`
	Atlas              Atlas                `json:"atlas"`
	Execution          ExecutionContext     `json:"execution"`
	SharePointSource   SharePointSource     `json:"sharepoint_source"`
	Skills             Skills               `json:"skills"`
	Agents             Agents               `json:"agents"`
	Memory             Memory               `json:"memory"`
	ContinuousUse      continuoususe.Status `json:"continuous_use"`
	ActionConfirmation *ActionConfirmation  `json:"action_confirmation,omitempty"`
	Omissions          []Omission           `json:"omissions"`
	// WorkspaceRoot is used only to anchor native hook directives. It is not
	// serialized into the bounded packet or persisted as context content.
	WorkspaceRoot string `json:"-"`
	// MaestroCLIPath is the exact installed executable that emitted this
	// lifecycle packet. It is a local directive anchor, never bounded context
	// or persisted owner data.
	MaestroCLIPath string `json:"-"`
	// OwnerContextRoot anchors onboarding/refinement commands to the canonical
	// private data root. It is never serialized into the bounded packet.
	OwnerContextRoot string `json:"-"`
}

func Build(sources Sources) Packet {
	onboarding := sources.Owner.Onboarding
	if onboarding.State == "" {
		interview := ownerctx.ColdStartInterview()
		onboarding = ownerctx.OnboardingStatus{State: "required", Track: "selection_required", Remaining: []string{"professional-role"}}
		if len(interview.Steps) > 0 {
			onboarding.NextQuestion = ownerctx.InterviewStep{Facet: "onboarding-track", Question: "Você prefere a entrevista curta ou a completa?"}
		}
	}
	openTasks := sources.Owner.OpenTasks
	if openTasks.State == "" {
		openTasks.State = "unavailable"
	}
	selfIndex := sources.Owner.SelfIndex
	expansion := sources.Owner.Expansion
	if sources.Owner.Initialized && selfIndex.State == "" {
		selfIndex = ownerctx.Pointer{Path: "owner/self/README.md", Available: true, State: "available"}
	}
	if sources.Owner.Initialized && expansion.State == "" {
		expansion = ownerctx.ExpansionStatus{State: "action_required", Total: ownerctx.SelfFacetCount(), Unknown: ownerctx.SelfFacetCount(), NextFacet: "professional-role"}
	}
	if !sources.Owner.Initialized && expansion.State == "" {
		expansion.State = "unavailable"
	}
	sharePointSource := sources.SharePointSource
	if sharePointSource.State == "" {
		sharePointSource = priorwork.SourceSelectionStatus{
			SchemaVersion: 1, State: priorwork.SourceSelectionRequired,
			SourceAuthority: "sharepoint", LocalProjection: "metadata_and_source_pointers_only",
			AuthorizationState: "not_selected", CollectionRuntime: "claude",
			CollectionState: "unavailable", CodexCollectionState: "unavailable/corporate_policy",
		}
	}
	continuous := sources.ContinuousUse
	if continuous.SchemaVersion == 0 {
		fallback, err := continuoususe.Build(continuoususe.Source{
			WorkspaceState: sources.Workspace.State, CalibrationState: onboarding.State, CalibrationTrack: onboarding.Track,
			OpenTasksState: openTasks.State, OpenTasksCount: openTasks.Count,
			OpenWorkState: execution.ActivePointerUnavailable, MemoryState: normalizedMemoryState(sources.Memory.State),
		})
		if err == nil {
			continuous = fallback
		} else {
			continuous = continuoususe.Status{
				SchemaVersion: 1, State: continuoususe.StateUnavailable,
				Calibration: continuoususe.Calibration{State: onboarding.State, Track: onboarding.Track},
				OpenTasks:   continuoususe.OpenTasks{State: openTasks.State, Count: openTasks.Count},
				OpenWork:    continuoususe.OpenWork{State: execution.ActivePointerUnavailable, CheckpointState: execution.CheckpointUnavailable},
				Memory:      continuoususe.MemoryStatus{State: "unavailable"},
				Signals: continuoususe.SignalEvidence{CapabilityEvidence: continuoususe.CapabilityEvidence{
					State: continuoususe.EvidenceUnavailable, Unavailable: true, Reason: continuoususe.ReasonSourceUnavailable,
				}},
				Maintenance: continuoususe.CapabilityEvidence{
					State: continuoususe.EvidenceUnavailable, Unavailable: true, Reason: continuoususe.ReasonSourceUnavailable,
				},
				NextActions: []continuoususe.NextAction{{ID: continuoususe.ActionInspectContinuity, Command: "bcgos maestro status <workspace>", Reason: "inspect the bounded continuity projection after source state changes"}},
			}
		}
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
			Onboarding:     Onboarding{State: onboarding.State, Track: onboarding.Track, NextQuestion: onboarding.NextQuestion.Question, ReviewDigest: onboarding.ReviewDigest},
			SelfIndex:      pointer(selfIndex),
			Expansion:      SelfExpansion{State: expansion.State, Total: expansion.Total, Current: expansion.Current, Unknown: expansion.Unknown, Stale: expansion.Stale, NextFacet: expansion.NextFacet, ReviewCount: expansion.ReviewCount},
			OpenTasks:      OpenTasks{State: openTasks.State, Count: openTasks.Count},
		},
		Atlas: Atlas{
			Managed:   pointerAtlas("managed", sources.Atlas.Managed),
			Owner:     pointerAtlas("owner", sources.Atlas.Owner),
			Workspace: pointerAtlas("workspace", sources.Atlas.Workspace),
		},
		Execution: ExecutionContext{Active: executionPointer(sources.Execution)},
		SharePointSource: SharePointSource{
			State: sharePointSource.State, Pointer: sharePointSource.Pointer, FolderCount: sharePointSource.FolderCount,
			SourceAuthority: sharePointSource.SourceAuthority, LocalProjection: sharePointSource.LocalProjection,
			AuthorizationState: sharePointSource.AuthorizationState, CollectionRuntime: sharePointSource.CollectionRuntime,
			CollectionState: sharePointSource.CollectionState, CodexCollectionState: sharePointSource.CodexCollectionState,
		},
		Skills: Skills{CatalogPointer: skillsCatalogPointer, State: "available"},
		Agents: Agents{
			CatalogPointer: agentsCatalogPointer, Hub: "maestro", DefinitionsState: "available", RuntimeState: "unavailable",
			Message: "native agent orchestration requires a runtime adapter with tool and delegation enforcement",
		},
		Memory:           buildMemory(sources.Memory),
		ContinuousUse:    continuous,
		WorkspaceRoot:    sources.Workspace.WorkspacePath,
		OwnerContextRoot: sources.OwnerContextRoot,
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
	if packet.InteractionProfile.ID == "" || packet.InteractionProfile.Source == "" || packet.Workspace.State == "" || packet.Owner.Onboarding.State == "" || packet.Owner.Onboarding.Track == "" || packet.Skills.CatalogPointer != skillsCatalogPointer || packet.Skills.State != "available" || packet.Agents.CatalogPointer != agentsCatalogPointer || packet.Agents.Hub != "maestro" || packet.Agents.DefinitionsState != "available" || packet.Agents.RuntimeState != "unavailable" || packet.Agents.Message == "" || packet.Memory.Message == "" || packet.ContinuousUse.SchemaVersion != 1 || len(packet.ContinuousUse.NextActions) == 0 {
		return errors.New("session context packet is missing a required bounded source")
	}
	if err := packet.ContinuousUse.Validate(); err != nil {
		return err
	}
	if packet.Memory.State != "available" && packet.Memory.State != "empty" && packet.Memory.State != "unavailable" {
		return errors.New("session context packet has an invalid memory state")
	}
	if packet.Memory.State == "available" && len(packet.Memory.Layers) == 0 {
		return errors.New("available session memory is missing bounded layers")
	}
	if packet.Memory.State != "available" && (len(packet.Memory.Layers) != 0 || len(packet.Memory.Sections) != 0) {
		return errors.New("inactive session memory exposes bounded layers")
	}
	for _, layer := range packet.Memory.Layers {
		if !validMemoryLayer(layer.Layer) || layer.Pointer != "bcgos://memory/"+layer.Layer {
			return errors.New("session context packet has an invalid memory pointer")
		}
	}
	if packet.Owner.Onboarding.State != "required" && packet.Owner.Onboarding.State != "in_progress" && packet.Owner.Onboarding.State != "review_required" && packet.Owner.Onboarding.State != "complete" {
		return errors.New("session context packet has an invalid onboarding state")
	}
	if (packet.Owner.Onboarding.State == "required" || packet.Owner.Onboarding.State == "in_progress") && packet.Owner.Onboarding.NextQuestion == "" {
		return errors.New("pending onboarding is missing its next question")
	}
	if packet.Owner.Onboarding.State == "review_required" && (packet.Owner.Onboarding.NextQuestion != "" || packet.Owner.Onboarding.ReviewDigest == "") {
		return errors.New("review-required onboarding is missing its bounded review digest")
	}
	if packet.Owner.Onboarding.State != "review_required" && packet.Owner.Onboarding.ReviewDigest != "" {
		return errors.New("session context packet exposes a review digest outside review-required onboarding")
	}
	if packet.Owner.OpenTasks.State != "unavailable" && packet.Owner.OpenTasks.State != "empty" && packet.Owner.OpenTasks.State != "available" {
		return errors.New("session context packet has an invalid open task state")
	}
	if packet.Owner.Initialized {
		expansion := packet.Owner.Expansion
		if packet.Owner.SelfIndex.Path != "owner/self/README.md" || !packet.Owner.SelfIndex.Available || packet.Owner.SelfIndex.State != "available" {
			return errors.New("initialized owner is missing the canonical SELF index pointer")
		}
		if expansion.Total != ownerctx.SelfFacetCount() || expansion.Current < 0 || expansion.Unknown < 0 || expansion.Stale < 0 || expansion.ReviewCount < 0 || expansion.Current+expansion.Unknown+expansion.Stale != expansion.Total {
			return errors.New("session context packet has invalid SELF expansion counts")
		}
		switch expansion.State {
		case "current":
			if expansion.NextFacet != "" || expansion.ReviewCount != 0 || expansion.Current != expansion.Total {
				return errors.New("current SELF expansion summary is inconsistent")
			}
		case "action_required":
			if _, ok := sessionSafeFacets[expansion.NextFacet]; !ok || expansion.ReviewCount != 0 {
				return errors.New("action-required SELF expansion summary is inconsistent")
			}
		case "review_required":
			if expansion.NextFacet != "" || expansion.ReviewCount <= 0 {
				return errors.New("review-required SELF expansion summary is inconsistent")
			}
		default:
			return errors.New("session context packet has an invalid SELF expansion state")
		}
	} else if packet.Owner.Expansion.State != "unavailable" {
		return errors.New("uninitialized owner exposes SELF expansion state")
	}
	if err := validateSharePointSource(packet.SharePointSource); err != nil {
		return err
	}
	if len(packet.Skills.Selected) > 2 {
		return errors.New("session context packet selects too many skills")
	}
	for _, selected := range packet.Skills.Selected {
		if !validSelectedSkill(selected) {
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

func normalizedMemoryState(state string) string {
	switch state {
	case "available", "empty", "unavailable":
		return state
	default:
		return "unavailable"
	}
}

func validSelectedSkill(selected SkillSelection) bool {
	if !agentcatalog.ValidAgentID(selected.ID) {
		return false
	}
	switch selected.Reason {
	case "explicit_skill_reference", "lexical_intent", "deterministic_onboarding_state":
	default:
		return false
	}
	cleaned := path.Clean(strings.TrimSpace(selected.Pointer))
	if cleaned == "." || path.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return false
	}
	claudePointer := path.Join(".claude", "skills", selected.ID, "SKILL.md")
	codexPointer := path.Join(".codex", "skills", selected.ID, "SKILL.md")
	return cleaned == claudePointer || cleaned == codexPointer
}

func buildMemory(source MemorySource) Memory {
	result := Memory{State: "unavailable", Message: "local memory context is unavailable; raw history was not injected"}
	switch source.State {
	case "empty":
		result.State = "empty"
		result.Message = "local memory is active; no generated layer is currently available"
	case "available":
		if len(source.Bundle.Sections) == 0 {
			result.State = "empty"
			result.Message = "local memory is active; no generated layer is currently available"
			return result
		}
		result.State = "available"
		result.Message = "bounded local memory is available for this SessionStart"
		result.Sections = append([]basememory.ContextSection(nil), source.Bundle.Sections...)
		lastRank := -1
		for _, section := range source.Bundle.Sections {
			rank := memoryLayerRank(section.Layer)
			if rank <= lastRank || strings.TrimSpace(section.Content) == "" {
				return Memory{State: "unavailable", Message: "local memory context is invalid; raw history was not injected"}
			}
			lastRank = rank
			result.Layers = append(result.Layers, MemoryLayer{Layer: section.Layer, Pointer: "bcgos://memory/" + section.Layer, Truncated: section.Truncated})
		}
	}
	return result
}

func validMemoryLayer(layer string) bool {
	return memoryLayerRank(layer) >= 0
}

func memoryLayerRank(layer string) int {
	switch layer {
	case "lifetime":
		return 0
	case "L3":
		return 1
	case "L2":
		return 2
	case "L1":
		return 3
	default:
		return -1
	}
}

func validateSharePointSource(source SharePointSource) error {
	if source.State != priorwork.SourceSelectionRequired && source.State != priorwork.SourceSelected && source.State != priorwork.SourceDeferred && source.State != priorwork.SourceSelectionUnavailable {
		return errors.New("session context packet has an invalid SharePoint source selection state")
	}
	if source.SourceAuthority != "sharepoint" || source.LocalProjection != "metadata_and_source_pointers_only" || source.CollectionRuntime != "claude" || source.CollectionState != "unavailable" || source.CodexCollectionState != "unavailable/corporate_policy" {
		return errors.New("session context packet weakens the SharePoint source or runtime boundary")
	}
	switch source.State {
	case priorwork.SourceSelectionRequired:
		if source.Pointer != "" || source.FolderCount != 0 || source.AuthorizationState != "not_selected" {
			return errors.New("pending SharePoint source selection exposes unexpected state")
		}
	case priorwork.SourceSelected:
		if source.Pointer == "" || source.FolderCount <= 0 || source.AuthorizationState != "pending_signed_enrollment" {
			return errors.New("selected SharePoint source is missing its bounded authorization state")
		}
	case priorwork.SourceDeferred:
		if source.Pointer == "" || source.FolderCount != 0 || source.AuthorizationState != "deferred_by_owner" {
			return errors.New("deferred SharePoint source selection is invalid")
		}
	case priorwork.SourceSelectionUnavailable:
		if source.FolderCount != 0 || source.AuthorizationState != "unavailable" {
			return errors.New("unavailable SharePoint source selection is invalid")
		}
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
