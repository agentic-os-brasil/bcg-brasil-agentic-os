package atlasops

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/privatelock"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// A standing grant is how the owner authorizes a ritual to maintain part of
// their atlas without being asked every time. It is deliberately narrow: one
// ritual, one page family, a named operation set, and metadata that lets the
// owner see it, pause it and take it back.
const (
	GrantActive  = "active"
	GrantPaused  = "paused"
	GrantRevoked = "revoked"
	GrantExpired = "expired"
)

// Catch-up policy decides what a ritual may do about occurrences it missed.
const (
	CatchUpSkip   = "skip"
	CatchUpSingle = "single"
)

const grantsDocument = "grants.json"

type GrantRequest struct {
	Ritual        string
	RitualVersion string
	Segment       string
	Operations    []string
	Cadence       string
	CatchUp       string
	Reader        Reader
	Retention     string
	ExpiresAt     *time.Time
}

type Grant struct {
	GrantID       string     `json:"grant_id"`
	Ritual        string     `json:"ritual"`
	RitualVersion string     `json:"ritual_version"`
	Segment       string     `json:"segment"`
	Operations    []string   `json:"operations"`
	Cadence       string     `json:"cadence"`
	CatchUp       string     `json:"catch_up"`
	Reader        Reader     `json:"reader"`
	Retention     string     `json:"retention"`
	CreatedAt     time.Time  `json:"created_at"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	PausedAt      *time.Time `json:"paused_at,omitempty"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
	// Scheduled tells the caller whether occurrences fire on their own. It is
	// false today: no scheduler drives owner atlas rituals yet, so a grant
	// authorizes a manual occurrence and says so rather than implying a
	// cadence it cannot honour.
	Scheduled bool `json:"scheduled"`
}

// StateAt derives the grant's state rather than storing it, so a grant cannot
// drift out of sync with its own expiry. The clock is a parameter because
// expiry is the one transition nothing writes down: a grant becomes expired by
// time passing, not by anyone acting on it.
func (grant Grant) StateAt(now time.Time) string {
	switch {
	case grant.RevokedAt != nil:
		return GrantRevoked
	case grant.PausedAt != nil:
		return GrantPaused
	case grant.ExpiresAt != nil && !now.Before(*grant.ExpiresAt):
		return GrantExpired
	default:
		return GrantActive
	}
}

var grantableOperations = map[string]bool{
	"create-page":  true,
	"set-field":    true,
	"link":         true,
	"append-entry": true,
}

// CreateGrant records a new standing grant. Every field of the binding is
// required: a grant that does not say which ritual, which pages, which
// operations and how often is not inspectable, and an authority the owner
// cannot inspect is one they cannot meaningfully revoke.
func (engine *Engine) CreateGrant(request GrantRequest) (Grant, error) {
	segment, err := normalizeSegment(request.Segment)
	if err != nil {
		return Grant{}, err
	}
	if strings.TrimSpace(request.Ritual) == "" || strings.TrimSpace(request.RitualVersion) == "" {
		return Grant{}, errors.New("a standing grant requires a ritual and its version")
	}
	if strings.TrimSpace(request.Cadence) == "" {
		return Grant{}, errors.New("a standing grant requires a cadence")
	}
	if len(request.Operations) == 0 {
		return Grant{}, errors.New("a standing grant requires at least one allowed operation")
	}
	for _, operation := range request.Operations {
		if !grantableOperations[operation] {
			return Grant{}, fmt.Errorf("operation %q cannot be granted", operation)
		}
	}
	if !knownReader(request.Reader) {
		return Grant{}, fmt.Errorf("owner atlas reader %q is not recognized", request.Reader)
	}
	catchUp := request.CatchUp
	if catchUp == "" {
		catchUp = CatchUpSkip
	}
	if catchUp != CatchUpSkip && catchUp != CatchUpSingle {
		return Grant{}, fmt.Errorf("catch-up policy %q is not recognized", catchUp)
	}

	now := engine.now().UTC()
	grant := Grant{
		Ritual:        strings.TrimSpace(request.Ritual),
		RitualVersion: strings.TrimSpace(request.RitualVersion),
		Segment:       segment,
		Operations:    append([]string(nil), request.Operations...),
		Cadence:       strings.TrimSpace(request.Cadence),
		CatchUp:       catchUp,
		Reader:        request.Reader,
		Retention:     strings.TrimSpace(request.Retention),
		CreatedAt:     now,
		ExpiresAt:     request.ExpiresAt,
		Scheduled:     false,
	}
	grant.GrantID = "grant-" + digest(grant.Ritual + "\x00" + grant.RitualVersion + "\x00" + grant.Segment + "\x00" + now.Format(time.RFC3339Nano))[:32]

	err = engine.mutateGrants(func(grants []Grant) ([]Grant, error) {
		return append(grants, grant), nil
	})
	if err != nil {
		return Grant{}, err
	}
	return grant, nil
}

// Grant returns one grant by identifier.
func (engine *Engine) Grant(grantID string) (Grant, bool, error) {
	grants, err := engine.readGrants()
	if err != nil {
		return Grant{}, false, err
	}
	for _, grant := range grants {
		if grant.GrantID == grantID {
			return grant, true, nil
		}
	}
	return Grant{}, false, nil
}

// Grants returns every grant so the owner can see what holds authority over
// their atlas. The document is one bounded file, so this needs no enumeration.
func (engine *Engine) Grants() ([]Grant, error) {
	return engine.readGrants()
}

// AuthorizeGrant answers whether a grant permits one operation on one page
// right now. It is the only place a grant turns into permission.
func (engine *Engine) AuthorizeGrant(grantID, operation, page string) error {
	grant, found, err := engine.Grant(grantID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("standing grant %q does not exist", grantID)
	}
	if state := grant.StateAt(engine.now().UTC()); state != GrantActive {
		return fmt.Errorf("standing grant %q is %s", grantID, state)
	}
	permitted := false
	for _, allowed := range grant.Operations {
		if allowed == operation {
			permitted = true
			break
		}
	}
	if !permitted {
		return fmt.Errorf("standing grant %q does not allow %s", grantID, operation)
	}
	relative, err := engine.resolve(page)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(relative+"/", grant.Segment) && !strings.HasPrefix(relative, grant.Segment) {
		return fmt.Errorf("standing grant %q does not cover %s", grantID, relative)
	}
	return nil
}

// PauseGrant suspends a grant without discarding it.
func (engine *Engine) PauseGrant(grantID string) error {
	now := engine.now().UTC()
	return engine.updateGrant(grantID, func(grant *Grant) error {
		if grant.RevokedAt != nil {
			return fmt.Errorf("standing grant %q is revoked", grantID)
		}
		grant.PausedAt = &now
		return nil
	})
}

// ResumeGrant lifts a pause. A revoked grant cannot be resumed: revocation is
// how the owner takes an authority back for good, and a revocation that can be
// undone is not one.
func (engine *Engine) ResumeGrant(grantID string) error {
	return engine.updateGrant(grantID, func(grant *Grant) error {
		if grant.RevokedAt != nil {
			return fmt.Errorf("standing grant %q is revoked and cannot be resumed", grantID)
		}
		grant.PausedAt = nil
		return nil
	})
}

// RevokeGrant ends a grant permanently. It removes the authority, never the
// content: pages written while the grant was live are the owner's, and taking
// back permission is not a reason to erase their work.
func (engine *Engine) RevokeGrant(grantID string) error {
	now := engine.now().UTC()
	return engine.updateGrant(grantID, func(grant *Grant) error {
		if grant.RevokedAt == nil {
			grant.RevokedAt = &now
		}
		return nil
	})
}

func (engine *Engine) updateGrant(grantID string, mutate func(*Grant) error) error {
	return engine.mutateGrants(func(grants []Grant) ([]Grant, error) {
		for index := range grants {
			if grants[index].GrantID != grantID {
				continue
			}
			if err := mutate(&grants[index]); err != nil {
				return nil, err
			}
			return grants, nil
		}
		return nil, fmt.Errorf("standing grant %q does not exist", grantID)
	})
}

type grantsFile struct {
	SchemaVersion int     `json:"schema_version"`
	Grants        []Grant `json:"grants"`
}

func (engine *Engine) grantsPath() string {
	return engine.journalDirPath(grantsDocument)
}

func (engine *Engine) readGrants() ([]Grant, error) {
	body, err := scheduler.ReadPrivateFile(engine.grantsPath(), maximumPageBytes)
	if err != nil {
		if isNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var document grantsFile
	if err := json.Unmarshal(body, &document); err != nil {
		return nil, err
	}
	if document.SchemaVersion != schemaVersion {
		return nil, fmt.Errorf("owner atlas grant schema %d is not supported", document.SchemaVersion)
	}
	return document.Grants, nil
}

// mutateGrants serializes read-modify-write behind a cross-process lock so two
// sessions cannot silently drop one another's grant.
func (engine *Engine) mutateGrants(mutate func([]Grant) ([]Grant, error)) error {
	if err := scheduler.EnsurePrivateDirectory(engine.journalDirPath("")); err != nil {
		return err
	}
	release, err := privatelock.Acquire(engine.journalDirPath("grants.lock"))
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	grants, err := engine.readGrants()
	if err != nil {
		return err
	}
	updated, err := mutate(grants)
	if err != nil {
		return err
	}
	body, err := json.Marshal(grantsFile{SchemaVersion: schemaVersion, Grants: updated})
	if err != nil {
		return err
	}
	writeErr := scheduler.WriteNewPrivateFile(engine.grantsPath(), body)
	if errors.Is(writeErr, os.ErrExist) {
		return scheduler.ReplacePrivateFile(engine.grantsPath(), body)
	}
	return writeErr
}

// normalizeSegment turns a page family into a bounded prefix inside the root.
func normalizeSegment(segment string) (string, error) {
	trimmed := strings.TrimSpace(segment)
	if trimmed == "" {
		return "", errors.New("a standing grant requires a segment")
	}
	slashed := strings.TrimSuffix(path.Clean(strings.ReplaceAll(trimmed, "\\", "/")), "/")
	if slashed == "." || slashed == ".." || strings.HasPrefix(slashed, "../") || strings.HasPrefix(slashed, "/") {
		return "", fmt.Errorf("segment %q escapes the owner root", segment)
	}
	return slashed + "/", nil
}
