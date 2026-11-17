package ownerctx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const promptHistoryPending = ".transition.json"
const maximumHistoryJournalBytes = 3*maximumPromptSerializedStoreBytes + 16<<20

var ErrPromptHistoryRecovery = errors.New("prompt history transition requires exact reviewed recovery")

// PromptHistoryView contains private material for an attended adapter, never diagnostics.
type PromptHistoryView struct {
	Initialized bool                 `json:"initialized"`
	Revision    string               `json:"revision"`
	Config      PromptHistoryConfig  `json:"config"`
	Entries     []PromptHistoryEntry `json:"-"`
}
type PromptHistoryChange struct {
	Operation string               `json:"operation"`
	Config    *PromptHistoryConfig `json:"config,omitempty"`
	Input     *PromptHistoryInput  `json:"input,omitempty"`
	IDs       []string             `json:"ids,omitempty"`
}
type PromptHistoryReview struct {
	Root           string                 `json:"root"`
	Operation      string                 `json:"operation"`
	BeforeRevision string                 `json:"before_revision"`
	AfterRevision  string                 `json:"after_revision"`
	CurrentConfig  PromptHistoryConfig    `json:"current_config"`
	ProposedConfig PromptHistoryConfig    `json:"proposed_config"`
	Removed        []PromptHistoryReceipt `json:"removed"`
	Record         *PromptHistoryEntry    `json:"record,omitempty"`
	Export         []PromptHistoryEntry   `json:"export,omitempty"`
	Digest         string                 `json:"digest"`
	before         []PromptHistoryEntry
	after          []PromptHistoryEntry
}
type promptHistoryPair struct {
	Config  PromptHistoryConfig  `json:"config"`
	Entries []PromptHistoryEntry `json:"entries"`
}
type promptHistoryJournal struct {
	Version         int                 `json:"version"`
	Review          PromptHistoryReview `json:"review"`
	Before          promptHistoryPair   `json:"-"`
	After           promptHistoryPair   `json:"after"`
	OriginalConfig  []byte              `json:"original_config"`
	OriginalEntries []byte              `json:"original_entries"`
}

func historyRevision(config PromptHistoryConfig, entries []PromptHistoryEntry) string {
	if entries == nil {
		entries = []PromptHistoryEntry{}
	}
	body, _ := json.Marshal(promptHistoryPair{config, entries})
	return digest(string(body))
}
func PromptHistorySnapshot(root string) (PromptHistoryView, error) {
	entries, config, err := readPromptHistorySnapshot(root)
	if err != nil {
		return PromptHistoryView{}, err
	}
	if err = validateHistoryPair(promptHistoryPair{config, entries}); err != nil {
		return PromptHistoryView{}, err
	}
	path, _, err := promptHistoryPaths(root)
	if err != nil {
		return PromptHistoryView{}, err
	}
	_, statErr := os.Lstat(filepath.Dir(path))
	return PromptHistoryView{Initialized: statErr == nil, Revision: historyRevision(config, entries), Config: config, Entries: entries}, nil
}
func promptHistoryReviewDigest(r PromptHistoryReview) string {
	r.Digest = ""
	body, _ := json.Marshal(r)
	return digest(string(body))
}
func ReviewPromptHistory(root string, change PromptHistoryChange) (PromptHistoryReview, error) {
	view, err := PromptHistorySnapshot(root)
	if err != nil {
		return PromptHistoryReview{}, err
	}
	r := PromptHistoryReview{Root: root, Operation: change.Operation, BeforeRevision: view.Revision, CurrentConfig: view.Config, ProposedConfig: view.Config, before: view.Entries, after: append([]PromptHistoryEntry{}, view.Entries...), Removed: []PromptHistoryReceipt{}}
	effectiveOwner := view.Config.OwnerID
	for _, entry := range view.Entries {
		if effectiveOwner == "" {
			effectiveOwner = entry.OwnerID
		}
		if entry.OwnerID != effectiveOwner {
			return r, ErrRevisionConflict
		}
	}
	if effectiveOwner != "" {
		r.ProposedConfig.OwnerID = effectiveOwner
	}
	now := time.Now().UTC()
	switch change.Operation {
	case "configure":
		if change.Config == nil {
			return r, errors.New("history config required")
		}
		config := *change.Config
		if effectiveOwner != "" {
			if config.OwnerID != "" && config.OwnerID != effectiveOwner {
				return r, ErrRevisionConflict
			}
			config.OwnerID = effectiveOwner
		}
		if err = validatePromptHistoryConfig(config); err != nil {
			return r, err
		}
		r.ProposedConfig = config
		r.after = retainPromptHistory(r.after, config, now)
	case "record":
		if change.Input == nil {
			return r, errors.New("history input required")
		}
		in := *change.Input
		if err = validatePromptHistoryInput(in); err != nil {
			return r, err
		}
		if in.OccurrenceID == "" {
			return r, errors.New("stable history occurrence required")
		}
		if effectiveOwner != "" && effectiveOwner != in.OwnerID {
			return r, ErrRevisionConflict
		}
		if in.RecordedAt.IsZero() {
			in.RecordedAt = now
		}
		if in.RecordedAt.After(now.Add(5*time.Minute)) || now.Sub(in.RecordedAt) > time.Duration(view.Config.MaxAgeSeconds)*time.Second {
			return r, errors.New("history timestamp outside retention")
		}
		entry := PromptHistoryEntry{SchemaVersion: 1, OccurrenceID: in.OccurrenceID, OwnerID: in.OwnerID, RecordedAt: in.RecordedAt.UTC(), Language: in.Language, Source: in.Source, SessionID: in.SessionID, ScopeKind: in.ScopeKind, ScopeID: in.ScopeID, SHA256: digest(in.Prompt), ContentKind: "user_prompt", Prompt: in.Prompt}
		entry.ID = "prompt-" + digest(entry.OwnerID + "\x00" + entry.SessionID + "\x00" + entry.OccurrenceID + "\x00" + entry.SHA256)[:24]
		found := false
		for _, before := range r.before {
			if before.OccurrenceID == entry.OccurrenceID {
				if !samePromptOccurrence(before, entry) {
					return r, ErrRevisionConflict
				}
				entry = before
				found = true
				break
			}
		}
		r.Record = &entry
		if !found {
			r.after = retainPromptHistory(append(r.after, entry), view.Config, now)
		}
		found = false
		for _, after := range r.after {
			if after.ID == entry.ID {
				found = true
			}
		}
		if !found {
			return r, errors.New("history prompt excluded by retention")
		}
		r.ProposedConfig.OwnerID = in.OwnerID
	case "delete", "export":
		if len(change.IDs) < 1 || len(change.IDs) > 8 {
			return r, errors.New("select one to eight history entries")
		}
		ids := map[string]bool{}
		for _, id := range change.IDs {
			if !observationIdentifier.MatchString(id) || ids[id] {
				return r, errors.New("invalid history entry selection")
			}
			ids[id] = true
		}
		next := []PromptHistoryEntry{}
		n := 0
		for _, entry := range r.after {
			if ids[entry.ID] {
				n++
				if change.Operation == "export" {
					r.Export = append(r.Export, entry)
				}
			} else {
				next = append(next, entry)
			}
		}
		if n != len(ids) {
			return r, errors.New("history selection not found")
		}
		if change.Operation == "delete" {
			r.after = next
		}
		if change.Operation == "export" {
			body, _ := json.Marshal(r.Export)
			if len(body) > 32<<10 {
				return r, errors.New("history export exceeds reviewed packet bound")
			}
		}
	case "reset":
		r.after = []PromptHistoryEntry{}
	default:
		return r, errors.New("unknown history change")
	}
	kept := map[string]bool{}
	for _, entry := range r.after {
		kept[entry.ID] = true
	}
	for _, entry := range r.before {
		if !kept[entry.ID] {
			r.Removed = append(r.Removed, promptHistoryReceipt(entry))
		}
	}
	r.AfterRevision = historyRevision(r.ProposedConfig, r.after)
	r.Digest = promptHistoryReviewDigest(r)
	return r, nil
}

// ApplyReviewedPromptHistory fences the complete pair; a pending pair is only
// resumed by the same digest after another exact attended review.
func ApplyReviewedPromptHistory(root string, r PromptHistoryReview, confirmed bool) error {
	return ApplyReviewedPromptHistoryGuarded(root, r, confirmed, nil)
}
func ApplyReviewedPromptHistoryGuarded(root string, r PromptHistoryReview, confirmed bool, guard func() error) error {
	if !confirmed {
		return ErrConfirmationRequired
	}
	if root != r.Root || r.Digest == "" || promptHistoryReviewDigest(r) != r.Digest {
		return ErrRevisionConflict
	}
	if guard != nil {
		if err := guard(); err != nil {
			return err
		}
	}
	return withPromptHistoryLockPolicy(root, true, func(entriesPath, configPath string) error {
		if guard != nil {
			if err := guard(); err != nil {
				return err
			}
		}
		dir, err := scheduler.HoldDirectory(filepath.Dir(entriesPath), nil)
		if err != nil {
			return err
		}
		defer dir.Close()
		current, err := readHistoryPairHeld(dir)
		if err != nil {
			return err
		}
		if pending, e := readHistoryHeld(dir, promptHistoryPending, maximumHistoryJournalBytes); e == nil {
			var journal promptHistoryJournal
			if e = decodeHistoryJournal(pending, &journal); e != nil || journal.Version != 1 || journal.Review.Digest != r.Digest || promptHistoryReviewDigest(journal.Review) != r.Digest {
				return ErrPromptHistoryRecovery
			}
			if historyRevision(journal.Before.Config, journal.Before.Entries) != r.BeforeRevision || historyRevision(journal.After.Config, journal.After.Entries) != r.AfterRevision {
				return ErrPromptHistoryRecovery
			}
			return finishHistoryPair(dir, journal, pending, nil)
		} else if !errors.Is(e, os.ErrNotExist) {
			return e
		}
		revision := historyRevision(current.Config, current.Entries)
		if revision == r.AfterRevision {
			return nil
		}
		if revision != r.BeforeRevision {
			return ErrRevisionConflict
		}
		if r.Operation == "export" {
			return nil
		}
		if historyRevision(r.ProposedConfig, r.after) != r.AfterRevision {
			return ErrRevisionConflict
		}
		journal := promptHistoryJournal{Version: 1, Review: r, Before: current, After: promptHistoryPair{r.ProposedConfig, r.after}}
		return publishHistoryPair(dir, journal, nil)
	})
}
func ReviewPendingPromptHistory(root string) (PromptHistoryReview, error) {
	entries, _, err := promptHistoryPaths(root)
	if err != nil {
		return PromptHistoryReview{}, err
	}
	dir, err := scheduler.HoldDirectory(filepath.Dir(entries), nil)
	if err != nil {
		return PromptHistoryReview{}, err
	}
	defer dir.Close()
	body, err := readHistoryHeld(dir, promptHistoryPending, maximumHistoryJournalBytes)
	if err != nil {
		return PromptHistoryReview{}, err
	}
	var j promptHistoryJournal
	if err = decodeHistoryJournal(body, &j); err != nil || j.Version != 1 || j.Review.Root != root || promptHistoryReviewDigest(j.Review) != j.Review.Digest || historyRevision(j.Before.Config, j.Before.Entries) != j.Review.BeforeRevision || historyRevision(j.After.Config, j.After.Entries) != j.Review.AfterRevision {
		return PromptHistoryReview{}, ErrPromptHistoryRecovery
	}
	j.Review.before = j.Before.Entries
	j.Review.after = j.After.Entries
	return j.Review, nil
}
func readHistoryHeld(dir *scheduler.HeldDirectory, name string, max int) ([]byte, error) {
	file, err := dir.OpenFile(name, os.O_RDONLY)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if err = scheduler.ValidatePrivateFile(file, 0600); err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(file, int64(max)+1))
	if err == nil && len(body) > max {
		return nil, errors.New("history file exceeds bound")
	}
	return body, err
}
func readHistoryPairHeld(dir *scheduler.HeldDirectory) (promptHistoryPair, error) {
	var pair promptHistoryPair
	body, err := readHistoryHeld(dir, "config.json", 16<<10)
	if err != nil {
		return pair, err
	}
	if err = json.Unmarshal(body, &pair.Config); err != nil {
		return pair, err
	}
	if err = validatePromptHistoryConfig(pair.Config); err != nil {
		return pair, err
	}
	body, err = readHistoryHeld(dir, "entries.jsonl", maximumPromptSerializedStoreBytes)
	if err != nil {
		return pair, err
	}
	pair.Entries, err = parsePromptHistoryEntries(body)
	if err == nil {
		err = validateHistoryPair(pair)
	}
	return pair, err
}
func historyPairMatches(current, allowed promptHistoryPair) bool {
	return historyRevision(current.Config, current.Entries) == historyRevision(allowed.Config, allowed.Entries)
}
func validateHistoryPair(pair promptHistoryPair) error {
	if err := validatePromptHistoryConfig(pair.Config); err != nil {
		return err
	}
	if len(pair.Entries) > maximumPromptEntries {
		return ErrPromptHistoryRecovery
	}
	owner := pair.Config.OwnerID
	total := 0
	for _, entry := range pair.Entries {
		if err := validatePromptHistoryEntry(entry); err != nil {
			return err
		}
		if owner == "" {
			owner = entry.OwnerID
		}
		if owner != entry.OwnerID {
			return ErrRevisionConflict
		}
		total += len(entry.Prompt)
		if total > maximumPromptStoreBytes {
			return ErrPromptHistoryRecovery
		}
	}
	return nil
}
func decodeHistoryJournal(body []byte, j *promptHistoryJournal) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(j); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return ErrPromptHistoryRecovery
	}
	if len(j.OriginalConfig) > 16<<10 || len(j.OriginalEntries) > maximumPromptSerializedStoreBytes {
		return ErrPromptHistoryRecovery
	}
	if err := json.Unmarshal(j.OriginalConfig, &j.Before.Config); err != nil {
		return err
	}
	var err error
	j.Before.Entries, err = parsePromptHistoryEntries(j.OriginalEntries)
	if err != nil {
		return err
	}
	if err = validateHistoryPair(j.Before); err != nil {
		return err
	}
	return validateHistoryPair(j.After)
}
func publishHistoryPair(dir *scheduler.HeldDirectory, j promptHistoryJournal, fault func(string) error) error {
	current, err := readHistoryPairHeld(dir)
	if err != nil {
		return err
	}
	if !historyPairMatches(current, j.Before) {
		return ErrRevisionConflict
	}
	j.OriginalConfig, err = readHistoryHeld(dir, "config.json", 16<<10)
	if err != nil {
		return err
	}
	j.OriginalEntries, err = readHistoryHeld(dir, "entries.jsonl", maximumPromptSerializedStoreBytes)
	if err != nil {
		return err
	}
	original := promptHistoryPair{}
	if err = json.Unmarshal(j.OriginalConfig, &original.Config); err != nil {
		return err
	}
	original.Entries, err = parsePromptHistoryEntries(j.OriginalEntries)
	if err != nil || !historyPairMatches(original, j.Before) {
		return ErrRevisionConflict
	}
	if err = validateHistoryPair(j.Before); err != nil {
		return err
	}
	if err = validateHistoryPair(j.After); err != nil {
		return err
	}
	body, err := json.Marshal(j)
	if err != nil {
		return err
	}
	if err = dir.PublishNewFile(promptHistoryPending, body); err != nil {
		return err
	}
	return finishHistoryPair(dir, j, body, fault)
}
func finishHistoryPair(dir *scheduler.HeldDirectory, j promptHistoryJournal, raw []byte, fault func(string) error) error {
	config, err := json.MarshalIndent(j.After.Config, "", "  ")
	if err != nil {
		return err
	}
	config = append(config, '\n')
	entries, err := encodePromptHistoryEntries(j.After.Entries)
	if err != nil {
		return err
	}
	currentConfig, err := readHistoryHeld(dir, "config.json", 16<<10)
	if err != nil {
		return err
	}
	currentEntries, err := readHistoryHeld(dir, "entries.jsonl", maximumPromptSerializedStoreBytes)
	if err != nil {
		return err
	}
	configBefore := bytes.Equal(currentConfig, j.OriginalConfig)
	configAfter := bytes.Equal(currentConfig, config)
	entriesBefore := bytes.Equal(currentEntries, j.OriginalEntries)
	entriesAfter := bytes.Equal(currentEntries, entries)
	if !(configBefore && entriesBefore || configAfter && entriesBefore || configAfter && entriesAfter) {
		return ErrPromptHistoryRecovery
	}
	for _, item := range []struct {
		name string
		body []byte
	}{{"config.json", config}, {"entries.jsonl", entries}} {
		if err = replaceHistoryHeld(dir, item.name, item.body); err != nil {
			return err
		}
		if fault != nil {
			if err = fault(item.name); err != nil {
				return err
			}
		}
	}
	currentConfig, err = readHistoryHeld(dir, "config.json", 16<<10)
	if err != nil || !bytes.Equal(currentConfig, config) {
		return errors.Join(err, ErrPromptHistoryRecovery)
	}
	currentEntries, err = readHistoryHeld(dir, "entries.jsonl", maximumPromptSerializedStoreBytes)
	if err != nil || !bytes.Equal(currentEntries, entries) {
		return errors.Join(err, ErrPromptHistoryRecovery)
	}
	pending, err := readHistoryHeld(dir, promptHistoryPending, len(raw)+1)
	if err != nil || !bytes.Equal(pending, raw) {
		return errors.Join(err, ErrPromptHistoryRecovery)
	}
	if err = dir.Remove(promptHistoryPending); err != nil {
		return err
	}
	return dir.SyncDirectory(".")
}
func replaceHistoryHeld(dir *scheduler.HeldDirectory, name string, body []byte) error {
	nonce, err := randomCapability()
	if err != nil {
		return err
	}
	stage := ".history-" + nonce
	if err = dir.PublishNewFile(stage, body); err != nil {
		return err
	}
	defer dir.Remove(stage)
	return dir.Rename(stage, name, true)
}
