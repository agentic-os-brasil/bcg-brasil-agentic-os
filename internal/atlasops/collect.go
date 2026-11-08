package atlasops

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Reader names who the projection is for. The tiers come from the accepted
// owner atlas contract: the owner sees their own corpus, Maestro and Yoda
// receive bounded task-relevant projections, and a delegated agent gets an
// attenuated excerpt or a pointer and only when explicitly authorized.
type Reader string

const (
	ReaderOwnerSession Reader = "owner_session"
	ReaderMaestro      Reader = "maestro"
	ReaderYoda         Reader = "yoda"
	ReaderDelegate     Reader = "delegate"
)

const (
	maximumCollectPages = 24
	maximumCollectBytes = 128 << 10
)

// CollectRequest is always explicit about three things: why the content is
// wanted, who will read it, and which pages. There is no request shape that
// means "everything".
type CollectRequest struct {
	Purpose string
	Reader  Reader
	Pages   []string
	// Authorized records that the owner allowed this specific delegated read.
	// It is meaningless for the other tiers.
	Authorized bool
}

type PageProjection struct {
	StableID          string `json:"stable_id,omitempty"`
	LifecycleRevision uint64 `json:"lifecycle_revision"`
	Page              string `json:"page"`
	Revision          string `json:"revision"`
	Pointer           string `json:"pointer"`
	Content           string `json:"content,omitempty"`
	Attenuated        bool   `json:"attenuated"`
}

type Omission struct {
	Page   string `json:"page"`
	Reason string `json:"reason"`
}

type Projection struct {
	Purpose     string           `json:"purpose"`
	Reader      Reader           `json:"reader"`
	GeneratedAt time.Time        `json:"generated_at"`
	Pages       []PageProjection `json:"pages"`
	Omissions   []Omission       `json:"omissions,omitempty"`
}

// Collect returns the smallest useful projection of named pages. A page that
// does not exist becomes an omission rather than an error, so a caller can act
// on what is there and still see honestly what was not.
func (engine *Engine) Collect(request CollectRequest) (Projection, error) {
	if err := engine.validateBinding(); err != nil {
		return Projection{}, err
	}
	if strings.TrimSpace(request.Purpose) == "" {
		return Projection{}, errors.New("owner atlas collect requires a declared purpose")
	}
	if !knownReader(request.Reader) {
		return Projection{}, fmt.Errorf("owner atlas reader %q is not recognized", request.Reader)
	}
	if len(request.Pages) == 0 {
		// The absence of named pages is the whole-root request the contract
		// refuses. A segment is reached through its index page, which is
		// itself a page, so nothing legitimate needs enumeration.
		return Projection{}, errors.New("owner atlas collect requires named pages; a whole-root projection is not available")
	}
	if len(request.Pages) > maximumCollectPages {
		return Projection{}, fmt.Errorf("owner atlas collect is bounded to %d pages", maximumCollectPages)
	}
	if request.Reader == ReaderDelegate && !request.Authorized {
		return Projection{}, errors.New("a delegated reader requires explicit owner authorization for this projection")
	}

	cohort, err := engine.LifecycleRevision()
	if err != nil {
		return Projection{}, err
	}
	projection := Projection{
		Purpose:     strings.TrimSpace(request.Purpose),
		Reader:      request.Reader,
		GeneratedAt: engine.now().UTC(),
	}
	budget := maximumCollectBytes
	for _, requested := range request.Pages {
		relative, err := engine.resolve(requested)
		if err != nil {
			// A malformed or escaping path is the caller's mistake, not a
			// missing page, so it fails the whole request rather than
			// degrading quietly into an omission.
			return Projection{}, err
		}
		page, err := engine.readPage(relative)
		if err != nil {
			if isNotExist(err) {
				projection.Omissions = append(projection.Omissions, Omission{Page: relative, Reason: "page does not exist"})
				continue
			}
			return Projection{}, err
		}
		if page.LifecycleRevision != cohort {
			return Projection{}, ErrLifecycleChanged
		}
		body := page.Content
		if request.Reader == ReaderDelegate {
			// An authorized delegate learns that the page exists and can ask
			// the owner for what it holds. It does not receive the body.
			page.Attenuated, page.Content = true, ""
			projection.Pages = append(projection.Pages, page)
			continue
		}
		if len(body) > budget {
			projection.Omissions = append(projection.Omissions, Omission{Page: relative, Reason: "projection budget exhausted"})
			continue
		}
		budget -= len(body)
		page.Content = body
		projection.Pages = append(projection.Pages, page)
	}
	if engine.beforeAuthoredDisclosure != nil {
		engine.beforeAuthoredDisclosure()
	}
	// All named pages retain the same lifecycle cohort through disclosure.
	for _, page := range projection.Pages {
		if err := engine.validatePageProjection(page); err != nil {
			return Projection{}, err
		}
	}
	current, err := engine.LifecycleRevision()
	if err != nil {
		return Projection{}, err
	}
	if current != cohort {
		return Projection{}, ErrLifecycleChanged
	}

	return projection, nil
}

func knownReader(reader Reader) bool {
	switch reader {
	case ReaderOwnerSession, ReaderMaestro, ReaderYoda, ReaderDelegate:
		return true
	}
	return false
}

// ReviewPage returns one complete bounded page only for an attended owner review.
// Ordinary collection keeps its aggregate budget; reads never recover operations.
func (engine *Engine) ReviewPage(request CollectRequest) (PageProjection, error) {
	if request.Reader != ReaderOwnerSession || strings.TrimSpace(request.Purpose) == "" || len(request.Pages) != 1 {
		return PageProjection{}, errors.New("full page review requires an owner, purpose and one named page")
	}
	relative, err := engine.resolve(request.Pages[0])
	if err != nil {
		return PageProjection{}, err
	}
	page, err := engine.readPage(relative)
	if err != nil {
		return PageProjection{}, err
	}
	if engine.beforeAuthoredDisclosure != nil {
		engine.beforeAuthoredDisclosure()
	}
	if err = engine.validatePageProjection(page); err != nil {
		return PageProjection{}, err
	}
	return page, nil
}
