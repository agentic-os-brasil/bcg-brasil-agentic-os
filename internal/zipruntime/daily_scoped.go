package zipruntime

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// DailyCheckpoint is the existing selected, agent-authored ZIP checkpoint format.
type DailyCheckpoint = dailyCheckpoint

type DailyEntry struct {
	ID        string `json:"id"`
	Digest    string `json:"digest"`
	LocalDate string `json:"local_date"`
}
type DailyStatus struct {
	State                 string       `json:"state"`
	Scope                 string       `json:"scope"`
	Generation            string       `json:"generation"`
	Dream                 string       `json:"dream"`
	Acknowledged          string       `json:"acknowledged"`
	Pending               int          `json:"pending"`
	PendingTotal          int          `json:"pending_total"`
	OtherWorkspacePending int          `json:"other_workspace_pending"`
	Entries               []DailyEntry `json:"entries"`
	More                  bool         `json:"more"`
	Recovery              string       `json:"recovery,omitempty"`
}

// DailyReviewMax bounds the complete serialized current/proposed review.
const DailyReviewMax = 2 << 20

// JSON may escape every original checkpoint byte sixfold inside its intent.
const dailyTransitionMax = dailyInputMax*6 + 2048

type DailyPageReview struct {
	Current  string `json:"current"`
	Proposed string `json:"proposed"`
	Revision string `json:"revision"`
}
type DailyReview struct {
	Operation        string                     `json:"operation"`
	Scope            string                     `json:"scope"`
	Workspace        string                     `json:"workspace"`
	Generation       string                     `json:"generation"`
	Dream            string                     `json:"dream"`
	Acknowledged     string                     `json:"acknowledged"`
	Checkpoints      []DailyCheckpoint          `json:"checkpoints"`
	Replays          []DailyEntry               `json:"replays,omitempty"`
	Pages            map[string]DailyPageReview `json:"pages,omitempty"`
	PendingRevisions map[string]string          `json:"pending_revisions"`
	PageRevisions    map[string]string          `json:"page_revisions"`
	Recovery         string                     `json:"recovery,omitempty"`
	Digest           string                     `json:"digest"`
}
type DailyResult struct {
	State       string       `json:"state"`
	ID          string       `json:"id,omitempty"`
	Generation  string       `json:"generation,omitempty"`
	Entries     []DailyEntry `json:"committed,omitempty"`
	Unconfirmed []string     `json:"unconfirmed,omitempty"`
	Evidence    string       `json:"evidence,omitempty"`
}

// DailyQueue retains one already admitted storage scope; Workspace is capture
// identity, not a storage root. No scope selection or marker parsing occurs here.
type DailyQueue struct {
	path, workspace, scope string
	directory              *scheduler.HeldDirectory
	pin                    *os.File
	liveGuard              func() error
	// FaultPoint is an in-process crash seam; adapters never populate it.
	FaultPoint func(string) error
}

func OpenDailyQueue(path, workspace, scope string) (*DailyQueue, error) {
	w, e := dailyRoot(workspace)
	if e != nil || w != workspace || scope == "" {
		return nil, errors.New("daily_binding_invalid")
	}
	pin, e := scheduler.OpenDirectory(path)
	if e != nil {
		return nil, e
	}
	held, e := scheduler.HoldDirectory(path, pin)
	if e != nil {
		pin.Close()
		return nil, e
	}
	q := &DailyQueue{path: path, workspace: workspace, scope: scope, directory: held, pin: pin}
	if e = q.validate(); e != nil {
		q.Close()
		return nil, e
	}
	return q, nil
}
func (q *DailyQueue) Scope() string { return q.scope }
func (q *DailyQueue) Close() {
	if q.directory != nil {
		q.directory.Close()
		q.directory = nil
	}
	if q.pin != nil {
		q.pin.Close()
		q.pin = nil
	}
}
func (q *DailyQueue) validate() error {
	if q.liveGuard != nil {
		if e := q.liveGuard(); e != nil {
			return e
		}
	}
	if q.pin == nil || q.directory == nil {
		return errors.New("daily_custody_closed")
	}
	current, e := scheduler.OpenDirectory(q.path)
	if e != nil {
		return e
	}
	defer current.Close()
	a, e := current.Stat()
	if e != nil {
		return e
	}
	b, e := q.pin.Stat()
	if e != nil || !os.SameFile(a, b) {
		return errors.New("daily_binding_changed")
	}
	return nil
}
func dailyHash(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func (q *DailyQueue) read(path string, max int) ([]byte, error) {
	b, e := q.directory.ReadFile(path, int64(max), strings.HasPrefix(path, ".maestro/") || strings.HasPrefix(path, "memory/"))
	if e != nil {
		return nil, e
	}
	if !utf8.Valid(b) {
		return nil, errors.New("daily_encoding_invalid")
	}
	return b, nil
}
func (q *DailyQueue) optional(path string, max int) ([]byte, error) {
	b, e := q.read(path, max)
	if errors.Is(e, os.ErrNotExist) {
		return nil, nil
	}
	return b, e
}
func (q *DailyQueue) marker(path string) (string, error) {
	b, e := q.optional(path, 128)
	if e != nil {
		return "", e
	}
	if b == nil {
		return "", nil
	}
	if !dailyGenerationValid(b) {
		return "", errors.New("daily_generation_invalid")
	}
	return strings.TrimSpace(string(b)), nil
}
func (q *DailyQueue) put(path string, before, after []byte) error {
	if e := q.validate(); e != nil {
		return e
	}
	if len(before) > dailyPageMax || len(after) > dailyPageMax {
		return errors.New("daily_page_full")
	}
	maximum := max(len(before), len(after), 1)
	return q.directory.UpdatePrivateFile(path, int64(maximum), func(current []byte) ([]byte, error) {
		if !bytes.Equal(current, before) {
			return nil, errors.New("daily_preimage_changed")
		}
		return after, nil
	})
}
func (q *DailyQueue) create(path string, body []byte) error {
	if e := q.validate(); e != nil {
		return e
	}
	if e := q.directory.EnsureDirectory(filepath.Dir(path)); e != nil {
		return e
	}
	return q.directory.PublishNewFile(path, body)
}
func (q *DailyQueue) remove(path string, body []byte) error {
	e := q.directory.RemoveExact(path, int64(len(body)), body, func(fn func() error) error {
		if e := q.validate(); e != nil {
			return e
		}
		if e := fn(); e != nil {
			return e
		}
		return q.validate()
	})
	if e != nil {
		return e
	}
	return q.removeUpdateLock(path)
}

// All daily users hold daily.lock; the per-leaf CAS lock is no longer in use.
func (q *DailyQueue) removeUpdateLock(path string) error {
	if e := q.directory.Remove(path + ".lock"); e != nil && !errors.Is(e, os.ErrNotExist) {
		return e
	}
	return q.directory.SyncDirectory(filepath.Dir(path))
}

func (q *DailyQueue) locked(guard func() error, fn func() error) error {
	if guard == nil {
		return errors.New("daily_binding_guard_required")
	}
	if e := guard(); e != nil {
		return e
	}
	if e := q.validate(); e != nil {
		return e
	}
	if e := q.directory.EnsureDirectory(".maestro"); e != nil {
		return e
	}
	if e := q.directory.Mkdir(".maestro/daily.lock"); e != nil {
		return errors.New("daily_writer_busy")
	}
	lock, e := q.directory.StatDirectory(".maestro/daily.lock")
	if e != nil {
		return e
	}
	defer func() {
		if current, e := q.directory.StatDirectory(".maestro/daily.lock"); e == nil && os.SameFile(lock, current) {
			_ = q.directory.Remove(".maestro/daily.lock")
		}
	}()
	if e := guard(); e != nil {
		return e
	}
	if e := q.validate(); e != nil {
		return e
	}
	q.liveGuard = guard
	defer func() { q.liveGuard = nil }()
	return fn()
}
func (q *DailyQueue) checkpoint(body []byte, id string) (DailyCheckpoint, error) {
	if len(body) > dailyInputMax {
		return DailyCheckpoint{}, errors.New("daily_input_size")
	}
	return dailyValidate(body, q.workspace, dailyScope{id: q.scope, path: q.path}, id+".json")
}
func dailyCheckpointBytes(c DailyCheckpoint) []byte {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(c)
	return bytes.TrimSuffix(out.Bytes(), []byte("\n"))
}
func (q *DailyQueue) entry(c DailyCheckpoint) DailyEntry {
	body, _ := json.Marshal(c)
	return DailyEntry{ID: c.ID, Digest: dailyHash(body), LocalDate: c.LocalDate}
}
func (q *DailyQueue) Status() (DailyStatus, error) {
	out := DailyStatus{State: "empty", Scope: q.scope, Entries: []DailyEntry{}}
	if e := q.validate(); e != nil {
		return out, e
	}
	var e error
	if out.Generation, e = q.marker(".maestro/daily-generation"); e != nil {
		return out, e
	}
	if out.Dream, e = q.marker("memory/.dream-requested"); e != nil {
		return out, e
	}
	if out.Acknowledged, e = q.marker("memory/.dream-ack"); e != nil {
		return out, e
	}
	if e = q.requestConsistent(out.Generation, out.Dream, out.Acknowledged); e != nil {
		return out, e
	}
	entries, e := q.directory.ReadDir(".maestro/daily-pending", 257)
	if errors.Is(e, os.ErrNotExist) {
		entries = nil
		e = nil
	}
	if errors.Is(e, io.EOF) {
		e = nil
	}
	if e != nil {
		return out, e
	}
	if len(entries) > 256 {
		return out, errors.New("daily_queue_limit")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") || strings.HasSuffix(entry.Name(), ".json.lock") {
			continue
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			return out, errors.New("daily_pending_invalid")
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		body, e := q.read(".maestro/daily-pending/"+entry.Name(), dailyInputMax)
		if e != nil {
			return out, e
		}
		out.PendingTotal++
		var capture DailyCheckpoint
		if e = strictDaily(body, &capture); e != nil {
			return out, e
		}
		// A shared owner queue keeps other workspace captures opaque and untouched.
		if q.scope == "owner" && capture.Scope == "owner" && filepath.IsAbs(capture.Workspace) && capture.Workspace != q.workspace {
			out.OtherWorkspacePending++
			continue
		}
		c, e := q.checkpoint(body, id)
		if e != nil {
			return out, e
		}
		out.Pending++
		if len(out.Entries) < 32 {
			out.Entries = append(out.Entries, q.entry(c))
		}
	}
	out.More = out.Pending > len(out.Entries)
	if out.Pending > 0 || out.Generation != "" {
		out.State = "ready"
	}
	if body, e := q.optional(".maestro/daily-transition.json", dailyTransitionMax); e != nil {
		return out, e
	} else if body != nil {
		var tr dailyTransition
		if strictDaily(body, &tr) != nil || q.validateTransition(tr) != nil {
			return out, errors.New("daily_transition_invalid")
		}
		out.Recovery = dailyHash(body)
		out.State = "recovery_pending"
	}
	return out, q.validate()
}
func (q *DailyQueue) requestConsistent(generation, dream, ack string) error {
	if generation == "" {
		if dream == "" && ack == "" {
			return nil
		}
		return errors.New("daily_request_changed")
	}
	if dream == generation || dream == "" && ack == generation {
		return nil
	}
	body, e := q.read(".maestro/daily-transition.json", dailyTransitionMax)
	if e != nil {
		return errors.New("daily_request_changed")
	}
	var tr dailyTransition
	if strictDaily(body, &tr) != nil || q.validateTransition(tr) != nil || generation != tr.Next || dream != "" && dream != tr.Previous {
		return errors.New("daily_request_changed")
	}
	return nil
}

func strictDaily(body []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if e := d.Decode(out); e != nil {
		return e
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("daily_trailing_input")
	}
	return nil
}
func (q *DailyQueue) review(operation string, checkpoints []DailyCheckpoint) (DailyReview, error) {
	r := DailyReview{Operation: operation, Scope: q.scope, Workspace: q.workspace, Checkpoints: checkpoints, PendingRevisions: map[string]string{}, PageRevisions: map[string]string{}, Pages: map[string]DailyPageReview{}}
	var e error
	if e = q.validate(); e != nil {
		return r, e
	}
	if r.Generation, e = q.marker(".maestro/daily-generation"); e != nil {
		return r, e
	}
	if r.Dream, e = q.marker("memory/.dream-requested"); e != nil {
		return r, e
	}
	if r.Acknowledged, e = q.marker("memory/.dream-ack"); e != nil {
		return r, e
	}
	if e = q.requestConsistent(r.Generation, r.Dream, r.Acknowledged); e != nil {
		return r, e
	}
	transition, e := q.optional(".maestro/daily-transition.json", dailyTransitionMax)
	if e != nil {
		return r, e
	}
	if transition != nil {
		r.Recovery = dailyHash(transition)
	}
	for _, c := range checkpoints {
		canonical := dailyCheckpointBytes(c)
		if _, e = q.checkpoint(canonical, c.ID); e != nil {
			return r, e
		}
		body, e := q.optional(".maestro/daily-pending/"+c.ID+".json", dailyInputMax)
		if e != nil {
			return r, e
		}
		if body != nil {
			existing, e := q.checkpoint(body, c.ID)
			if e != nil || q.entry(existing) != q.entry(c) {
				return r, errors.New("daily_id_conflict")
			}
			r.PendingRevisions[c.ID] = dailyHash(body)
		}
		page, e := q.optional("daily/"+c.LocalDate+".md", dailyPageMax)
		if e != nil {
			return r, e
		}
		r.PageRevisions[c.LocalDate] = dailyHash(page)
		if operation == "daily.drain" {
			before := r.Pages[c.LocalDate]
			if before.Revision == "" {
				before = DailyPageReview{Current: string(page), Proposed: string(page), Revision: dailyHash(page)}
			}
			proposed, e := dailyRender(c, q.scope, []byte(before.Proposed))
			if e != nil {
				return r, e
			}
			before.Proposed = string(proposed)
			r.Pages[c.LocalDate] = before
			if e = dailySealReview(&r); e != nil {
				return r, e
			}
		}
	}
	if e = dailySealReview(&r); e != nil {
		return r, e
	}
	return r, q.validate()
}
func dailySealReview(r *DailyReview) error {
	r.Digest = ""
	raw, _ := json.MarshalIndent(r, "", "  ")
	if len(raw) > DailyReviewMax-1024 {
		return errors.New("daily_review_too_large")
	}
	r.Digest = dailyHash(raw)
	return nil
}
func (q *DailyQueue) ReviewEnqueue(c DailyCheckpoint) (DailyReview, error) {
	return q.review("daily.enqueue", []DailyCheckpoint{c})
}
func (q *DailyQueue) ReviewDrain(ids []string) (DailyReview, error) {
	if len(ids) < 1 || len(ids) > 32 {
		return DailyReview{}, errors.New("daily_selection_invalid")
	}
	seen := map[string]bool{}
	checkpoints := []DailyCheckpoint{}
	replays := []DailyEntry{}
	trBody, e := q.optional(".maestro/daily-transition.json", dailyTransitionMax)
	if e != nil {
		return DailyReview{}, e
	}
	var tr dailyTransition
	if trBody != nil {
		if strictDaily(trBody, &tr) != nil || q.validateTransition(tr) != nil {
			return DailyReview{}, errors.New("daily_transition_invalid")
		}
	}
	for _, id := range ids {
		if !dailyToken.MatchString(id) || seen[id] {
			return DailyReview{}, errors.New("daily_selection_invalid")
		}
		seen[id] = true
		body, e := q.optional(".maestro/daily-pending/"+id+".json", dailyInputMax)
		if e != nil {
			return DailyReview{}, e
		}
		var c DailyCheckpoint
		if body == nil {
			if trBody != nil && tr.Checkpoint.ID == id {
				c = tr.Checkpoint
			} else {
				proof, e := q.committedProof(id)
				if e != nil {
					return DailyReview{}, e
				}
				replays = append(replays, proof.DailyEntry)
				continue
			}
		} else {
			c, e = q.checkpoint(body, id)
			if e != nil {
				return DailyReview{}, e
			}
		}
		checkpoints = append(checkpoints, c)
	}
	if trBody != nil && !seen[tr.Checkpoint.ID] {
		return DailyReview{}, errors.New("daily_recovery_selection_required")
	}
	sort.SliceStable(checkpoints, func(i, j int) bool {
		if trBody != nil {
			return checkpoints[i].ID == tr.Checkpoint.ID && checkpoints[j].ID != tr.Checkpoint.ID
		}
		return checkpoints[i].ID < checkpoints[j].ID
	})
	r, e := q.review("daily.drain", checkpoints)
	if e != nil {
		return r, e
	}
	sort.Slice(replays, func(i, j int) bool { return replays[i].ID < replays[j].ID })
	r.Replays = replays
	for _, entry := range replays {
		page, e := q.read("daily/"+entry.LocalDate+".md", dailyPageMax)
		if e != nil {
			return r, e
		}
		if _, ok := r.Pages[entry.LocalDate]; !ok {
			r.Pages[entry.LocalDate] = DailyPageReview{Current: string(page), Proposed: string(page), Revision: dailyHash(page)}
		}
		r.PageRevisions[entry.LocalDate] = dailyHash(page)
	}

	if e = dailySealReview(&r); e != nil {
		return r, e
	}
	return r, q.validate()
}

type dailyCommitProof struct {
	DailyEntry
	BlockSHA256 string `json:"block_sha256"`
}

// Compact commit evidence binds the canonical payload digest to the exact
// rendered block; completed prose remains only in its original daily page.
func (q *DailyQueue) committedProof(id string) (dailyCommitProof, error) {
	var proof dailyCommitProof
	body, e := q.read(".maestro/daily-proofs/"+id+".json", 512)
	if e != nil {
		return proof, e
	}
	if strictDaily(body, &proof) != nil || proof.ID != id || len(proof.Digest) != 64 || len(proof.BlockSHA256) != 64 {
		return proof, errors.New("daily_commit_invalid")
	}
	binding, e := q.read(".maestro/daily-committed/"+id+".json", 256)
	if e != nil || string(binding) != proof.LocalDate+" "+proof.Digest+"\n" {
		return proof, errors.New("daily_commit_invalid")
	}
	if len(proof.LocalDate) != 10 || strings.ContainsAny(proof.LocalDate, "/\\") {
		return proof, errors.New("daily_commit_invalid")
	}
	page, e := q.read("daily/"+proof.LocalDate+".md", dailyPageMax)
	if e != nil {
		return proof, e
	}
	start := []byte("\n<!-- maestro:daily-id " + id + " " + proof.Digest + " -->")
	end := []byte("<!-- maestro:daily-end " + id + " -->\n")
	if bytes.Count(page, start) != 1 || bytes.Count(page, end) != 1 {
		return proof, errors.New("daily_block_invalid")
	}
	a := bytes.Index(page, start)
	b := bytes.Index(page[a:], end)
	if b < 0 || dailyHash(page[a:a+b+len(end)]) != proof.BlockSHA256 {
		return proof, errors.New("daily_block_invalid")
	}
	return proof, nil
}

func (q *DailyQueue) Enqueue(review DailyReview, guard func() error) (DailyResult, error) {
	out := DailyResult{State: "pending"}
	e := q.locked(guard, func() error {
		if len(review.Checkpoints) != 1 {
			return errors.New("daily_review_invalid")
		}
		current, e := q.ReviewEnqueue(review.Checkpoints[0])
		if e != nil {
			return e
		}
		if current.Digest != review.Digest {
			return errors.New("daily_review_changed")
		}
		c := review.Checkpoints[0]
		out.ID = c.ID
		out.Generation = current.Generation
		committed, e := q.optional(".maestro/daily-committed/"+c.ID+".json", 256)
		if e != nil {
			return e
		}
		if committed != nil {
			if e = q.ensureProof(c); e != nil {
				return e
			}
			out.State = "replayed"
			return nil
		}
		if current.PendingRevisions[c.ID] != "" {
			out.State = "queued"
			return nil
		}
		status, e := q.Status()
		if e != nil {
			return e
		}
		if status.Recovery != "" {
			return errors.New("daily_recovery_required")
		}
		if status.PendingTotal >= 256 {
			return errors.New("daily_queue_limit")
		}
		body := dailyCheckpointBytes(c)
		if e = q.create(".maestro/daily-pending/"+c.ID+".json", body); e != nil {
			return e
		}
		out.State = "queued"
		return nil
	})
	return out, e
}
func (q *DailyQueue) Drain(review DailyReview, guard func() error) (DailyResult, error) {
	out := DailyResult{State: "pending", Entries: []DailyEntry{}}
	e := q.locked(guard, func() error {
		ids := []string{}
		for _, c := range review.Checkpoints {
			ids = append(ids, c.ID)
		}
		for _, entry := range review.Replays {
			ids = append(ids, entry.ID)
		}
		current, e := q.ReviewDrain(ids)
		if e != nil {
			return e
		}
		if current.Digest != review.Digest {
			return errors.New("daily_review_changed")
		}
		for _, entry := range current.Replays {
			if _, e = q.committedProof(entry.ID); e != nil {
				return e
			}
			out.Entries = append(out.Entries, entry)
		}
		expected := current.PageRevisions
		for _, c := range current.Checkpoints {
			if e = guard(); e != nil {
				return e
			}
			if e = q.save(c, expected[c.LocalDate]); e != nil {
				return e
			}
			if e = q.verify(c); e != nil {
				return e
			}
			page, e := q.read("daily/"+c.LocalDate+".md", dailyPageMax)
			if e != nil {
				return e
			}
			expected[c.LocalDate] = dailyHash(page)
			out.Entries = append(out.Entries, q.entry(c))
		}
		out.Generation, e = q.marker(".maestro/daily-generation")
		if e != nil {
			return e
		}
		out.State = "persisted"
		return q.validate()
	})
	if e != nil {
		out.Generation = ""
		if len(out.Entries) > 0 {
			out.State = "partial"
		}
		confirmed := map[string]bool{}
		for _, entry := range out.Entries {
			confirmed[entry.ID] = true
		}
		for _, c := range review.Checkpoints {
			if !confirmed[c.ID] {
				out.Unconfirmed = append(out.Unconfirmed, c.ID)
			}
		}
		for _, entry := range review.Replays {
			if !confirmed[entry.ID] {
				out.Unconfirmed = append(out.Unconfirmed, entry.ID)
			}
		}
	}
	return out, e
}

type dailyTransition struct {
	Checkpoint      DailyCheckpoint `json:"checkpoint"`
	Previous        string          `json:"previous"`
	Next            string          `json:"next"`
	BeforePage      string          `json:"before_page"`
	AfterPage       string          `json:"after_page"`
	PendingRevision string          `json:"pending_revision"`
}

func (q *DailyQueue) validateTransition(tr dailyTransition) error {
	body := dailyCheckpointBytes(tr.Checkpoint)
	if _, e := q.checkpoint(body, tr.Checkpoint.ID); e != nil {
		return e
	}
	if tr.Previous != "" && !dailyGenerationValid([]byte(tr.Previous+"\n")) {
		return errors.New("daily_transition_invalid")
	}
	binding := q.binding(tr.Checkpoint)
	previous := []byte(nil)
	if tr.Previous != "" {
		previous = []byte(tr.Previous + "\n")
	}
	next := dailyHash(append(append(previous, []byte(tr.Checkpoint.ID+" ")...), binding...))
	if next != tr.Next || len(tr.BeforePage) != 64 || len(tr.AfterPage) != 64 || len(tr.PendingRevision) != 64 {
		return errors.New("daily_transition_invalid")
	}
	return nil
}
func (q *DailyQueue) binding(c DailyCheckpoint) []byte {
	return []byte(c.LocalDate + " " + q.entry(c).Digest + "\n")
}
func (q *DailyQueue) fail(stage string) error {
	if q.FaultPoint != nil {
		return q.FaultPoint(stage)
	}
	return nil
}
func (q *DailyQueue) save(c DailyCheckpoint, expectedPage string) error {
	entry := q.entry(c)
	pagePath := "daily/" + c.LocalDate + ".md"
	pendingPath := ".maestro/daily-pending/" + c.ID + ".json"
	transitionPath := ".maestro/daily-transition.json"
	transition, e := q.optional(transitionPath, dailyTransitionMax)
	if e != nil {
		return e
	}
	var tr dailyTransition
	if transition != nil {
		if strictDaily(transition, &tr) != nil || q.validateTransition(tr) != nil || q.entry(tr.Checkpoint) != entry {
			return errors.New("daily_transition_conflict")
		}
	} else {
		committed, e := q.optional(".maestro/daily-committed/"+c.ID+".json", 256)
		if e != nil {
			return e
		}
		if committed != nil {
			if e = q.ensureProof(c); e != nil {
				return e
			}
			body, e := q.optional(pendingPath, dailyInputMax)
			if e != nil {
				return e
			}
			if body != nil {
				actual, e := q.checkpoint(body, c.ID)
				if e != nil || q.entry(actual) != entry {
					return errors.New("daily_id_conflict")
				}
				return q.remove(pendingPath, body)
			}
			return nil
		}
		old, e := q.optional(pagePath, dailyPageMax)
		if e != nil {
			return e
		}
		if dailyHash(old) != expectedPage {
			return errors.New("daily_page_changed")
		}
		next, e := dailyRender(c, q.scope, old)
		if e != nil {
			return e
		}
		generation, e := q.marker(".maestro/daily-generation")
		if e != nil {
			return e
		}
		pending, e := q.read(pendingPath, dailyInputMax)
		if e != nil {
			return e
		}
		actual, e := q.checkpoint(pending, c.ID)
		if e != nil || q.entry(actual) != entry {
			return errors.New("daily_id_conflict")
		}
		prev := []byte(nil)
		if generation != "" {
			prev = []byte(generation + "\n")
		}
		tr = dailyTransition{Checkpoint: c, Previous: generation, Next: dailyHash(append(append(prev, []byte(c.ID+" ")...), q.binding(c)...)), BeforePage: dailyHash(old), AfterPage: dailyHash(next), PendingRevision: dailyHash(pending)}
		transition, _ = json.Marshal(tr)
		if e = q.create(transitionPath, transition); e != nil {
			return e
		}
		if e = q.fail("intent"); e != nil {
			return e
		}
	}
	binding := q.binding(c)
	intentPath := ".maestro/daily-ids/" + c.ID + ".json"
	intent, e := q.optional(intentPath, 256)
	if e != nil {
		return e
	}
	if intent == nil {
		if e = q.create(intentPath, binding); e != nil {
			return e
		}
	} else if !bytes.Equal(intent, binding) {
		return errors.New("daily_id_conflict")
	}
	if e = q.fail("binding"); e != nil {
		return e
	}
	old, e := q.optional(pagePath, dailyPageMax)
	if e != nil {
		return e
	}
	if dailyHash(old) == tr.BeforePage {
		next, e := dailyRender(c, q.scope, old)
		if e != nil || dailyHash(next) != tr.AfterPage {
			return errors.New("daily_page_changed")
		}
		if e = q.put(pagePath, old, next); e != nil {
			return e
		}
	} else if dailyHash(old) != tr.AfterPage {
		return errors.New("daily_page_changed")
	}
	if e = q.fail("page"); e != nil {
		return e
	}
	generation, e := q.marker(".maestro/daily-generation")
	if e != nil {
		return e
	}
	if generation == tr.Previous {
		var old []byte
		if generation != "" {
			old = []byte(generation + "\n")
		}
		if e = q.put(".maestro/daily-generation", old, []byte(tr.Next+"\n")); e != nil {
			return e
		}
	} else if generation != tr.Next {
		return errors.New("daily_generation_changed")
	}
	if e = q.fail("generation"); e != nil {
		return e
	}
	commitPath := ".maestro/daily-committed/" + c.ID + ".json"
	committed, e := q.optional(commitPath, 256)
	if e != nil {
		return e
	}
	if committed == nil {
		if e = q.create(commitPath, binding); e != nil {
			return e
		}
	} else if !bytes.Equal(committed, binding) {
		return errors.New("daily_id_conflict")
	}
	if e = q.fail("committed"); e != nil {
		return e
	}
	if e = q.ensureProof(c); e != nil {
		return e
	}
	if e = q.fail("proof"); e != nil {
		return e
	}
	ack, e := q.marker("memory/.dream-ack")
	if e != nil {
		return e
	}
	if ack != tr.Next {
		request, e := q.optional("memory/.dream-requested", 128)
		if e != nil {
			return e
		}
		if request != nil && string(request) != tr.Previous+"\n" && string(request) != tr.Next+"\n" {
			return errors.New("daily_request_changed")
		}
		if e = q.put("memory/.dream-requested", request, []byte(tr.Next+"\n")); e != nil {
			return e
		}
	}
	if e = q.fail("request"); e != nil {
		return e
	}
	pending, e := q.optional(pendingPath, dailyInputMax)
	if e != nil {
		return e
	}
	if pending != nil {
		if dailyHash(pending) != tr.PendingRevision {
			return errors.New("daily_pending_changed")
		}
		if e = q.remove(pendingPath, pending); e != nil {
			return e
		}
	}
	if pending == nil {
		if e = q.removeUpdateLock(pendingPath); e != nil {
			return e
		}
	}
	if e = q.fail("consumed"); e != nil {
		return e
	}
	if e = q.verify(c); e != nil {
		return e
	}
	return q.remove(transitionPath, transition)
}

// Proof completion requires a known reviewed checkpoint and its exact live block.
func (q *DailyQueue) ensureProof(c DailyCheckpoint) error {
	if e := q.verify(c); e != nil {
		return e
	}
	proof, _ := json.Marshal(dailyCommitProof{DailyEntry: q.entry(c), BlockSHA256: dailyHash([]byte(dailyBlock(c)))})
	proofPath := ".maestro/daily-proofs/" + c.ID + ".json"
	prior, e := q.optional(proofPath, 512)
	if e != nil {
		return e
	}
	if prior == nil {
		if e = q.create(proofPath, proof); e != nil {
			return e
		}
	} else if !bytes.Equal(prior, proof) {
		return errors.New("daily_id_conflict")
	}
	return nil
}
func (q *DailyQueue) verify(c DailyCheckpoint) error {
	binding := q.binding(c)
	commit, e := q.read(".maestro/daily-committed/"+c.ID+".json", 256)
	if e != nil || !bytes.Equal(commit, binding) {
		return errors.New("daily_commit_invalid")
	}
	generation, e := q.marker(".maestro/daily-generation")
	if e != nil || generation == "" {
		return errors.New("daily_generation_invalid")
	}
	body, e := q.read("daily/"+c.LocalDate+".md", dailyPageMax)
	if e != nil {
		return e
	}
	block := dailyBlock(c)
	if bytes.Count(body, []byte(block)) != 1 {
		return errors.New("daily_block_invalid")
	}
	return nil
}
func (q *DailyQueue) ReviewAck(generation string) (DailyReview, error) {
	if !dailyGenerationValid([]byte(generation + "\n")) {
		return DailyReview{}, errors.New("daily_generation_invalid")
	}
	r, e := q.review("daily.ack", nil)
	if e != nil {
		return r, e
	}
	if r.Recovery != "" {
		return r, errors.New("daily_recovery_required")
	}
	if r.Generation != generation || r.Dream != "" && r.Dream != generation || r.Dream == "" && r.Acknowledged != generation {
		return r, errors.New("daily_generation_changed")
	}
	return r, nil
}
func (q *DailyQueue) Ack(review DailyReview, guard func() error) (DailyResult, error) {
	out := DailyResult{State: "pending"}
	e := q.locked(guard, func() error {
		r, e := q.ReviewAck(review.Generation)
		if e != nil {
			return e
		}
		if r.Digest != review.Digest {
			return errors.New("daily_review_changed")
		}
		if r.Acknowledged != r.Generation {
			var before []byte
			if r.Acknowledged != "" {
				before = []byte(r.Acknowledged + "\n")
			}
			if e = q.put("memory/.dream-ack", before, []byte(r.Generation+"\n")); e != nil {
				return e
			}
		}
		if r.Dream != "" {
			if e = q.remove("memory/.dream-requested", []byte(r.Dream+"\n")); e != nil {
				return e
			}
		}
		out = DailyResult{State: "acknowledged", Generation: r.Generation, Evidence: "agent_asserted_success"}
		return nil
	})
	return out, e
}
func dailyBlock(c DailyCheckpoint) string {
	b, _ := json.Marshal(c)
	digest := dailyHash(b)
	var block strings.Builder
	fmt.Fprintf(&block, "\n<!-- maestro:daily-id %s %s -->\n### %s — checkpoint\n\n%s\n\n", c.ID, digest, c.CapturedAt, c.Summary)
	for _, g := range []struct {
		title string
		items []string
	}{{"Decisions", c.Decisions}, {"Next actions", c.NextActions}} {
		if len(g.items) > 0 {
			fmt.Fprintf(&block, "%s:\n", g.title)
			for _, v := range g.items {
				fmt.Fprintf(&block, "- %s\n", v)
			}
		}
	}
	fmt.Fprintf(&block, "\nProvenance: agent-authored; session=%s; scope=%s. Not semantic attestation.\n<!-- maestro:daily-end %s -->\n", c.SessionID, c.Scope, c.ID)
	return block.String()
}
func dailyRender(c DailyCheckpoint, scope string, old []byte) ([]byte, error) {
	block := dailyBlock(c)
	if bytes.Contains(old, []byte("<!-- maestro:daily-id "+c.ID+" ")) {
		if bytes.Count(old, []byte(block)) == 1 {
			return old, nil
		}
		return nil, errors.New("daily_block_conflict")
	}
	if len(old) == 0 {
		sensitivity := "owner-private"
		id := "daily/" + c.LocalDate
		if scope != "owner" {
			sensitivity = "client-confidential"
			if strings.HasPrefix(scope, "workspace/") {
				sensitivity = "internal"
			} else {
				parts := strings.Split(scope, "/")
				if len(parts) == 4 {
					id = "accounts/" + parts[1] + "/cases/" + parts[3] + "/" + id
				}
			}
		}
		old = []byte(fmt.Sprintf("---\nid: %s\ntitle: Daily %s\nsummary: Agent-authored daily checkpoints\ntype: daily\nscope: %s\nstatus: active\nsensitivity: %s\nupdated: %s\n---\n\n# Daily %s\n", id, c.LocalDate, scope, sensitivity, c.LocalDate, c.LocalDate))
	}
	if len(old)+len(block) > dailyPageMax {
		return nil, errors.New("daily_page_full")
	}
	return append(append([]byte{}, old...), block...), nil
}
