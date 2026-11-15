package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// DeterministicWeeklySynthesizer is the bundled, runtime-neutral rollup
// adapter. It never calls a remote model: it preserves bounded, provenance
// checked continuity from already active artifacts.
const DeterministicWeeklySynthesizerID = "deterministic-weekly-v1"

type DeterministicWeeklySynthesizer struct {
	Daily         DeterministicL1Synthesizer
	MaxRunes      map[string]int
	MaxBytes      map[string]int
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

	byteLimit := synthesizer.MaxBytes[request.TargetLayer]
	if synthesizer.MaxBytes != nil && byteLimit <= 0 {
		return "", errors.New("deterministic weekly synthesizer requires an explicit byte bound for the target")
	}
	type input struct {
		document SourceDocument
		artifact Artifact
	}
	sources := make([]input, 0, len(request.Sources))
	totalBytes := 0
	for _, source := range request.Sources {
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
		sources = append(sources, input{source, artifact})
	}
	sort.SliceStable(sources, func(i, j int) bool {
		left, right := weeklySourceRank(request.TargetLayer, sources[i].artifact.Layer), weeklySourceRank(request.TargetLayer, sources[j].artifact.Layer)
		if left != right {
			return left < right
		}
		return sources[i].document.ID > sources[j].document.ID
	})
	header := "# " + weeklyLayerLabel(request.TargetLayer) + " · " + request.Period
	selected := []string{}
	seen := map[string]bool{}
	for _, source := range sources {
		units := weeklyContentUnits(source.artifact)
		if len(units) == 0 && weeklySourceRank(request.TargetLayer, source.artifact.Layer) == 0 {
			return "", errors.New("current weekly source has no complete observation")
		}
		for _, unit := range units {
			key := strings.TrimSpace(unit) // Interior whitespace may carry authored/code meaning.
			if seen[key] {
				continue
			}
			seen[key] = true
			body := header + "\n\n" + strings.Join(append(append([]string(nil), selected...), unit), "\n\n")
			if utf8.RuneCountInString(body) > limit || byteLimit > 0 && len(body) > byteLimit {
				if len(selected) == 0 && weeklySourceRank(request.TargetLayer, source.artifact.Layer) == 0 {
					return "", errors.New("no complete current weekly observation fits the configured bounds")
				}
				continue
			}
			selected = append(selected, unit)
		}
	}
	if len(selected) == 0 {
		return "", errors.New("weekly synthesis has no complete observation within the configured bounds")
	}
	return header + "\n\n" + strings.Join(selected, "\n\n"), nil
}

func weeklySourceRank(target, layer string) int {
	if target == "L3" && layer == "L3" || target == "lifetime" && layer == "lifetime" {
		return 1
	}
	return 0
}

// Remove only this producer family's generated framing. Authored headings,
// paragraph continuations and fenced examples remain part of complete units.
func weeklyContentUnits(artifact Artifact) []string {
	generated := artifact.SynthesizerID == DeterministicL1SynthesizerID || artifact.SynthesizerID == DeterministicWeeklySynthesizerID || artifact.SynthesizerID == "deterministic-weekly-v1"
	var units, lines []string
	var fence byte
	fenceLength := 0
	oldWeeklyFraming := false
	flush := func() {
		if unit := strings.TrimSpace(strings.Join(lines, "\n")); unit != "" {
			units = append(units, unit)
		}
		lines = nil
	}
	headingOnly := func() bool {
		for _, line := range lines {
			if t := strings.TrimSpace(line); t != "" && !strings.HasPrefix(t, "#") {
				return false
			}
		}
		return true
	}
	all := strings.Split(strings.ReplaceAll(artifact.Content, "\r\n", "\n"), "\n")
	for i, line := range all {
		trimmed := strings.TrimSpace(line)
		if fence != 0 {
			lines = append(lines, line)
			character, length := weeklyFenceRun(trimmed)
			if character == fence && length >= fenceLength && strings.TrimSpace(trimmed[length:]) == "" {
				fence, fenceLength = 0, 0
			}
			continue
		}
		if character, length := weeklyFenceRun(trimmed); character != 0 {
			fence, fenceLength = character, length
			lines = append(lines, line)
			continue
		}
		if generated && generatedWeeklyFraming(trimmed) {
			if strings.HasPrefix(trimmed, "## source ") {
				oldWeeklyFraming = true
			}
			flush()
			continue
		}
		if trimmed == "" {
			if len(lines) > 0 && headingOnly() {
				lines = append(lines, "")
				continue
			}
			// An indented continuation belongs to the preceding bullet, including
			// its separating blank line. It cannot be split to satisfy a budget.
			if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[0]), "- ") && i+1 < len(all) && strings.HasPrefix(all[i+1], "  ") {
				lines = append(lines, "")
				continue
			}
			flush()
			continue
		}
		if strings.HasPrefix(line, "- ") && len(lines) > 0 && !headingOnly() {
			flush()
		}
		lines = append(lines, line)
	}
	flush()
	// L1 and the old recursive weekly format were oldest-first. This adapter's
	// flat output is newest-first; never reverse it again on the next cycle.
	oldWeekly := artifact.SynthesizerID == "deterministic-weekly-v1" && oldWeeklyFraming
	// The old recursive producer hard-cut its final unit with this marker.
	// Do not carry that ambiguous fragment forward as a complete observation.
	// Flat output and authored/fenced ellipses are not blanket-stripped.
	if oldWeekly && len(units) > 0 && strings.HasSuffix(units[len(units)-1], "…") {
		units = units[:len(units)-1]
	}
	if artifact.SynthesizerID == DeterministicL1SynthesizerID || oldWeekly {
		for i, j := 0, len(units)-1; i < j; i, j = i+1, j-1 {
			units[i], units[j] = units[j], units[i]
		}
	}
	return units
}

func weeklyFenceRun(line string) (byte, int) {
	if len(line) < 3 || line[0] != '`' && line[0] != '~' {
		return 0, 0
	}
	length := 1
	for length < len(line) && line[length] == line[0] {
		length++
	}
	if length < 3 {
		return 0, 0
	}
	return line[0], length
}

func generatedWeeklyFraming(line string) bool {
	if strings.HasPrefix(line, "# L1 continuity · ") {
		_, err := time.Parse("2006-01-02", strings.TrimPrefix(line, "# L1 continuity · "))
		return err == nil
	}
	for _, prefix := range []string{"# L2 weekly continuity · ", "# L3 rolling continuity · ", "# Lifetime candidate · "} {
		if strings.HasPrefix(line, prefix) {
			return weeklyPeriod(strings.TrimPrefix(line, prefix))
		}
	}
	if strings.HasPrefix(line, "## source ") {
		id := strings.TrimPrefix(line, "## source ")
		if strings.HasPrefix(id, "L1/") {
			_, err := time.Parse("2006-01-02", strings.TrimPrefix(id, "L1/"))
			return err == nil
		}
		return strings.HasPrefix(id, "L2/") && weeklyPeriod(strings.TrimPrefix(id, "L2/")) || id == "L3/rolling" || id == "lifetime/lifetime"
	}
	return false
}
func weeklyPeriod(value string) bool {
	if len(value) != 8 || value[4:6] != "-W" {
		return false
	}
	if _, err := strconv.Atoi(value[:4]); err != nil {
		return false
	}
	week, err := strconv.Atoi(value[6:])
	return err == nil && week >= 1 && week <= 53
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

// DeterministicLifetimeEligibility consumes only candidate-bound generation
// evidence supplied by the engine from committed L3 provenance.
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
	generations, err := validateL3CandidateEvidence(artifact)
	if err != nil {
		return false, "", err
	}
	if generations < minimum {
		return false, fmt.Sprintf("requires %d canonical L3 generations; observed %d", minimum, generations), nil
	}
	return true, fmt.Sprintf("eligible after %d canonical L3 generations", generations), nil
}
