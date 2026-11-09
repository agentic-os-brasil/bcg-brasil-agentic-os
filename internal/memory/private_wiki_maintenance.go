package memory

import (
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/privatewiki"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
)

// PrivateWikiMaintenanceSnapshot contains only source-owned opaque metadata.
// It grants neither publication nor access to a source body.
type PrivateWikiMaintenanceSnapshot struct {
	SourceReady           bool      `json:"source_ready"`
	SourceIdentity        string    `json:"source_identity,omitempty"`
	SourceWatermark       string    `json:"source_watermark,omitempty"`
	EventID               string    `json:"event_id,omitempty"`
	EventAt               time.Time `json:"event_at,omitempty"`
	EventPending          bool      `json:"event_pending"`
	EventMissing          bool      `json:"event_missing"`
	EventRecovered        bool      `json:"event_recovered"`
	DenialGeneration      uint64    `json:"denial_generation"`
	RetirementComplete    bool      `json:"retirement_complete"`
	State                 string    `json:"state"`
	Parent                string    `json:"parent,omitempty"`
	Generation            string    `json:"generation,omitempty"`
	GraphEvidence         string    `json:"graph_evidence,omitempty"`
	Implementation        string    `json:"implementation,omitempty"`
	Configuration         string    `json:"configuration,omitempty"`
	ImplementationCurrent bool      `json:"implementation_current"`
	ConfigurationCurrent  bool      `json:"configuration_current"`
	Concepts              int       `json:"concepts"`
	Files                 int       `json:"files"`
}

// InspectPrivateWikiMaintenance inspects existing source, event and graph
// authority without preparing a cohort, key, event, lock or directory.
func (engine *Engine) InspectPrivateWikiMaintenance(workspace string) (PrivateWikiMaintenanceSnapshot, error) {
	out := PrivateWikiMaintenanceSnapshot{State: "unavailable"}
	if _, err := processidentity.Current(); err != nil {
		return out, err
	}
	custody, err := engine.holdPrivateWikiWorkspace(workspace)
	if err != nil {
		return out, err
	}
	defer custody()
	key, _, _, err := engine.privateWikiScope(workspace)
	if err != nil {
		return out, err
	}
	barrier, err := engine.readVisibility(workspace)
	if err != nil {
		return out, err
	}
	out.DenialGeneration = barrier.Generation
	out.RetirementComplete = barrier.PrivateWiki == nil || barrier.PrivateWiki.RetiredThrough == barrier.Generation
	if barrier.PrivateWiki == nil {
		// A genuine legacy source has no protected derived subtree. Its absence
		// is not a repair opportunity for a downgraded protected checkpoint.
		if _, err := memoryReadDir(engine.privateWikiRoot(workspace)); !errors.Is(err, os.ErrNotExist) {
			return out, ErrPrivateWikiCohort
		}
	}
	snapshot, sourceErr := engine.privateWikiSnapshot(workspace)
	if sourceErr == nil {
		out.SourceReady = true
		out.SourceWatermark = snapshot.input.Watermark
		out.SourceIdentity = privateWikiMAC(key, "maintenance-source", []byte(snapshot.input.Watermark), []byte(snapshot.input.Implementation), []byte(snapshot.input.Configuration))
		body, err := memoryReadFile(engine.privateWikiEventPath(workspace, snapshot.commitName), 4096)
		if errors.Is(err, os.ErrNotExist) {
			out.EventMissing = true
		} else if err != nil {
			return out, ErrPrivateWikiCohort
		} else {
			var event privateWikiSourceEvent
			policy, _ := json.Marshal(engine.Policy)
			if decodeMemoryJSON(body, &event) != nil || !validPrivateWikiEvent(event, workspace) || event.SourceCommit != snapshot.commitName || event.Generation != barrier.Generation || event.PolicyDigest != privatewiki.Digest(policy) || event.Reason == "access_revoked" || !event.Recovered && !event.CreatedAt.Equal(snapshot.input.CommittedAt) {
				return out, ErrPrivateWikiCohort
			}
			out.EventID, out.EventAt, out.EventRecovered = event.ID, event.CreatedAt, event.Recovered
		}
	} else if barrier.PrivateWiki != nil && barrier.PrivateWiki.DenialEvent != nil {
		// Denial can leave no eligible rollup. Its real event remains observable;
		// it cannot turn an absent source into compilation/qualification proof.
		event := barrier.PrivateWiki.DenialEvent
		out.EventID, out.EventAt, out.EventRecovered = event.ID, event.CreatedAt, event.Recovered
		out.SourceIdentity = privateWikiMAC(key, "retirement-source", []byte(event.ID))
	} else {
		return out, sourceErr
	}
	if !out.RetirementComplete {
		out.State, out.EventPending = "retirement_pending", out.EventID != ""
		return out, nil
	}
	if !out.SourceReady {
		return out, nil
	}
	if barrier.PrivateWiki == nil {
		out.State, out.EventPending = "absent", out.EventID != ""
		return out, nil
	}
	graph, err := engine.loadPrivateWiki(workspace, snapshot)
	if errors.Is(err, os.ErrNotExist) {
		out.State, out.EventPending = "absent", out.EventID != ""
	} else if err != nil {
		return out, err
	} else {
		out.Parent, out.Generation = graph.Manifest.Generation, graph.Manifest.Generation
		out.State = "stale"
		out.EventPending = out.EventID != ""
		if privateWikiSourcesMatch(graph.Manifest, snapshot.input) {
			out.State, out.EventPending = "current", false
			out.GraphEvidence = privateWikiMAC(key, "maintenance-graph", graph.Files["manifest.json"])
			out.Implementation, out.Configuration = graph.Manifest.Implementation, graph.Manifest.Configuration
			out.ImplementationCurrent = graph.Manifest.Implementation == snapshot.input.Implementation
			out.ConfigurationCurrent = graph.Manifest.Configuration == snapshot.input.Configuration
			out.Concepts, out.Files = len(graph.Manifest.Concepts), len(graph.Files)
		}
	}
	after, err := engine.privateWikiSnapshot(workspace)
	if err != nil || after.input.Watermark != snapshot.input.Watermark {
		return PrivateWikiMaintenanceSnapshot{State: "unavailable"}, ErrPrivateWikiStale
	}
	return out, nil
}
