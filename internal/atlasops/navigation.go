package atlasops

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
)

var ErrNavigationStale = errors.New("authored_navigation_stale")
var ErrNavigationUnavailable = errors.New("authored_navigation_unavailable")

const navigationMaximumBytes = 4 << 20

type NavigationReview struct {
	Sources            []atlas.NavigationSource `json:"sources"`
	ExpectedGeneration string                   `json:"expected_generation"`
	Generation         string                   `json:"generation"`
}
type NavigationReceipt struct {
	State       string    `json:"state"`
	Generation  string    `json:"generation"`
	WorkspaceID string    `json:"workspace_id"`
	PublishedAt time.Time `json:"published_at"`
	SourceCount int       `json:"source_count"`
	PageCount   int       `json:"page_count"`
	Reference   string    `json:"reference"`
}
type navigationGeneration struct {
	Schema      int                      `json:"schema"`
	Workspace   *workspaceScope          `json:"workspace"`
	GeneratedAt time.Time                `json:"generated_at"`
	Provenance  Provenance               `json:"provenance"`
	Navigation  atlas.AuthoredNavigation `json:"navigation"`
	PublishedAt time.Time                `json:"-"`
}
type navigationPointer struct {
	Schema      int             `json:"schema"`
	Workspace   *workspaceScope `json:"workspace"`
	Generation  string          `json:"generation"`
	Digest      string          `json:"digest"`
	PublishedAt time.Time       `json:"published_at"`
}

func (engine *Engine) navigationPointer() (navigationPointer, error) {
	var pointer navigationPointer
	if engine.workspace == nil {
		return pointer, ErrNavigationUnavailable
	}
	if err := engine.validateBinding(); err != nil {
		return pointer, err
	}
	body, err := readAtlasFile(engine.journalDirPath("navigation-current.json"), 16<<10, true)
	if err != nil {
		return pointer, err
	}
	if json.Unmarshal([]byte(body), &pointer) != nil || pointer.Schema != 1 || !validDigest(pointer.Generation) || !validDigest(pointer.Digest) || pointer.PublishedAt.IsZero() {
		return navigationPointer{}, ErrNavigationUnavailable
	}
	if err = engine.validateScope(pointer.Workspace); err != nil {
		return navigationPointer{}, err
	}
	return pointer, nil
}
func (engine *Engine) navigationSources(pages []string, purpose string) ([]atlas.NavigationSource, error) {
	if engine.workspace == nil {
		return nil, ErrNavigationUnavailable
	}
	projection, err := engine.Collect(CollectRequest{Purpose: purpose, Reader: ReaderOwnerSession, Pages: pages})
	if err != nil {
		return nil, err
	}
	sources := make([]atlas.NavigationSource, 0, len(pages))
	for _, page := range projection.Pages {
		sources = append(sources, atlas.NavigationSource{Page: page.Page, Revision: page.Revision, Pointer: page.Pointer, Text: page.Content})
	}
	for _, omission := range projection.Omissions {
		if omission.Reason != "page does not exist" {
			return nil, errors.New("authored_navigation_collection_incomplete")
		}
		sources = append(sources, atlas.NavigationSource{Page: omission.Page, Missing: true})
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].Page < sources[j].Page })
	return sources, nil
}

// ReviewNavigation collects only the named admitted cohort. The envelope binds
// exact source bytes, observed absence and the current generation before review.
func (engine *Engine) ReviewNavigation(pages []string, purpose string) (NavigationReview, error) {
	var review NavigationReview
	sources, err := engine.navigationSources(pages, purpose)
	if err != nil {
		return review, err
	}
	compiled, err := atlas.CompileAuthoredNavigation(engine.workspace.WorkspaceID, sources)
	if err != nil {
		return review, err
	}
	pointer, err := engine.navigationPointer()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return review, err
	}
	return NavigationReview{Sources: sources, ExpectedGeneration: pointer.Generation, Generation: compiled.Generation}, nil
}
func navigationReviewDigest(review NavigationReview) string {
	body, _ := json.Marshal(review)
	return digest(string(body))
}
func navigationPages(sources []atlas.NavigationSource) []string {
	pages := make([]string, 0, len(sources))
	for _, source := range sources {
		pages = append(pages, source.Page)
	}
	return pages
}

// PublishNavigation uses the existing corpus lock and held-parent publisher.
// The immutable complete generation is durable before its tiny current pointer
// changes. A staged artifact is harmless after interruption and never recovered
// or made visible by a read.
func (engine *Engine) PublishNavigation(review NavigationReview, provenance Provenance) (NavigationReceipt, error) {
	var receipt NavigationReceipt
	if provenance.Origin != OriginAttended || provenance.validate() != nil {
		return receipt, errors.New("authored_navigation_requires_attended_review")
	}
	release, err := engine.beginMutation()
	if err != nil {
		return receipt, err
	}
	defer func() { _ = release() }()
	fresh, err := engine.ReviewNavigation(navigationPages(review.Sources), "Revalidate exact reviewed authored navigation")
	if err != nil {
		return receipt, err
	}
	if navigationReviewDigest(fresh) != navigationReviewDigest(review) {
		return receipt, ErrReviewedTargetChanged
	}
	for _, source := range fresh.Sources {
		if err = engine.authorize(provenance, "compile-navigation", source.Page); err != nil {
			return receipt, err
		}
	}
	if fresh.Generation == fresh.ExpectedGeneration {
		generation, err := engine.ReadNavigation(fresh.Generation)
		if err != nil {
			return receipt, err
		}
		return navigationReceipt(generation, StateUnchanged), nil
	}
	compiled, err := atlas.CompileAuthoredNavigation(engine.workspace.WorkspaceID, fresh.Sources)
	if err != nil {
		return receipt, err
	}
	generation := navigationGeneration{Schema: 1, Workspace: engine.workspace, GeneratedAt: engine.now().UTC(), Provenance: provenance, Navigation: compiled}
	body, err := json.Marshal(generation)
	if err != nil || len(body) > navigationMaximumBytes {
		return receipt, ErrNavigationUnavailable
	}
	filename := engine.journalDirPath("navigation-" + compiled.Generation + ".json")
	err = publishAtlasFile(filename, body, true, true)
	if errors.Is(err, os.ErrExist) {
		// An earlier interrupted publication or an explicit reversion can reuse only
		// the exact complete cohort owned by this workspace, never overwrite it.
		existing, readErr := engine.readNavigationGeneration(compiled.Generation, "")
		if readErr != nil {
			return receipt, readErr
		}
		if navigationReviewDigest(NavigationReview{Sources: existing.Navigation.Sources, Generation: existing.Navigation.Generation}) != navigationReviewDigest(NavigationReview{Sources: fresh.Sources, Generation: fresh.Generation}) {
			return receipt, ErrNavigationUnavailable
		}
		generation = existing
		body, err = json.Marshal(generation)
	}
	if err != nil {
		return receipt, err
	}
	// Reuse the owning publication interruption seam, without changing any source.
	if engine.afterPageEffect != nil {
		if err = engine.afterPageEffect(); err != nil {
			return receipt, err
		}
	}
	latest, err := engine.ReviewNavigation(navigationPages(review.Sources), "Fence publication against current authored sources")
	if err != nil {
		return receipt, err
	}
	if navigationReviewDigest(latest) != navigationReviewDigest(review) {
		return receipt, ErrReviewedTargetChanged
	}
	pointer := navigationPointer{Schema: 1, Workspace: engine.workspace, Generation: compiled.Generation, Digest: digest(string(body)), PublishedAt: engine.now().UTC()}
	encoded, _ := json.Marshal(pointer)
	if err = engine.validateBinding(); err != nil {
		return receipt, err
	}
	for _, source := range fresh.Sources {
		if err = engine.authorize(provenance, "compile-navigation", source.Page); err != nil {
			return receipt, err
		}
	}
	if err = publishAtlasFile(engine.journalDirPath("navigation-current.json"), encoded, fresh.ExpectedGeneration == "", true); err != nil {
		return receipt, err
	}
	generation.PublishedAt = pointer.PublishedAt
	return navigationReceipt(generation, StateWritten), nil
}
func navigationReceipt(generation navigationGeneration, state string) NavigationReceipt {
	return NavigationReceipt{State: state, Generation: generation.Navigation.Generation, WorkspaceID: generation.Navigation.WorkspaceID, PublishedAt: generation.PublishedAt, SourceCount: len(generation.Navigation.Sources), PageCount: len(generation.Navigation.Pages), Reference: "index.md"}
}
func (engine *Engine) readNavigationGeneration(id, expectedDigest string) (navigationGeneration, error) {
	var generation navigationGeneration
	if !validDigest(id) {
		return generation, ErrNavigationUnavailable
	}
	body, err := readAtlasFile(engine.journalDirPath("navigation-"+id+".json"), navigationMaximumBytes, true)
	if err != nil {
		return generation, err
	}
	if expectedDigest != "" && digest(string(body)) != expectedDigest {
		return generation, ErrNavigationUnavailable
	}
	if json.Unmarshal([]byte(body), &generation) != nil || generation.Schema != 1 || generation.Navigation.Generation != id || generation.Navigation.WorkspaceID != engine.workspace.WorkspaceID || generation.GeneratedAt.IsZero() {
		return navigationGeneration{}, ErrNavigationUnavailable
	}
	if err = engine.validateScope(generation.Workspace); err != nil {
		return navigationGeneration{}, err
	}
	canonical, err := atlas.CompileAuthoredNavigation(engine.workspace.WorkspaceID, generation.Navigation.Sources)
	if err != nil {
		return navigationGeneration{}, err
	}
	a, _ := json.Marshal(canonical)
	b, _ := json.Marshal(generation.Navigation)
	if string(a) != string(b) {
		return navigationGeneration{}, ErrNavigationUnavailable
	}
	return generation, nil
}

// ReadNavigation never initializes, repairs, promotes a staged generation or
// falls back to stale text. Every current source remains an admission fence.
func (engine *Engine) ReadNavigation(id string) (navigationGeneration, error) {
	pointer, err := engine.navigationPointer()
	if err != nil {
		return navigationGeneration{}, err
	}
	if id != pointer.Generation {
		return navigationGeneration{}, ErrNavigationStale
	}
	generation, err := engine.readNavigationGeneration(id, pointer.Digest)
	if err != nil {
		return navigationGeneration{}, err
	}
	current, err := engine.navigationSources(navigationPages(generation.Navigation.Sources), "Validate visibility of authored navigation")
	if err != nil {
		return navigationGeneration{}, ErrNavigationStale
	}
	compiled, err := atlas.CompileAuthoredNavigation(engine.workspace.WorkspaceID, current)
	if err != nil || compiled.Generation != id {
		return navigationGeneration{}, ErrNavigationStale
	}
	after, err := engine.navigationPointer()
	if err != nil || after.Generation != pointer.Generation || after.Digest != pointer.Digest || !after.PublishedAt.Equal(pointer.PublishedAt) {
		return navigationGeneration{}, ErrNavigationStale
	}
	generation.PublishedAt = pointer.PublishedAt
	return generation, nil
}

// NavigationView hides the private publication envelope from the adapter.
type NavigationProjection struct {
	atlas.AuthoredNavigation
	PublishedAt time.Time `json:"published_at"`
}

func (engine *Engine) NavigationView(id string) (NavigationProjection, error) {
	generation, err := engine.ReadNavigation(id)
	return NavigationProjection{AuthoredNavigation: generation.Navigation, PublishedAt: generation.PublishedAt}, err
}
func (engine *Engine) NavigationStatus() (map[string]any, error) {
	pointer, err := engine.navigationPointer()
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{"state": "absent", "workspace_id": engine.workspace.WorkspaceID}, nil
	}
	if err != nil {
		return nil, err
	}
	generation, err := engine.ReadNavigation(pointer.Generation)
	if errors.Is(err, ErrNavigationStale) {
		return map[string]any{"state": "stale", "workspace_id": engine.workspace.WorkspaceID}, nil
	}
	if err != nil {
		return nil, err
	}
	receipt := navigationReceipt(generation, "ready")
	return map[string]any{"state": receipt.State, "generation": receipt.Generation, "workspace_id": receipt.WorkspaceID, "source_count": receipt.SourceCount, "page_count": receipt.PageCount, "reference": receipt.Reference}, nil
}
func ValidateNavigationPurpose(purpose string) error {
	if strings.TrimSpace(purpose) == "" || len(purpose) > 768 {
		return errors.New("authored_navigation_purpose_invalid")
	}
	return validateAtlasText(purpose)
}
