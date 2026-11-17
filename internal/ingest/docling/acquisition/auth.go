package acquisition

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const maximumCarrierBytes = 256 << 20
const maximumAttempts = 3

type authenticated struct {
	recipe     Recipe
	descriptor docling.ProfileDescriptor
	registry   releaseverify.AuthorityRegistry
	carrier    map[string][]byte
}

func authenticate(ctx context.Context, o Options, now time.Time) (authenticated, error) {
	var a authenticated
	if err := ctx.Err(); err != nil {
		return a, err
	}
	registryBody, err := readBounded(o.RegistryPath, 1<<20)
	if err != nil || digest(registryBody) != o.RegistrySHA256 {
		return a, errors.New("acquisition_authority_unavailable")
	}
	registry, err := releaseverify.ParseAuthorityRegistry(bytes.NewReader(registryBody), func() time.Time { return now })
	if err != nil {
		return a, errors.New("acquisition_authority_unavailable")
	}
	body, err := readBounded(filepath.Join(o.ReleaseDirectory, releaseverify.ManifestName), 1<<20)
	if err != nil {
		return a, errors.New("acquisition_manifest_untrusted")
	}
	signature, err := readBounded(filepath.Join(o.ReleaseDirectory, releaseverify.ManifestSignatureName), 64)
	if err != nil {
		return a, errors.New("acquisition_manifest_untrusted")
	}
	m, key, err := releaseverify.VerifyManifest(body, signature, registry)
	if err != nil {
		return a, errors.New("acquisition_manifest_untrusted")
	}
	recipeBody, err := readBounded(filepath.Join(o.RecipeDirectory, RecipeName), 1<<20)
	if err != nil {
		return a, errors.New("acquisition_recipe_unavailable")
	}
	recipeSignature, err := readBounded(filepath.Join(o.RecipeDirectory, RecipeSignatureName), 64)
	if err != nil || !ed25519.Verify(key, recipeBody, recipeSignature) {
		return a, errors.New("acquisition_recipe_untrusted")
	}
	var r Recipe
	if err = releasecontract.DecodeStrictJSON(recipeBody, &r); err != nil {
		return a, errors.New("acquisition_recipe_invalid")
	}
	canonical, err := releasecontract.CanonicalJSON(r)
	if err != nil || !bytes.Equal(recipeBody, canonical) {
		return a, errors.New("acquisition_recipe_noncanonical")
	}
	issued, e1 := time.Parse(time.RFC3339, r.IssuedAt)
	expires, e2 := time.Parse(time.RFC3339, r.ExpiresAt)
	if e1 != nil || e2 != nil || !strings.HasSuffix(r.IssuedAt, "Z") || !strings.HasSuffix(r.ExpiresAt, "Z") || issued.After(now) || !expires.After(now) || !expires.After(issued) || expires.Sub(issued) > 14*24*time.Hour {
		return a, errors.New("acquisition_recipe_expired")
	}
	if r.SchemaVersion != 1 || r.Purpose != Purpose || r.Issuer != m.Issuer || r.ManifestSHA256 != digest(body) || r.OS != "windows" || r.Arch != "amd64" || !m.Modern() || m.Channel != "canary" || len(m.Artifacts) != 3 {
		return a, errors.New("acquisition_recipe_binding_mismatch")
	}
	date := strings.TrimPrefix(m.Issuer.ID, "maestro-engineering-pilot-")
	if _, err = time.Parse("20060102", date); err != nil || m.Issuer.ID != "maestro-engineering-pilot-"+date || m.Issuer.KeyID != "pilot-"+date {
		return a, errors.New("acquisition_engineering_authority_required")
	}
	if err = engineeringWindow(registryBody, r, issued, expires); err != nil {
		return a, err
	}
	d, err := docling.DescribeProfile(r.OS, r.Arch, r.Profile)
	if err != nil {
		return a, errors.New("acquisition_profile_unavailable")
	}
	db, err := releasecontract.CanonicalJSON(d)
	inputs := append([]docling.Input{d.Python}, d.Inputs...)
	total := int64(0)
	seen := map[string]bool{}
	for _, input := range inputs {
		if input.Filename == "" || filepath.Base(input.Filename) != input.Filename || strings.ContainsAny(input.Filename, "/\\:") || seen[input.Filename] || !validDigest(input.SHA256) || input.Size <= 0 || input.Size > 256<<20 {
			return a, errors.New("acquisition_profile_invalid")
		}
		if _, err = publicURL(input.URL, input.URL); err != nil {
			return a, err
		}
		seen[input.Filename] = true
		total += input.Size
	}
	if err != nil || digest(db) != r.DescriptorSHA256 || len(inputs) != r.InputCount || total != r.TotalInputBytes || total > 256<<20 {
		return a, errors.New("acquisition_profile_binding_mismatch")
	}
	carrier := map[string][]byte{releaseverify.ManifestName: body, releaseverify.ManifestSignatureName: signature}
	expected := map[string]bool{releaseverify.ManifestName: true, releaseverify.ManifestSignatureName: true, m.ReleaseNotes.Name: true}
	notes, err := readBounded(filepath.Join(o.ReleaseDirectory, m.ReleaseNotes.Name), 8<<20)
	if err != nil || digest(notes) != m.ReleaseNotes.SHA256 {
		return a, errors.New("acquisition_carrier_untrusted")
	}
	carrier[m.ReleaseNotes.Name] = notes
	runtimeCount, serverCount, bundleCount := 0, 0, 0
	carrierBytes := int64(len(body) + len(signature) + len(notes))
	for _, artifact := range m.Artifacts {
		expected[artifact.SignatureRef] = true
		sig, err := readBounded(filepath.Join(o.ReleaseDirectory, artifact.SignatureRef), 64)
		if err != nil || len(sig) != 64 {
			return a, errors.New("acquisition_carrier_untrusted")
		}
		carrier[artifact.SignatureRef] = sig
		if artifact.Kind == "runtime_pack" {
			runtimeCount++
			if artifact != r.RuntimeArtifact || artifact.Adapter != "docling" || artifact.OS != r.OS || artifact.Arch != r.Arch || artifact.Size > d.MaximumArchiveBytes {
				return a, errors.New("acquisition_runtime_binding_mismatch")
			}
			if _, err = os.Lstat(filepath.Join(o.ReleaseDirectory, artifact.Name)); !errors.Is(err, os.ErrNotExist) {
				return a, errors.New("acquisition_runtime_must_be_absent")
			}
			continue
		}
		expected[artifact.Name] = true
		if artifact.Size > maximumCarrierBytes || carrierBytes+artifact.Size > maximumCarrierBytes {
			return a, errors.New("acquisition_carrier_limit")
		}
		artifactBody, err := readBounded(filepath.Join(o.ReleaseDirectory, artifact.Name), artifact.Size)
		if err != nil || int64(len(artifactBody)) != artifact.Size || digest(artifactBody) != artifact.SHA256 || !ed25519.Verify(key, artifactBody, sig) {
			return a, errors.New("acquisition_carrier_untrusted")
		}
		carrier[artifact.Name] = artifactBody
		carrierBytes += artifact.Size
		switch artifact.Kind {
		case "mcp_server":
			serverCount++
			selected, err := readBounded(o.ServerPath, artifact.Size)
			if artifact.OS != r.OS || artifact.Arch != r.Arch || artifact.SHA256 != r.ServerSHA256 || err != nil || !bytes.Equal(selected, artifactBody) {
				return a, errors.New("acquisition_server_binding_mismatch")
			}
		case "bundle":
			bundleCount++
		}
	}
	if runtimeCount != 1 || serverCount != 1 || bundleCount != 1 {
		return a, errors.New("acquisition_closed_carrier_required")
	}
	entries, err := os.ReadDir(o.ReleaseDirectory)
	if err != nil || len(entries) != len(expected) {
		return a, errors.New("acquisition_closed_carrier_required")
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !expected[entry.Name()] {
			return a, errors.New("acquisition_closed_carrier_required")
		}
	}
	return authenticated{r, d, registry, carrier}, nil
}

func reviewAt(ctx context.Context, o Options, reviewed, now time.Time) (Plan, error) {
	var p Plan
	a, err := authenticate(ctx, o, now)
	if err != nil {
		return p, err
	}
	if reviewed.After(now) || now.Sub(reviewed) >= 30*time.Minute {
		return p, errors.New("acquisition_review_expired")
	}
	expires, _ := time.Parse(time.RFC3339, a.recipe.ExpiresAt)
	until := reviewed.Add(30 * time.Minute)
	if expires.Before(until) {
		until = expires
	}
	paths, err := boundPaths(o)
	if err != nil {
		return p, err
	}
	states, attempt, err := cacheState(o.CacheRoot, append([]docling.Input{a.descriptor.Python}, a.descriptor.Inputs...))
	if err != nil {
		return p, err
	}
	retained := int64(0)
	if _, statErr := os.Lstat(o.CacheRoot); statErr == nil {
		retained, err = retainedBytes(o.CacheRoot)
		if err != nil {
			return p, err
		}
	}
	carrierBytes := int64(0)
	for _, body := range a.carrier {
		carrierBytes += int64(len(body))
	}
	additional := 3*a.recipe.TotalInputBytes + a.descriptor.MaximumExpandedBytes + 2*a.descriptor.MaximumArchiveBytes + carrierBytes
	if retained+additional > 2<<30 {
		return p, errors.New("acquisition_retained_working_limit")
	}
	p = Plan{ReviewedAt: reviewed.UTC().Format(time.RFC3339), ValidUntil: until.UTC().Format(time.RFC3339), Recipe: a.recipe, Inputs: states, Attempt: attempt, MaximumAdditionalBytes: additional, RetainedBytes: retained}
	encoded, err := releasecontract.CanonicalJSON(struct {
		Plan  Plan     `json:"plan"`
		Paths []string `json:"paths"`
	}{p, paths})
	if err != nil {
		return Plan{}, err
	}
	p.SHA256 = digest(encoded)
	return p, nil
}
func boundPaths(o Options) ([]string, error) {
	var result []string
	for _, name := range []string{o.ReleaseDirectory, o.RecipeDirectory, o.ServerPath, o.RegistryPath, o.CacheRoot} {
		if !filepath.IsAbs(name) || filepath.Clean(name) != name {
			return nil, errors.New("acquisition_absolute_paths_required")
		}
		result = append(result, name)
	}
	inside := func(parent, child string) bool {
		relative, err := filepath.Rel(parent, child)
		return err == nil && (relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
	}
	if inside(o.ReleaseDirectory, o.RecipeDirectory) || inside(o.RecipeDirectory, o.ReleaseDirectory) {
		return nil, errors.New("acquisition_recipe_requires_separate_directory")
	}
	for _, name := range result[:4] {
		if inside(o.CacheRoot, name) || inside(name, o.CacheRoot) {
			return nil, errors.New("acquisition_cache_requires_separate_directory")
		}
	}
	if err := scheduler.ValidateDirectory(filepath.Dir(o.CacheRoot)); err != nil {
		return nil, errors.New("acquisition_cache_parent_unavailable")
	}
	return result, nil
}

// The canonical registry parser above owns its schema and key validation. This
// mirrors the adapter's finite engineering-pilot scope without importing an
// adapter into core acquisition or granting a new release authority.
func engineeringWindow(body []byte, r Recipe, issued, expires time.Time) error {
	var scope struct {
		Authorities []struct {
			Issuer     string `json:"issuer"`
			KeyID      string `json:"key_id"`
			ValidFrom  string `json:"valid_from"`
			ValidUntil string `json:"valid_until"`
		} `json:"authorities"`
	}
	if err := json.Unmarshal(body, &scope); err != nil || len(scope.Authorities) != 1 {
		return errors.New("acquisition_engineering_authority_required")
	}
	entry := scope.Authorities[0]
	start, e1 := time.Parse(time.RFC3339, entry.ValidFrom)
	end, e2 := time.Parse(time.RFC3339, entry.ValidUntil)
	if e1 != nil || e2 != nil || entry.Issuer != r.Issuer.ID || entry.KeyID != r.Issuer.KeyID || end.Sub(start) > 14*24*time.Hour || issued.Before(start) || expires.After(end) {
		return errors.New("acquisition_engineering_authority_window_mismatch")
	}
	return nil
}
