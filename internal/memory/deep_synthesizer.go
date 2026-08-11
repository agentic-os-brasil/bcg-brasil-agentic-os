package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// DeterministicWeeklySynthesizer is the bundled, runtime-neutral rollup
// adapter. It never calls a remote model: it preserves bounded, provenance
// checked continuity from already active artifacts.
const DeterministicWeeklySynthesizerID = "deterministic-weekly-v1"

type DeterministicWeeklySynthesizer struct {
	Daily         DeterministicL1Synthesizer
	MaxRunes      map[string]int
	MaxInputBytes int
}

func (synthesizer DeterministicWeeklySynthesizer) Synthesize(ctx context.Context, request SynthesisRequest) (string, error) {
	if request.Cycle == "daily" && request.TargetLayer == "L1" {
		return synthesizer.Daily.Synthesize(ctx, request)
	}
	if request.Cycle != "weekly" || request.WorkspaceID == "" || request.Period == "" {
		return "", errors.New("deterministic weekly synthesizer requires a bounded weekly request")
	}
	if request.TargetLayer != "L2" && request.TargetLayer != "L3" && request.TargetLayer != "lifetime" {
		return "", errors.New("deterministic weekly synthesizer accepts only L2, L3 or lifetime")
	}
	limit := synthesizer.MaxRunes[request.TargetLayer]
	if limit <= 0 || synthesizer.MaxInputBytes <= 0 {
		return "", errors.New("deterministic weekly synthesizer requires explicit positive bounds")
	}
	if len(request.Sources) == 0 {
		return "", errors.New("deterministic weekly synthesis requires at least one source artifact")
	}

	sources := append([]SourceDocument(nil), request.Sources...)
	sort.Slice(sources, func(left, right int) bool {
		leftRank, rightRank := weeklySourceRank(request.TargetLayer, sources[left].ID), weeklySourceRank(request.TargetLayer, sources[right].ID)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return sources[left].ID < sources[right].ID
	})
	totalBytes := 0
	parts := make([]string, 0, len(sources))
	for _, source := range sources {
		totalBytes += len(source.Content)
		if totalBytes > synthesizer.MaxInputBytes {
			return "", errors.New("weekly synthesis source bytes exceed the configured bound")
		}
		var artifact Artifact
		if err := json.Unmarshal(source.Content, &artifact); err != nil {
			return "", fmt.Errorf("decode weekly source artifact: %w", err)
		}
		if err := validateArtifactCore(artifact); err != nil {
			return "", fmt.Errorf("validate weekly source artifact: %w", err)
		}
		if artifact.WorkspaceID != request.WorkspaceID || !validWeeklySource(request.TargetLayer, artifact.Layer) {
			return "", errors.New("weekly synthesis source is outside its allowed layer boundary")
		}
		parts = append(parts, "## source "+source.ID+"\n"+strings.TrimSpace(artifact.Content))
	}

	header := "# " + weeklyLayerLabel(request.TargetLayer) + " · " + request.Period
	body := header + "\n\n" + strings.Join(parts, "\n\n")
	return truncateWeeklyRunes(body, limit), nil
}

func weeklySourceRank(target, sourceID string) int {
	// When rolling L3 has a tight context budget, retain the prior L3 first so
	// continuity is visible to the named lifetime eligibility policy.
	if target == "L3" && strings.HasPrefix(sourceID, "L3/") {
		return 0
	}
	return 1
}

func validWeeklySource(target, layer string) bool {
	switch target {
	case "L2":
		return layer == "L1"
	case "L3":
		return layer == "L2" || layer == "L3"
	case "lifetime":
		return layer == "L3" || layer == "lifetime"
	default:
		return false
	}
}

func weeklyLayerLabel(layer string) string {
	switch layer {
	case "L2":
		return "L2 weekly continuity"
	case "L3":
		return "L3 rolling continuity"
	default:
		return "Lifetime candidate"
	}
}

func truncateWeeklyRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit < 2 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}

// DeterministicLifetimeEligibility requires the current L3 rollup to carry
// two weekly L3 generations. It therefore cannot promote on the first weekly
// pass and records why a candidate was retained for further continuity.
type DeterministicLifetimeEligibility struct {
	MinL3Generations int
}

func (policy DeterministicLifetimeEligibility) Evaluate(_ context.Context, artifact Artifact) (bool, string, error) {
	if artifact.Layer != "lifetime" || strings.TrimSpace(artifact.Content) == "" {
		return false, "", errors.New("deterministic lifetime eligibility requires a non-empty lifetime candidate")
	}
	minimum := policy.MinL3Generations
	if minimum < 2 {
		minimum = 2
	}
	generations := strings.Count(artifact.Content, "# L3 rolling continuity ·")
	if generations < minimum {
		return false, fmt.Sprintf("requires %d weekly L3 generations; observed %d", minimum, generations), nil
	}
	return true, fmt.Sprintf("eligible after %d weekly L3 generations", generations), nil
}
