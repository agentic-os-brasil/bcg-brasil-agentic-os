package memory

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

type CapturePage struct {
	Captures           []CaptureReceipt `json:"captures"`
	Total              int              `json:"total"`
	NextOffset         int              `json:"next_offset"`
	VisibilityRevision uint64           `json:"visibility_revision"`
}

func (engine *Engine) ListCaptures(workspaceID string, offset, limit int) (CapturePage, error) {
	return engine.capturePage(workspaceID, "", offset, limit)
}
func (engine *Engine) SearchCaptures(workspaceID, query string, offset, limit int) (CapturePage, error) {
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 256 || !utf8.ValidString(query) {
		return CapturePage{}, errors.New("memory search requires 1 to 256 UTF-8 bytes")
	}
	return engine.capturePage(workspaceID, strings.ToLower(query), offset, limit)
}
func (engine *Engine) capturePage(workspaceID, query string, offset, limit int) (CapturePage, error) {
	if err := engine.ownedPreflight(workspaceID); err != nil {
		return CapturePage{}, err
	}
	if offset < 0 || offset > 4096 || limit < 1 || limit > 100 {
		return CapturePage{}, errors.New("memory page requires offset 0 to 4096 and limit 1 to 100")
	}
	b, err := engine.readVisibility(workspaceID)
	if err != nil {
		return CapturePage{}, err
	}
	captures, err := engine.allCaptures(workspaceID)
	if err != nil {
		return CapturePage{}, err
	}
	matches := []Capture{}
	for _, c := range captures {
		if c.Revision > b.Denied[c.ID] && (query == "" || (captureReceipt(c, b.Revision).Eligible && strings.Contains(strings.ToLower(c.Text), query))) {
			matches = append(matches, c)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if !matches[i].RecordedAt.Equal(matches[j].RecordedAt) {
			return matches[i].RecordedAt.Before(matches[j].RecordedAt)
		}
		return matches[i].ID < matches[j].ID
	})
	page := CapturePage{Captures: []CaptureReceipt{}, Total: len(matches), NextOffset: -1, VisibilityRevision: b.Revision}
	end := offset + limit
	if end > len(matches) {
		end = len(matches)
	}
	for i := offset; i < end; i++ {
		page.Captures = append(page.Captures, captureReceipt(matches[i], b.Revision))
	}
	if end < len(matches) {
		page.NextOffset = end
	}
	after, err := engine.readVisibility(workspaceID)
	if err != nil {
		return CapturePage{}, err
	}
	if after.Revision != b.Revision {
		return CapturePage{}, ErrCaptureConflict
	}
	return page, nil
}

// Envelope identity survives journal rewriting and does not alter capture-v2 MACs.
func journalCaptureID(c Capture) string {
	body, _ := json.Marshal(c)
	sum := sha256.Sum256(append([]byte("maestro-capture-identity-v1\x00"), body...))
	return hex.EncodeToString(sum[:16])
}
func identifiedJournalCapture(c Capture) Capture {
	c.ID = journalCaptureID(c)
	c.Revision = 1
	if c.SchemaVersion == 0 {
		sum := sha256.Sum256([]byte(c.Text))
		c.SourceDigest = hex.EncodeToString(sum[:])
	}
	return c
}

func (engine *Engine) journalPaths(workspaceID, directory string) ([]string, error) {
	path := engine.currentPath(workspaceID, "l1", directory)
	entries, err := memoryReadDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !continuityCaptureName.MatchString(entry.Name()) {
			return nil, errors.New("invalid memory journal entry")
		}
		paths = append(paths, filepath.Join(path, entry.Name()))
	}
	return paths, nil
}

func (engine *Engine) journalCaptures(workspaceID string) ([]Capture, error) {
	captures := []Capture{}
	for _, directory := range []string{"captures", "attested-captures"} {
		paths, err := engine.journalPaths(workspaceID, directory)
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			body, err := memoryReadFile(path, 1<<20)
			if err != nil {
				return nil, err
			}
			entries, err := engine.decodeJournal(workspaceID, body, directory == "attested-captures")
			if err != nil {
				return nil, err
			}
			for _, c := range entries {
				captures = append(captures, identifiedJournalCapture(c))
				if len(captures) > 4096 {
					return nil, errors.New("memory captures exceed discovery bound")
				}
			}
		}
	}
	return captures, nil
}

func (engine *Engine) decodeJournal(workspaceID string, body []byte, attested bool) ([]Capture, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	captures := []Capture{}
	for scanner.Scan() {
		var c Capture
		if err := decodeMemoryJSON(scanner.Bytes(), &c); err != nil {
			return nil, err
		}
		if c.WorkspaceID != workspaceID || !c.Sanitized || c.RecordedAt.IsZero() || strings.TrimSpace(c.Text) == "" || c.ID != "" || c.Revision != 0 {
			return nil, errors.New("invalid canonical capture journal")
		}
		if attested {
			if c.SchemaVersion != AttestedCaptureSchemaVersion {
				return nil, errors.New("journal is not capture-v2")
			}
			if err := (CaptureAttestor{Root: engine.Root}).Verify(c); err != nil {
				return nil, err
			}
		} else if c.SchemaVersion != 0 {
			return nil, errors.New("invalid legacy capture schema")
		}
		captures = append(captures, c)
		if len(captures) > 4096 {
			return nil, errors.New("memory capture journal exceeds entry bound")
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(captures) == 0 {
		return nil, errors.New("memory capture journal is empty")
	}
	return captures, nil
}

func (engine *Engine) allCaptures(workspaceID string) ([]Capture, error) {
	journal, err := engine.journalCaptures(workspaceID)
	if err != nil {
		return nil, err
	}
	owned, err := engine.ownedCaptures(workspaceID)
	if err != nil {
		return nil, err
	}
	byID := map[string]Capture{}
	for _, c := range append(journal, owned...) {
		byID[c.ID] = c
	}
	if len(byID) > 4096 {
		return nil, errors.New("memory captures exceed discovery bound")
	}
	captures := make([]Capture, 0, len(byID))
	for _, c := range byID {
		captures = append(captures, c)
	}
	return captures, nil
}

func (engine *Engine) currentCapture(workspaceID, id string) (Capture, error) {
	c, err := engine.readOwnedCapture(workspaceID, id)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		return c, err
	}
	captures, err := engine.journalCaptures(workspaceID)
	if err != nil {
		return Capture{}, err
	}
	for _, c := range captures {
		if c.ID == id {
			return c, nil
		}
	}
	return Capture{}, os.ErrNotExist
}

func (engine *Engine) filterJournal(workspaceID string, body []byte, attested bool, b visibilityBarrier) ([]byte, bool, error) {
	captures, err := engine.decodeJournal(workspaceID, body, attested)
	if err != nil {
		return nil, false, err
	}
	var visible bytes.Buffer
	changed := false
	for _, c := range captures {
		if b.Denied[journalCaptureID(c)] >= 1 {
			changed = true
			continue
		}
		encoded, err := json.Marshal(c)
		if err != nil {
			return nil, false, err
		}
		visible.Write(encoded)
		visible.WriteByte('\n')
	}
	return visible.Bytes(), changed, nil
}

func (engine *Engine) purgeDeniedJournals(workspaceID string, b visibilityBarrier) error {
	for _, directory := range []string{"captures", "attested-captures"} {
		paths, err := engine.journalPaths(workspaceID, directory)
		if err != nil {
			return err
		}
		for _, path := range paths {
			body, err := memoryReadFile(path, 1<<20)
			if err != nil {
				return err
			}
			visible, changed, err := engine.filterJournal(workspaceID, body, directory == "attested-captures", b)
			if err != nil {
				return err
			}
			if !changed {
				continue
			}
			if len(visible) == 0 {
				err = memoryRemoveAll(path)
			} else {
				err = memoryPublish(path, visible, false)
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}
