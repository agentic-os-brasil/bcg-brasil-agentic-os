package zipruntime

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/zipmigration"
)

// Options permits isolated callers without changing the process environment.
type Options struct {
	UserHome  string
	LookupEnv func(string) (string, bool)
}

// Paths keeps managed assets separate from the admitted owner identity.
type Paths struct {
	Mode, Diagnostic                                                      string
	Writable                                                              bool
	ProductRoot, Home, BrainRoot, MetadataRoot, StateRoot, OwnerScopeRoot string
}

func (p Paths) MarshalJSON() ([]byte, error) {
	v := map[string]any{"mode": p.Mode, "writable": p.Writable, "diagnostic": p.Diagnostic}
	if p.Writable {
		v["product_root"], v["brain_root"], v["metadata_root"], v["owner_scope_root"] = p.ProductRoot, p.BrainRoot, p.MetadataRoot, p.OwnerScopeRoot
		v["home"], v["state_root"] = nil, nil
		if p.Home != "" {
			v["home"], v["state_root"] = p.Home, p.StateRoot
		}
	}
	return json.Marshal(v)
}

// ResolvePaths is read-only; neither an absent HOME nor legacy data authorizes bootstrap.
func ResolvePaths(product string, options Options) Paths {
	blocked := func(code string) Paths { return Paths{Mode: "blocked", Diagnostic: code} }
	if options.UserHome == "" {
		var err error
		options.UserHome, err = os.UserHomeDir()
		if err != nil {
			return blocked("user-home-invalid")
		}
	}
	home, err := datahome.Resolve(datahome.Options{UserHome: options.UserHome, ProductRoot: product, LookupEnv: options.LookupEnv})
	if err != nil {
		return blocked(home.Diagnostic)
	}
	// Status also detects pending/revoked receipts and unsafe legacy ancestors.
	legacy := zipmigration.Run(product, zipmigration.Options{Status: true})
	if legacy.State != "not_needed" && legacy.State != "committed" {
		return blocked("migration_required")
	}
	brain, err := os.ReadDir(filepath.Join(product, "brain"))
	if err != nil && !os.IsNotExist(err) {
		return blocked("legacy-inaccessible")
	}
	hasLegacy := len(brain) > 0 || legacy.State == "committed"
	if home.Writable {
		if hasLegacy {
			return blocked("legacy-home-conflict")
		}
		state := filepath.Join(home.Home, "state")
		if err := scheduler.ValidatePrivateDirectory(state); err != nil && !os.IsNotExist(err) {
			return blocked("state-invalid")
		}
		if err := validateHookState(home.BrainRoot); err != nil {
			return blocked("hook-state-unsafe")
		}
		return Paths{Mode: "external", Writable: true, Diagnostic: "ready", ProductRoot: product, Home: home.Home, BrainRoot: home.BrainRoot, MetadataRoot: filepath.Join(home.BrainRoot, ".maestro"), StateRoot: filepath.Join(home.Home, "state"), OwnerScopeRoot: home.Home}
	}
	if home.Diagnostic != "fresh-unconfirmed" {
		return blocked(home.Diagnostic)
	}
	if hasLegacy && home.SelectedBy == "default" {
		if err := validateHookState(filepath.Join(product, "brain")); err != nil {
			return blocked("hook-state-unsafe")
		}
		return Paths{Mode: "legacy", Writable: true, Diagnostic: "legacy-bound", ProductRoot: product, BrainRoot: filepath.Join(product, "brain"), MetadataRoot: filepath.Join(product, "brain", ".maestro"), OwnerScopeRoot: product}
	}
	if hasLegacy {
		return blocked("legacy-home-conflict")
	}
	return Paths{Mode: "initialization_required", Diagnostic: "fresh-unconfirmed"}
}
