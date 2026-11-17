package ownerctx

import (
	"errors"
	"os"
)

// weeklySelfSourceChange is an internal source-owner barrier, not model input.
// Owners pass identities whose bytes, eligibility or policy will change. All is
// reserved for reset/configuration/retention changes affecting the whole source.
type weeklySelfSourceChange struct {
	All          bool
	Facets       []string
	Prompts      []string
	Observations []string
}

// invalidateWeeklySelfSourcesLocked takes ONLY the weekly lock, last. Its caller
// must already exclude the owning source mutation from weekly snapshots. Purge
// happens before the source changes: a later source-write failure may lose a
// rebuildable derived view but cannot leave stale private text after deletion.
// Never call PruneWeeklySelf from a source mutation; it reacquires source locks.
//
// This helper alone wires no mutation. Reviewed history/observation/facet/reset
// owners must call it in their own fenced transaction and recovery paths.
func invalidateWeeklySelfSourcesLocked(root string, change weeklySelfSourceChange) (bool, error) {
	if _, err := readOwnerFile(weeklySelfPath(root), weeklySelfMaximumStateBytes); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	changed := false
	err := withWeeklySelfState(root, false, func(state *weeklySelfState) error {
		if state.Pending != nil && change.affects(sourceFromWeeklyInput(state.Pending.Input)) {
			state.Pending = nil
			changed = true
		}
		if state.Working != nil && change.affects(state.Working.Source) {
			state.Working = nil
			changed = true
		}
		if changed {
			return writeWeeklySelfState(root, *state)
		}
		return nil
	})
	return changed && err == nil, err
}
func (change weeklySelfSourceChange) affects(source weeklySelfSource) bool {
	if change.All {
		return true
	}
	for _, id := range change.Facets {
		if _, ok := source.FacetDigests[id]; ok {
			return true
		}
	}
	for _, id := range change.Prompts {
		if _, ok := source.Prompts[id]; ok {
			return true
		}
	}
	for _, id := range change.Observations {
		if _, ok := source.Observations[id]; ok {
			return true
		}
		if _, ok := source.ObservationSupport[id]; ok {
			return true
		}
	}
	return false
}
