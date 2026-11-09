package setup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

// ClientCommand is trusted process composition, not a model/JSON callback.
// A test composition is labelled engineering; production uses bounded exec.
type ClientCommand func(context.Context, string, []string, string) ([]byte, error)
type CodeReview struct {
	Observation     installer.CodeObservation `json:"observation"`
	BeforePresent   bool                      `json:"before_present"`
	BeforeEnabled   bool                      `json:"before_enabled"`
	ExpectedVersion string                    `json:"expected_version"`
	Actions         [][]string                `json:"actions"`
	Digest          string                    `json:"review_digest"`
}
type pluginRow struct {
	ID             string            `json:"id"`
	Version        string            `json:"version"`
	Scope          string            `json:"scope"`
	Enabled        bool              `json:"enabled"`
	InstallPath    string            `json:"installPath"`
	FolderVersion  string            `json:"folderVersion"`
	ReadFromFolder bool              `json:"readFromFolder"`
	Errors         []json.RawMessage `json:"errors"`
	Notes          []json.RawMessage `json:"notes"`
	fields         map[string]json.RawMessage
}

func (p *pluginRow) UnmarshalJSON(body []byte) error {
	// Native readback is observation, not a default-filled desired state.
	// In particular absent enabled must never mean observed disabled.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return err
	}
	for _, name := range []string{"id"} {
		value, ok := fields[name]
		if !ok || bytes.Equal(value, []byte("null")) {
			return errors.New("Code readback lacks explicit " + name)
		}
	}
	type plain pluginRow
	if err := json.Unmarshal(body, (*plain)(p)); err != nil {
		return err
	}
	p.fields = fields
	return nil
}

type marketplaceRow struct {
	Name   string          `json:"name"`
	Source json.RawMessage `json:"source"`
	Path   string          `json:"path"`
}

func clientCommand(ctx context.Context, path string, args []string, cwd string) ([]byte, error) {
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(bounded, path, args...)
	cmd.Dir = cwd
	cmd.WaitDelay = 2 * time.Second
	var stdout boundedClientOutput
	cmd.Stdout = &stdout
	cmd.Stderr = &boundedClientOutput{}
	err := cmd.Run()
	return stdout.Bytes(), err
}

type boundedClientOutput struct{ bytes.Buffer }

func (b *boundedClientOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 1<<20 {
		return 0, errors.New("client output exceeds bound")
	}
	return b.Buffer.Write(p)
}
func codeVersion(body []byte) (string, error) {
	fields := strings.Fields(string(body))
	if len(fields) == 0 {
		return "", errors.New("Code version unavailable")
	}
	v := fields[0]
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return "", errors.New("unsupported Code version")
	}
	var n [3]int
	for i, p := range parts {
		x, e := strconv.Atoi(p)
		if e != nil || x < 0 || strconv.Itoa(x) != p {
			return "", errors.New("unsupported Code version")
		}
		n[i] = x
	}
	if n[0] < 2 || n[0] == 2 && (n[1] < 1 || n[1] == 1 && n[2] < 289) {
		return "", errors.New("Code JSON/in-place readback requires at least 2.1.289")
	}
	return v, nil
}
func jsonClient(body []byte, out any) error {
	if len(body) > 64<<10 {
		return errors.New("client JSON bound")
	}
	return json.Unmarshal(body, out)
}
func observeCode(ctx context.Context, r installer.ClientRecord, run ClientCommand) (installer.CodeObservation, bool, bool, error) {
	o := r.Code
	o.State = "registration_pending"
	o.EvidenceClass = "supported_cli_readback"
	if run != nil {
		o.EvidenceClass = "engineering_process_witness"
	} else {
		run = clientCommand
	}
	if !filepath.IsAbs(o.Executable) || filepath.Clean(o.Executable) != o.Executable {
		return o, false, false, errors.New("explicit absolute Code executable required")
	}
	sha, target, err := packaging.InspectNativeExecutable(o.Executable)
	if err != nil {
		return o, false, false, err
	}
	if target.OS != runtime.GOOS || target.Arch != runtime.GOARCH {
		return o, false, false, errors.New("Code executable target mismatch")
	}
	o.ExecutableSHA256 = sha
	body, err := run(ctx, o.Executable, []string{"--version"}, r.Binding.Workspace)
	if err != nil {
		return o, false, false, err
	}
	o.Version, err = codeVersion(body)
	if err != nil {
		return o, false, false, err
	}
	for _, args := range [][]string{{"plugin", "list", "--help"}, {"plugin", "install", "--help"}, {"plugin", "update", "--help"}, {"plugin", "disable", "--help"}, {"plugin", "marketplace", "add", "--help"}, {"plugin", "marketplace", "list", "--help"}, {"plugin", "marketplace", "update", "--help"}} {
		body, err = run(ctx, o.Executable, args, r.Binding.Workspace)
		if err != nil || !bytes.Contains(body, []byte("--json")) {
			return o, false, false, errors.New("supported Code JSON command capability unavailable")
		}
	}
	body, err = run(ctx, o.Executable, []string{"plugin", "marketplace", "list", "--json"}, r.Binding.Workspace)
	if err != nil {
		return o, false, false, err
	}
	var marketplaces []marketplaceRow
	if err = jsonClient(body, &marketplaces); err != nil {
		return o, false, false, err
	}
	marketPresent := false
	for _, m := range marketplaces {
		if m.Name != o.Marketplace {
			continue
		}
		if marketPresent {
			return o, false, false, errors.New("ambiguous marketplace identity")
		}
		marketPresent = true
		var source string
		_ = json.Unmarshal(m.Source, &source)
		if source != "directory" && source != "file" && source != "local" {
			return o, false, false, errors.New("foreign marketplace source; preserved unchanged")
		}
		if m.Path != r.Materialization.CodeRoot {
			return o, false, false, errors.New("foreign marketplace path; preserved unchanged")
		}
		o.SourcePath = m.Path
	}
	body, err = run(ctx, o.Executable, []string{"plugin", "list", "--json"}, r.Binding.Workspace)
	if err != nil {
		return o, false, false, err
	}
	var plugins []pluginRow
	if err = jsonClient(body, &plugins); err != nil {
		return o, false, false, err
	}
	present := false
	for _, p := range plugins {
		if p.ID != o.PluginID {
			continue
		}
		// Cached unrelated plugins need no local-in-place fields. Require all
		// explicit observations only after selecting the exact Maestro identity.
		for _, name := range []string{"version", "scope", "enabled", "installPath", "folderVersion", "readFromFolder"} {
			value, ok := p.fields[name]
			if !ok || bytes.Equal(value, []byte("null")) {
				return o, false, false, errors.New("Code readback lacks explicit " + name)
			}
		}
		if present || p.Scope != o.Scope {
			return o, false, false, errors.New("ambiguous plugin scope; preserved unchanged")
		}
		present = true
		expected := filepath.Join(r.Materialization.CodeRoot, "plugins", "maestro-product")
		if p.InstallPath != expected || !p.ReadFromFolder {
			return o, false, false, errors.New("plugin does not read expected local source")
		}
		o.InstallPath = p.InstallPath
		o.InstalledVersion = p.Version
		o.FolderVersion = p.FolderVersion
		o.ReadFromFolder = p.ReadFromFolder
		o.Enabled = p.Enabled
		o.Errors = len(p.Errors)
		o.Notes = len(p.Notes)
		if o.Errors == 0 && o.FolderVersion == r.Materialization.Release {
			o.State = "registered"
		}
	}
	if present && !marketPresent {
		return o, false, false, errors.New("plugin marketplace ownership unavailable")
	}
	return o, marketPresent, present, nil
}
func ReviewCodeRegistration(ctx context.Context, r installer.ClientRecord, run ClientCommand) (CodeReview, error) {
	o, market, present, err := observeCode(ctx, r, run)
	if err != nil {
		return CodeReview{}, err
	}
	review := CodeReview{Observation: o, BeforePresent: present, BeforeEnabled: o.Enabled, ExpectedVersion: r.Materialization.Release, Actions: [][]string{}}
	if !market {
		review.Actions = append(review.Actions, []string{"plugin", "marketplace", "add", r.Materialization.CodeRoot, "--scope", o.Scope, "--json"})
	}
	if !present {
		review.Actions = append(review.Actions, []string{"plugin", "install", o.PluginID, "--scope", o.Scope, "--json"})
	} else if o.State != "registered" {
		review.Actions = append(review.Actions, []string{"plugin", "marketplace", "update", o.Marketplace, "--json"}, []string{"plugin", "update", o.PluginID, "--scope", o.Scope, "--json"})
	}
	if present && !o.Enabled && len(review.Actions) > 0 {
		review.Actions = append(review.Actions, []string{"plugin", "disable", o.PluginID, "--scope", o.Scope, "--json"})
	}
	body, err := releasecontract.CanonicalJSON(review)
	if err != nil {
		return review, err
	}
	review.Digest = fmt.Sprintf("%x", sha256.Sum256(body))
	return review, nil
}

// checkReviewedCodeIdentity observes the selected executable around each
// supported effect. It cannot make path-based process execution atomic; drift
// observed after effects requires explicit reconciliation of actual client state.
func checkReviewedCodeIdentity(ctx context.Context, reviewed installer.CodeObservation, run ClientCommand, cwd string) error {
	hash, target, err := packaging.InspectNativeExecutable(reviewed.Executable)
	if err != nil {
		return err
	}
	if hash != reviewed.ExecutableSHA256 || target.OS != runtime.GOOS || target.Arch != runtime.GOARCH {
		return errors.New("reviewed Code executable bytes changed")
	}
	body, err := run(ctx, reviewed.Executable, []string{"--version"}, cwd)
	if err != nil {
		return err
	}
	version, err := codeVersion(body)
	if err != nil {
		return err
	}
	if version != reviewed.Version {
		return errors.New("reviewed Code executable version changed")
	}
	after, _, err := packaging.InspectNativeExecutable(reviewed.Executable)
	if err != nil {
		return err
	}
	if after != reviewed.ExecutableSHA256 {
		return errors.New("reviewed Code executable changed during version observation")
	}
	return nil
}
func ApplyCodeRegistration(ctx context.Context, r installer.ClientRecord, run ClientCommand, digest string, confirmed bool) (installer.CodeObservation, error) {
	if !confirmed {
		return installer.CodeObservation{}, errors.New("explicit Code registration confirmation required")
	}
	review, err := ReviewCodeRegistration(ctx, r, run)
	if err != nil {
		return installer.CodeObservation{}, err
	}
	if review.Digest != digest {
		return installer.CodeObservation{}, errors.New("Code registration review changed")
	}
	execute := run
	if execute == nil {
		execute = clientCommand
	}
	effects := 0
	drift := func(observation installer.CodeObservation, cause error) (installer.CodeObservation, error) {
		observation.State = "registration_pending"
		if effects > 0 {
			return observation, fmt.Errorf("Code registration partial after %d supported command attempts; reconcile actual client state: %w", effects, cause)
		}
		return observation, fmt.Errorf("Code registration stopped before supported command attempts: %w", cause)
	}
	for _, args := range review.Actions {
		if err := checkReviewedCodeIdentity(ctx, review.Observation, execute, r.Binding.Workspace); err != nil {
			return drift(review.Observation, err)
		}
		body, e := execute(ctx, r.Code.Executable, args, r.Binding.Workspace)
		effects++
		// Result JSON is advisory. Actual supported list readback is the oracle,
		// including documented nonzero already-disabled outcomes.
		if e != nil && !(len(args) > 1 && args[1] == "disable") {
			return installer.CodeObservation{}, fmt.Errorf("Code command incomplete; reconcile actual lists: %w", e)
		}
		if len(body) > 1<<20 {
			return installer.CodeObservation{}, errors.New("Code mutation output bound")
		}
	}
	observation, _, present, err := observeCode(ctx, r, run)
	if err != nil {
		return observation, err
	}
	if observation.ExecutableSHA256 != review.Observation.ExecutableSHA256 || observation.Version != review.Observation.Version {
		return drift(observation, errors.New("final Code executable identity differs from reviewed hash/version"))
	}
	if err := checkReviewedCodeIdentity(ctx, review.Observation, execute, r.Binding.Workspace); err != nil {
		return drift(observation, err)
	}
	if !present || observation.State != "registered" || review.BeforePresent && !review.BeforeEnabled && observation.Enabled {
		return observation, errors.New("Code registration readback pending")
	}
	return observation, nil
}
