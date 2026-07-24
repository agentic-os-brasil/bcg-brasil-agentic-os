// Package profile owns the user-local interaction preference shared by the
// Agentic OS. It is deliberately separate from memory and workspace content.
package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Policy struct {
	SchemaVersion  int          `json:"schema_version"`
	DefaultProfile string       `json:"default_profile"`
	Profiles       []Definition `json:"profiles"`
	Invariants     Invariants   `json:"invariants"`
}

type Definition struct {
	ID            string `json:"id"`
	Communication string `json:"communication"`
	Suggestions   string `json:"suggestions"`
}

type Invariants struct {
	Controls             string `json:"controls"`
	GrantsAuthority      bool   `json:"grants_authority"`
	AllowsRemoteProvider bool   `json:"allows_remote_provider"`
	PersistsInMemory     bool   `json:"persists_in_memory"`
	StoredInWorkspace    bool   `json:"stored_in_workspace"`
}

type State struct {
	Profile string `json:"profile"`
	Source  string `json:"source"`
	Warning string `json:"warning,omitempty"`
}

type Store struct {
	Root   string
	Policy Policy
}

type configuration struct {
	SchemaVersion int    `json:"schema_version"`
	Profile       string `json:"profile"`
}

func Load(reader io.Reader) (Policy, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var policy Policy
	if err := decoder.Decode(&policy); err != nil {
		return Policy{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Policy{}, errors.New("interaction profile policy contains multiple JSON values")
		}
		return Policy{}, err
	}
	if err := policy.Validate(); err != nil {
		return Policy{}, err
	}
	return policy, nil
}

func (policy Policy) Validate() error {
	if policy.SchemaVersion != 1 {
		return fmt.Errorf("unsupported interaction profile policy schema version %d", policy.SchemaVersion)
	}
	if len(policy.Profiles) != 3 || policy.DefaultProfile != "standard" {
		return errors.New("interaction profile policy must define standard, advanced and power with standard as default")
	}
	wanted := []string{"standard", "advanced", "power"}
	for index, id := range wanted {
		definition := policy.Profiles[index]
		if definition.ID != id || definition.Communication == "" || definition.Suggestions == "" {
			return errors.New("interaction profile policy has an invalid profile definition")
		}
	}
	if policy.Invariants.Controls != "progressive_disclosure_and_communication" || policy.Invariants.GrantsAuthority || policy.Invariants.AllowsRemoteProvider || policy.Invariants.PersistsInMemory || policy.Invariants.StoredInWorkspace {
		return errors.New("interaction profile policy weakens a required safety boundary")
	}
	return nil
}

func (policy Policy) Supports(id string) bool {
	for _, definition := range policy.Profiles {
		if definition.ID == id {
			return true
		}
	}
	return false
}

// Get returns the active local setting. Missing or invalid configuration falls
// back to the managed standard profile without mutating user state.
func (store Store) Get() (State, error) {
	if err := store.Policy.Validate(); err != nil {
		return State{}, err
	}
	file, err := os.Open(store.path())
	if errors.Is(err, os.ErrNotExist) {
		return State{Profile: store.Policy.DefaultProfile, Source: "default"}, nil
	}
	if err != nil {
		return State{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var value configuration
	if err := decoder.Decode(&value); err != nil {
		return State{Profile: store.Policy.DefaultProfile, Source: "fallback", Warning: "profile configuration is invalid; using standard"}, nil
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return State{Profile: store.Policy.DefaultProfile, Source: "fallback", Warning: "profile configuration is invalid; using standard"}, nil
	}
	if value.SchemaVersion != 1 || !store.Policy.Supports(value.Profile) {
		return State{Profile: store.Policy.DefaultProfile, Source: "fallback", Warning: "profile configuration is unsupported; using standard"}, nil
	}
	return State{Profile: value.Profile, Source: "configured"}, nil
}

// Ensure writes the default only for a first-time user. Invalid existing state
// remains untouched and visible through Get's fallback warning.
func (store Store) Ensure() (State, error) {
	state, err := store.Get()
	if err != nil || state.Source != "default" {
		return state, err
	}
	return store.write(state.Profile)
}

func (store Store) Set(id string) (State, error) {
	if err := store.Policy.Validate(); err != nil {
		return State{}, err
	}
	if !store.Policy.Supports(id) {
		return State{}, fmt.Errorf("unsupported interaction profile %q; choose standard, advanced or power", id)
	}
	return store.write(id)
}

func (store Store) path() string {
	return filepath.Join(store.Root, "config", "interaction-profile.json")
}

func (store Store) write(id string) (State, error) {
	if err := os.MkdirAll(filepath.Dir(store.path()), 0o700); err != nil {
		return State{}, err
	}
	body, err := json.MarshalIndent(configuration{SchemaVersion: 1, Profile: id}, "", "  ")
	if err != nil {
		return State{}, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(store.path()), ".interaction-profile-*.tmp")
	if err != nil {
		return State{}, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return State{}, err
	}
	if _, err := temporary.Write(append(body, '\n')); err != nil {
		_ = temporary.Close()
		return State{}, err
	}
	if err := temporary.Close(); err != nil {
		return State{}, err
	}
	if err := os.Rename(temporaryPath, store.path()); err != nil {
		return State{}, err
	}
	return State{Profile: id, Source: "configured"}, nil
}
