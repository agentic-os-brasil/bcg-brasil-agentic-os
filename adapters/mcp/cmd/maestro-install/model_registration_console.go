package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/setup"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
)

// Registration prepares a reviewed proposal. Qualification and enrollment stay
// separate original maintenance transitions, each requiring its own review.
func manageClientModelRegistration(ctx context.Context, o installer.ClientOptions, prompt func(string) (string, error), confirm func(any) error, output io.Writer) error {
	job, err := prompt(fmt.Sprintf("Model routine: choose %s or %s: ", maintenance.MemoryModelJobID, maintenance.YodaSelfReviewWeeklyJobID))
	if err != nil {
		return err
	}
	if job != maintenance.MemoryModelJobID && job != maintenance.YodaSelfReviewWeeklyJobID {
		return errors.New("choose one listed model routine; nothing registered")
	}
	selection := setup.ModelRegistrationSelection{JobID: job}
	selection.Provider, err = prompt("Choose the provider for this native Code session: ")
	if err != nil {
		return err
	}
	selection.Model, err = prompt("Choose the model identifier for this routine: ")
	if err != nil {
		return err
	}
	if selection.Provider == "" || selection.Model == "" {
		return errors.New("an explicit provider and model are required")
	}
	if job == maintenance.YodaSelfReviewWeeklyJobID {
		selection.OwnerPolicy, err = selectModelOwnerPolicy(ctx, o, prompt, output)
		if err != nil {
			return err
		}
	}
	if err := prepareClientModelProfile(ctx, o, prompt, confirm, output, false); err != nil {
		return err
	}
	review, err := setup.ReviewModelRegistration(ctx, o, selection)
	if err != nil {
		return err
	}
	if err = confirm(review); err != nil {
		return err
	}
	result, err := setup.ApplyModelRegistration(ctx, o, review, review.Digest, true)
	if err != nil {
		return err
	}
	if err = json.NewEncoder(output).Encode(result); err != nil {
		return err
	}
	_, err = fmt.Fprintln(output, "Registered inactive and unqualified. Use maintenance qualify, then separately enroll after reviewing the qualification and schedule. Native wake is a separate opt-in.")
	return err
}

func selectModelOwnerPolicy(ctx context.Context, o installer.ClientOptions, prompt func(string) (string, error), output io.Writer) (ownerctx.WeeklySelfOptions, error) {
	var policy ownerctx.WeeklySelfOptions
	status, err := installer.ReadClientsSeeded(ctx, o)
	if err != nil {
		return policy, err
	}
	if status.Record == nil || status.State != "current_materialization" {
		return policy, errors.New("current admitted client required for owner-policy selection")
	}
	ownerRoot := filepath.Join(status.Record.Binding.Home, "brain")
	accepted, err := ownerctx.ProjectAcceptedSessionContext(ownerRoot)
	if err != nil {
		return policy, err
	}
	boundOwner, err := ownerctx.ReadWeeklySelfOwnerID(ownerRoot)
	if err != nil {
		return policy, err
	}
	policy.OwnerID = boundOwner
	if policy.OwnerID == "" {
		policy.OwnerID, err = prompt("Choose a new label for personal learning history (no existing owner label is bound): ")
		if err != nil {
			return policy, err
		}
	}
	policy.WorkingLanguage, err = prompt("Choose the working language already used by these facets (for example en): ")
	if err != nil {
		return policy, err
	}
	policy.Limits = ownerctx.DefaultWeeklySelfLimits()
	policy.TranslatorID, policy.TranslatorVersion = "identity", "1"
	acceptedIDs := make(map[string]bool, len(accepted.Facets))
	acceptedNames := make([]string, 0, len(accepted.Facets))
	for _, facet := range accepted.Facets {
		acceptedIDs[facet.Facet] = true
		acceptedNames = append(acceptedNames, facet.Facet)
	}
	sort.Strings(acceptedNames)
	snapshot, err := ownerctx.ProjectSnapshot(ownerRoot, acceptedNames)
	if err != nil {
		return policy, err
	}
	allowed := make(map[string]bool)
	var names []string
	for _, facet := range snapshot.Facets {
		yodaReader := false
		for _, reader := range facet.Readers {
			if reader == "yoda" {
				yodaReader = true
			}
		}
		if !acceptedIDs[facet.Facet] || facet.Sensitivity == "sensitive" || !yodaReader {
			continue
		}
		candidate := policy
		candidate.Facets = []string{facet.Facet}
		if ownerctx.ValidateWeeklySelfPolicy(candidate) == nil {
			allowed[facet.Facet] = true
			names = append(names, facet.Facet)
		}
	}
	if len(names) == 0 {
		return policy, errors.New("no accepted non-sensitive facet matches this language and the original weekly policy; finish or review onboarding first")
	}
	sort.Strings(names)
	if err = json.NewEncoder(output).Encode(struct {
		AcceptedFacets []string `json:"accepted_facets"`
	}{names}); err != nil {
		return policy, err
	}
	answer, err := prompt("Choose accepted facet names from the list, separated by commas: ")
	if err != nil {
		return policy, err
	}
	seen := make(map[string]bool)
	for _, item := range strings.Split(answer, ",") {
		name := strings.TrimSpace(item)
		if name == "" || !allowed[name] || seen[name] {
			return policy, errors.New("choose distinct facets from the actual accepted non-sensitive list")
		}
		seen[name] = true
		policy.Facets = append(policy.Facets, name)
	}
	sort.Strings(policy.Facets)
	// Leave room for labels and provenance in the existing 4096-byte consumer.
	// The entire bounded policy is shown in the subsequent registration review.
	if bound := 2048 / len(policy.Facets); bound < policy.Limits.MaxWorkingFacetBytes {
		policy.Limits.MaxWorkingFacetBytes = bound
	}
	if err = ownerctx.ValidateWeeklySelfPolicy(policy); err != nil {
		return policy, err
	}
	current, err := ownerctx.ProjectAcceptedSessionContext(ownerRoot)
	if err != nil {
		return policy, err
	}
	currentOwner, err := ownerctx.ReadWeeklySelfOwnerID(ownerRoot)
	if err != nil {
		return policy, err
	}
	if accepted.Generation != current.Generation || boundOwner != currentOwner {
		return policy, errors.New("accepted owner context or history label changed; repeat the policy selection")
	}
	return policy, nil
}

// Only profile preparation/login is shared. Foreground calls and maintenance
// registration retain their separate exact owner reviews and effect owners.
func prepareClientModelProfile(ctx context.Context, o installer.ClientOptions, prompt func(string) (string, error), confirm func(any) error, output io.Writer, foreground bool) error {
	reviewProfile, applyProfile, inspectProfile := setup.ReviewModelProfile, setup.ApplyModelProfile, setup.InspectModelProfile
	reviewLogin, loginProfile := setup.ReviewModelProfileLogin, setup.LoginModelProfile
	if foreground {
		reviewProfile, applyProfile, inspectProfile = setup.ReviewForegroundProfile, setup.ApplyForegroundProfile, setup.InspectForegroundProfile
		reviewLogin, loginProfile = setup.ReviewForegroundProfileLogin, setup.LoginForegroundProfile
	}
	profile, err := reviewProfile(ctx, o)
	if err != nil {
		return err
	}
	if !profile.Prepared {
		if err = confirm(profile); err != nil {
			return err
		}
		prepared, err := applyProfile(ctx, o, profile, profile.Digest, true)
		if err != nil {
			return err
		}
		if err = json.NewEncoder(output).Encode(prepared); err != nil {
			return err
		}
	}
	status, err := inspectProfile(ctx, o)
	if err != nil {
		return err
	}
	if status.Authentication != "authenticated" {
		login, err := reviewLogin(ctx, o)
		if err != nil {
			return err
		}
		if err = confirm(login); err != nil {
			return err
		}
		status, err = loginProfile(ctx, o, login, login.Digest, true)
		if err != nil {
			return err
		}
		if status.Authentication != "authenticated" {
			return setup.ErrModelProfileLoginRequired
		}
	}

	return nil
}
func manageClientForegroundProfile(ctx context.Context, o installer.ClientOptions, prompt func(string) (string, error), confirm func(any) error, output io.Writer) error {
	if err := prepareClientModelProfile(ctx, o, prompt, confirm, output, true); err != nil {
		return err
	}
	_, err := fmt.Fprintln(output, "Foreground profile prepared and authenticated. Ask Maestro for agents.status, then announce and request one agents.run with an active role, selected context and explicit provider/model. Each call requires its own review, uses no tools and grants no scheduled work.")
	return err
}
