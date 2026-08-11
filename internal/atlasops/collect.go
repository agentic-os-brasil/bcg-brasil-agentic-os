package atlasops

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Reader names who the projection is for. The tiers come from the accepted
// owner atlas contract: the owner sees their own corpus, Maestro and Walter
// receive bounded task-relevant projections, and a delegated agent gets an
// attenuated excerpt or a pointer and only when explicitly authorized.
type Reader string

const (
	ReaderOwnerSession Reader = "owner_session"
	ReaderMaestro      Reader = "maestro"
	ReaderWalter       Reader = "walter"
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
	Page       string `json:"page"`
	Revision   string `json:"revision"`
	Pointer    string `json:"pointer"`
	Content    string `json:"content,omitempty"`
	Attenuated bool   `json:"attenuated"`
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
		body, err := engine.read(relative)
		if err != nil {
			if isNotExist(err) {
				projection.Omissions = append(projection.Omissions, Omission{Page: relative, Reason: "page does not exist"})
				continue
			}
			return Projection{}, err
		}
		page := PageProjection{
			Page:     relative,
			Revision: digest(body),
			Pointer:  "bcgos://atlas/owner/" + relative,
		}
		if request.Reader == ReaderDelegate {
			// An authorized delegate learns that the page exists and can ask
			// the owner for what it holds. It does not receive the body.
			page.Attenuated = true
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
	return projection, nil
}

func knownReader(reader Reader) bool {
	switch reader {
	case ReaderOwnerSession, ReaderMaestro, ReaderWalter, ReaderDelegate:
		return true
	}
	return false
}
