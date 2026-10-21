// Non-production, read-only metadata collector for issue #430.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"
)

const maxBytes = 1024 * 1024

var policyKeys = []string{"strictKnownMarketplaces", "blockedMarketplaces", "allowManagedHooksOnly", "disableSideloadFlags", "disableCommandPluginSources"}
var envKeys = []string{"DISABLE_AUTOUPDATER", "FORCE_AUTOUPDATE_PLUGINS"}

type observation struct {
	State   string `json:"state"`
	Boolean *bool  `json:"boolean,omitempty"`
	Count   *int   `json:"count,omitempty"`
}

type receipt struct {
	Schema              string                 `json:"schema"`
	ObservedAt          string                 `json:"observed_at"`
	Platform            string                 `json:"platform"`
	Scope               string                 `json:"scope"`
	FileState           string                 `json:"file_state"`
	Settings            map[string]observation `json:"settings"`
	ManagedEnv          map[string]observation `json:"managed_env"`
	ProcessEnv          map[string]observation `json:"collector_process_env"`
	EffectivePolicy     string                 `json:"effective_policy"`
	StandardTier        string                 `json:"standard_tier"`
	SharedFolderVerdict string                 `json:"shared_folder_verdict"`
	UninspectedSources  []string               `json:"uninspected_sources"`
	MarketplaceContents string                 `json:"marketplace_contents"`
}

func defaultPath(goos string) string {
	switch goos {
	case "darwin":
		return "/Library/Application Support/ClaudeCode/managed-settings.json"
	case "windows":
		return `C:\Program Files\ClaudeCode\managed-settings.json`
	case "linux":
		return "/etc/claude-code/managed-settings.json"
	default:
		return ""
	}
}

func collect(path, goos string, lookup func(string) (string, bool)) receipt {
	r := receipt{
		Schema: "maestro-managedsettings430/v1", ObservedAt: time.Now().UTC().Format(time.RFC3339),
		Platform: goos, Scope: "single_file_metadata_only", FileState: "unsupported_platform",
		Settings: map[string]observation{}, ManagedEnv: map[string]observation{}, ProcessEnv: map[string]observation{},
		EffectivePolicy: "uninspected", StandardTier: "unverified", SharedFolderVerdict: "pending_attended_evidence",
		UninspectedSources:  []string{"managed_settings_drop_ins", "mdm", "registry", "server", "parent_process", "status_setting_sources", "other_env_keys"},
		MarketplaceContents: "redacted_and_schema_unvalidated",
	}
	for _, key := range policyKeys {
		r.Settings[key] = observation{State: "unavailable"}
	}
	for _, key := range envKeys {
		r.ManagedEnv[key] = observation{State: "unavailable"}
		value, present := lookup(key)
		r.ProcessEnv[key] = envObservation(value, present)
	}
	if path == "" {
		return r
	}
	info, err := os.Lstat(path)
	if err != nil {
		r.FileState = readFailure(err)
		return r
	}
	if !info.Mode().IsRegular() {
		r.FileState = "not_regular"
		return r
	}
	f, err := os.Open(path)
	if err != nil {
		r.FileState = readFailure(err)
		return r
	}
	body, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	err = errors.Join(err, f.Close())
	if err != nil {
		r.FileState = "unreadable"
		return r
	}
	if len(body) > maxBytes {
		r.FileState = "too_large"
		return r
	}
	var doc map[string]json.RawMessage
	if !json.Valid(body) {
		r.FileState = "invalid_json"
		return r
	}
	if err := json.Unmarshal(body, &doc); err != nil || doc == nil {
		r.FileState = "invalid_document"
		return r
	}
	r.FileState = "read"
	for _, key := range policyKeys {
		value, exists := doc[key]
		state := observation{State: "absent"}
		if exists {
			state.State = "invalid_type"
			switch {
			case string(value) == "null":
				state.State = "null"
			case key == "strictKnownMarketplaces" || key == "blockedMarketplaces":
				var entries []json.RawMessage
				if json.Unmarshal(value, &entries) == nil {
					count := len(entries)
					state.State, state.Count = "nonempty_array", &count
					if count == 0 {
						state.State = "empty_array"
					}
				}
			default:
				var flag bool
				if json.Unmarshal(value, &flag) == nil {
					state.State, state.Boolean = "boolean", &flag
				}
			}
		}
		r.Settings[key] = state
	}
	var managed map[string]json.RawMessage
	value, exists := doc["env"]
	state := "absent"
	if exists {
		state = "invalid_type"
		if string(value) == "null" {
			state = "null"
		} else if json.Unmarshal(value, &managed) == nil {
			state = "object"
		}
	}
	for _, key := range envKeys {
		r.ManagedEnv[key] = observation{State: state}
		if state != "object" {
			continue
		}
		value, exists := managed[key]
		if !exists {
			r.ManagedEnv[key] = observation{State: "absent"}
			continue
		}
		var text string
		if string(value) == "null" {
			r.ManagedEnv[key] = observation{State: "null"}
		} else if json.Unmarshal(value, &text) != nil {
			r.ManagedEnv[key] = observation{State: "invalid_type"}
		} else {
			r.ManagedEnv[key] = envObservation(text, true)
		}
	}
	return r
}

func readFailure(err error) string {
	if errors.Is(err, os.ErrNotExist) {
		return "missing"
	}
	return "unreadable"
}

func envObservation(value string, present bool) observation {
	state := "unrecognized_value"
	switch {
	case !present:
		state = "absent"
	case value == "":
		state = "empty"
	case value == "1" || strings.EqualFold(value, "true"):
		state = "enabled"
	case value == "0" || strings.EqualFold(value, "false"):
		state = "disabled"
	}
	return observation{State: state}
}

func run(args []string, out, diagnostic io.Writer, lookup func(string) (string, bool)) int {
	path := defaultPath(runtime.GOOS)
	if len(args) == 2 && args[0] == "--settings" && args[1] != "" {
		path = args[1]
	} else if len(args) != 0 {
		fmt.Fprintln(diagnostic, "use managedsettings430 [--settings <local-file>]; arguments omitted from diagnostics")
		return 2
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(collect(path, runtime.GOOS, lookup)); err != nil {
		fmt.Fprintln(diagnostic, "receipt output unavailable")
		return 2
	}
	return 0
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.LookupEnv)) }
