// Package capabilitybundle validates the source inventory for optional
// professional capability bundles. It does not install, activate or authorize
// a bundle; those are separate release and local-transaction concerns.
package capabilitybundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

const Unavailable = "unavailable"

type Catalog struct {
	SchemaVersion int      `json:"schema_version"`
	Bundles       []Bundle `json:"bundles"`
}

type Bundle struct {
	ID                 string   `json:"id"`
	DisplayName        string   `json:"display_name"`
	Availability       string   `json:"availability"`
	AvailabilityReason string   `json:"availability_reason"`
	DependsOn          []string `json:"depends_on"`
	Tracks             []string `json:"tracks"`
	CatalogPointer     string   `json:"catalog_pointer"`
}

type Plan struct {
	Tracks  []string `json:"tracks"`
	State   string   `json:"state"`
	Bundles []Bundle `json:"bundles"`
	Reason  string   `json:"reason,omitempty"`
}

func Parse(reader io.Reader) (Catalog, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var catalog Catalog
	if err := decoder.Decode(&catalog); err != nil {
		return Catalog{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Catalog{}, errors.New("capability bundle catalog contains multiple JSON values")
		}
		return Catalog{}, err
	}
	return catalog, catalog.Validate()
}

func LoadFile(path string) (Catalog, error) {
	file, err := os.Open(path)
	if err != nil {
		return Catalog{}, err
	}
	defer file.Close()
	return Parse(file)
}

func (catalog Catalog) Validate() error {
	if catalog.SchemaVersion != 1 || len(catalog.Bundles) < 2 {
		return errors.New("capability bundle catalog must use schema version 1 and contain base plus optional bundles")
	}
	byID := make(map[string]Bundle, len(catalog.Bundles))
	trackOwners := make(map[string]string)
	for _, bundle := range catalog.Bundles {
		if !validID(bundle.ID) || strings.TrimSpace(bundle.DisplayName) == "" || bundle.CatalogPointer != "bundles/"+bundle.ID+"/skills/catalog.json" {
			return fmt.Errorf("capability bundle %q has invalid identity or catalog pointer", bundle.ID)
		}
		if _, duplicate := byID[bundle.ID]; duplicate {
			return fmt.Errorf("capability bundle catalog contains duplicate bundle %q", bundle.ID)
		}
		if bundle.ID == "base" {
			if bundle.Availability != "included" || len(bundle.DependsOn) != 0 {
				return errors.New("base capability bundle must be included and have no dependencies")
			}
		} else if bundle.Availability != Unavailable || strings.TrimSpace(bundle.AvailabilityReason) == "" {
			return fmt.Errorf("optional capability bundle %q must remain explicitly unavailable until release activation exists", bundle.ID)
		}
		for _, track := range bundle.Tracks {
			if !validID(track) {
				return fmt.Errorf("capability bundle %q has invalid track %q", bundle.ID, track)
			}
			if owner, exists := trackOwners[track]; exists {
				return fmt.Errorf("capability track %q is claimed by bundles %q and %q", track, owner, bundle.ID)
			}
			trackOwners[track] = bundle.ID
		}
		byID[bundle.ID] = bundle
	}
	if _, ok := byID["base"]; !ok {
		return errors.New("capability bundle catalog must contain base")
	}
	for _, bundle := range catalog.Bundles {
		seenDependencies := map[string]bool{}
		for _, dependency := range bundle.DependsOn {
			if dependency == bundle.ID || !seenDependencies[dependency] && !validID(dependency) {
				return fmt.Errorf("capability bundle %q has invalid dependency %q", bundle.ID, dependency)
			}
			if seenDependencies[dependency] {
				return fmt.Errorf("capability bundle %q has duplicate dependency %q", bundle.ID, dependency)
			}
			seenDependencies[dependency] = true
			if _, ok := byID[dependency]; !ok {
				return fmt.Errorf("capability bundle %q depends on unknown bundle %q", bundle.ID, dependency)
			}
		}
	}
	visitState := make(map[string]uint8, len(byID))
	var visit func(string) error
	visit = func(id string) error {
		switch visitState[id] {
		case 1:
			return fmt.Errorf("capability bundle catalog contains dependency cycle involving %q", id)
		case 2:
			return nil
		}
		visitState[id] = 1
		for _, dependency := range byID[id].DependsOn {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		visitState[id] = 2
		return nil
	}
	for _, bundle := range catalog.Bundles {
		if err := visit(bundle.ID); err != nil {
			return err
		}
	}
	return nil
}

func (catalog Catalog) PlanForTracks(tracks []string) (Plan, error) {
	if err := catalog.Validate(); err != nil {
		return Plan{}, err
	}
	if len(tracks) == 0 {
		return Plan{}, errors.New("at least one capability track is required")
	}
	byID := make(map[string]Bundle, len(catalog.Bundles))
	for _, bundle := range catalog.Bundles {
		byID[bundle.ID] = bundle
	}
	requested := make(map[string]bool, len(tracks))
	for _, track := range tracks {
		if !validID(track) || requested[track] {
			return Plan{}, fmt.Errorf("invalid or duplicate capability track %q", track)
		}
		requested[track] = true
	}
	selected := map[string]bool{"base": true}
	for track := range requested {
		found := false
		for _, bundle := range catalog.Bundles {
			for _, supported := range bundle.Tracks {
				if supported == track {
					selected[bundle.ID] = true
					found = true
				}
			}
		}
		if !found {
			return Plan{}, fmt.Errorf("unknown capability track %q", track)
		}
	}
	var addDependencies func(string)
	addDependencies = func(id string) {
		for _, dependency := range byID[id].DependsOn {
			if !selected[dependency] {
				selected[dependency] = true
				addDependencies(dependency)
			}
		}
	}
	for id := range selected {
		addDependencies(id)
	}
	plan := Plan{State: "base_only"}
	for track := range requested {
		plan.Tracks = append(plan.Tracks, track)
	}
	sort.Strings(plan.Tracks)
	for _, bundle := range catalog.Bundles {
		if selected[bundle.ID] {
			plan.Bundles = append(plan.Bundles, bundle)
			if bundle.Availability == Unavailable {
				plan.State = Unavailable
			}
		}
	}
	sort.Slice(plan.Bundles, func(left, right int) bool { return plan.Bundles[left].ID < plan.Bundles[right].ID })
	if plan.State == Unavailable {
		plan.Reason = "capability selection is inspectable, but optional bundle release identity, compatibility and local activation are not implemented"
	}
	return plan, nil
}

func validID(value string) bool {
	if len(value) < 2 || len(value) > 64 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-') {
			return false
		}
	}
	return true
}
