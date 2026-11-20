package product

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionhook"
)

// weeklySessionWorkingText is advisory data only. A consumer must match the
// returned canonical generation to its own independently accepted session view.
func (b *Bound) weeklySessionWorkingText() (text, generation, canonicalGeneration, status string) {
	service, err := b.maintenanceService()
	if err != nil {
		return "", "", "", "unavailable"
	}
	view, err := service.ReadWeeklySessionWorking()
	if err != nil || view.State != "available" || b.validate() != nil {
		return "", "", "", "unavailable"
	}
	return view.Text, view.Generation, view.CanonicalGeneration, view.State
}

// mergeWeeklySessionWorking keeps legacy context byte-for-byte when optional
// data is absent, refused, from another accepted generation, or out of budget.
func mergeWeeklySessionWorking(body, canonical, text, derived, accepted, status string, maximum int) (string, string, bool) {
	if status != "available" || text == "" || derived == "" || accepted != canonical || len(body)+1+len(text) > maximum {
		return body, canonical, false
	}
	sum := sha256.Sum256([]byte("owner-session-with-derived-yoda-v1\x00" + canonical + "\x00" + derived))
	return body + "\n" + text, hex.EncodeToString(sum[:]), true
}

// appendWeeklySessionHook preserves the original event, instructions and memory;
// optional working data cannot displace them or increase either existing cap.
func appendWeeklySessionHook(output sessionhook.ClaudeOutput, event, text, derived, accepted, canonical, status string) sessionhook.ClaudeOutput {
	if event != "SessionStart" || output.HookSpecificOutput.HookEventName != "SessionStart" {
		return output
	}
	merged, _, applied := mergeWeeklySessionWorking(output.HookSpecificOutput.AdditionalContext, canonical, text, derived, accepted, status, sessionhook.MaximumAdditionalContextBytes)
	if !applied {
		return output
	}
	candidate := output
	candidate.HookSpecificOutput.AdditionalContext = merged
	body, err := json.Marshal(candidate)
	if err != nil || len(body) > 24<<10 {
		return output
	}
	return candidate
}
func (b *Bound) withWeeklySessionHook(output sessionhook.ClaudeOutput, event string) sessionhook.ClaudeOutput {
	if event != "SessionStart" {
		return output
	}
	accepted, err := ownerctx.ProjectAcceptedSessionContext(b.ownerRoot())
	if err != nil {
		return output
	}
	text, derived, canonical, status := b.weeklySessionWorkingText()
	return appendWeeklySessionHook(output, event, text, derived, canonical, accepted.Generation, status)
}
