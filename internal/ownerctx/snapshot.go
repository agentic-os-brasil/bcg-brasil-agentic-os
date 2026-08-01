package ownerctx

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var ErrSnapshotStale = errors.New("UserSelfSnapshot is stale relative to canonical owner facets")

// UserSelfSnapshot is a read-only projection of owner facets. It is never an
// authority: every load recomputes the source digest and rejects stale data.
type UserSelfSnapshot struct {
	SchemaVersion         int                      `json:"schema_version"`
	Version               string                   `json:"version"`
	ProjectionOf          string                   `json:"projection_of"`
	CanonicalSourceDigest string                   `json:"canonical_source_digest"`
	GeneratedAt           time.Time                `json:"generated_at"`
	Facets                map[string]SnapshotFacet `json:"facets"`
}

type SnapshotFacet struct {
	Facet         string   `json:"facet"`
	SourcePath    string   `json:"source_path"`
	ContentDigest string   `json:"content_digest"`
	Sensitivity   string   `json:"sensitivity"`
	Readers       []string `json:"readers"`
	Refinement    string   `json:"refinement"`
	Content       string   `json:"content,omitempty"`
}

func (snapshot UserSelfSnapshot) Validate() error {
	return validateSnapshot(snapshot)
}

// ProjectSnapshot reads canonical facet files and returns a deterministic,
// versioned projection. It does not mutate the canonical files.
func ProjectSnapshot(root string, requested []string) (UserSelfSnapshot, error) {
	registryValue, err := readRegistry(root)
	if err != nil {
		return UserSelfSnapshot{}, err
	}
	ids := append([]string(nil), requested...)
	if len(ids) == 0 {
		for id := range registryValue.Facets {
			ids = append(ids, id)
		}
		sort.Strings(ids)
	}
	sort.Strings(ids)
	facets := make(map[string]SnapshotFacet, len(ids))
	var source strings.Builder
	for _, id := range ids {
		definition, ok := registryValue.Facets[id]
		if !ok {
			return UserSelfSnapshot{}, errors.New("requested self facet is not registered")
		}
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(definition.Path)))
		if err != nil {
			return UserSelfSnapshot{}, err
		}
		// Never truncate the authority. A facet larger than the projection
		// budget fails closed so mutations after a retained prefix cannot evade
		// freshness checks.
		if len(body) > maximumOwnerProjectionBytes {
			return UserSelfSnapshot{}, errors.New("owner self facet exceeds projection bound")
		}
		content := string(body)
		readers := append([]string(nil), definition.Readers...)
		sort.Strings(readers)
		facet := SnapshotFacet{Facet: id, SourcePath: definition.Path, Sensitivity: definition.Sensitivity, Readers: readers, Refinement: definition.Refinement, Content: content}
		facet.ContentDigest = snapshotFacetDigest(facet)
		facets[id] = facet
		source.WriteString(id)
		source.WriteByte('\x00')
		source.WriteString(facet.ContentDigest)
		source.WriteByte('\x00')
	}
	canonical := digest(source.String())
	return UserSelfSnapshot{
		SchemaVersion: 1, Version: "self-" + canonical[:16], ProjectionOf: "ownerctx.facets.v2",
		CanonicalSourceDigest: canonical, GeneratedAt: time.Now().UTC(), Facets: facets,
	}, nil
}

const maximumOwnerProjectionBytes = 32 << 10

func PersistSnapshot(root string, snapshot UserSelfSnapshot) error {
	if err := validateSnapshot(snapshot); err != nil {
		return err
	}
	current, err := ProjectSnapshot(root, mapKeys(snapshot.Facets))
	if err != nil {
		return err
	}
	if !sameSnapshotBinding(current, snapshot) {
		return ErrSnapshotStale
	}
	path := filepath.Join(root, "owner", "self", "projections", snapshot.Version+".json")
	body, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	return atomicPrivateWrite(path, append(body, '\n'))
}

func LoadSnapshot(root, version string) (UserSelfSnapshot, error) {
	if strings.TrimSpace(version) == "" {
		return UserSelfSnapshot{}, errors.New("self snapshot version is required")
	}
	path := filepath.Join(root, "owner", "self", "projections", version+".json")
	body, err := os.ReadFile(path)
	if err != nil {
		return UserSelfSnapshot{}, err
	}
	var snapshot UserSelfSnapshot
	if err := json.Unmarshal(body, &snapshot); err != nil {
		return UserSelfSnapshot{}, err
	}
	if err := validateSnapshot(snapshot); err != nil {
		return UserSelfSnapshot{}, err
	}
	current, err := ProjectSnapshot(root, mapKeys(snapshot.Facets))
	if err != nil {
		return UserSelfSnapshot{}, err
	}
	if !sameSnapshotBinding(current, snapshot) {
		return UserSelfSnapshot{}, ErrSnapshotStale
	}
	return snapshot, nil
}

func DeleteSnapshot(root, version string, confirmed bool) error {
	if !confirmed {
		return ErrConfirmationRequired
	}
	if strings.TrimSpace(version) == "" {
		return errors.New("self snapshot version is required")
	}
	return os.Remove(filepath.Join(root, "owner", "self", "projections", version+".json"))
}

func validateSnapshot(snapshot UserSelfSnapshot) error {
	if snapshot.SchemaVersion != 1 || snapshot.ProjectionOf != "ownerctx.facets.v2" || !strings.HasPrefix(snapshot.Version, "self-") || len(snapshot.CanonicalSourceDigest) != 64 || snapshot.Version != "self-"+snapshot.CanonicalSourceDigest[:16] || snapshot.GeneratedAt.IsZero() || len(snapshot.Facets) == 0 {
		return errors.New("UserSelfSnapshot is invalid")
	}
	if !validDigest(snapshot.CanonicalSourceDigest) {
		return errors.New("UserSelfSnapshot source digest is invalid")
	}
	for id, facet := range snapshot.Facets {
		if id == "" || facet.Facet != id || facet.SourcePath == "" || !validDigest(facet.ContentDigest) || facet.ContentDigest != snapshotFacetDigest(facet) {
			return errors.New("UserSelfSnapshot facet binding is invalid")
		}
	}
	return nil
}

func sameSnapshotBinding(current, stored UserSelfSnapshot) bool {
	if current.CanonicalSourceDigest != stored.CanonicalSourceDigest || current.Version != stored.Version || len(current.Facets) != len(stored.Facets) {
		return false
	}
	for id, facet := range current.Facets {
		other, ok := stored.Facets[id]
		if !ok || facet.SourcePath != other.SourcePath || facet.ContentDigest != other.ContentDigest || facet.Sensitivity != other.Sensitivity || facet.Refinement != other.Refinement || facet.Content != other.Content || !sameStrings(facet.Readers, other.Readers) {
			return false
		}
	}
	return true
}

func snapshotFacetDigest(facet SnapshotFacet) string {
	body, _ := json.Marshal(struct {
		Facet       string   `json:"facet"`
		SourcePath  string   `json:"source_path"`
		Sensitivity string   `json:"sensitivity"`
		Readers     []string `json:"readers"`
		Refinement  string   `json:"refinement"`
		Content     string   `json:"content"`
	}{facet.Facet, facet.SourcePath, facet.Sensitivity, append([]string(nil), facet.Readers...), facet.Refinement, facet.Content})
	return digest(string(body))
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func mapKeys(values map[string]SnapshotFacet) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
