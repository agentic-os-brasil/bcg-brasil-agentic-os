package ownerctx

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	PromptHistorySchemaVersion = 1
	PromptHistoryRelativePath  = "owner/prompt-history/entries.jsonl"
	PromptHistoryConfigPath    = "owner/prompt-history/config.json"
	maximumPromptBytes         = 64 << 10
	maximumPromptEntries       = 10000
	maximumPromptStoreBytes    = 8 << 20
	maximumPromptPacketCount   = 8
	maximumPromptPacketBytes   = 32 << 10
)

type PromptScopeKind string

const (
	PromptScopeGlobal    PromptScopeKind = "global"
	PromptScopeWorkspace PromptScopeKind = "workspace"
	PromptScopeAccount   PromptScopeKind = "account"
	PromptScopeCase      PromptScopeKind = "case"
)

type PromptHistoryConfig struct {
	SchemaVersion int    `json:"schema_version"`
	OwnerID       string `json:"owner_id,omitempty"`
	MaxEntries    int    `json:"max_entries"`
	MaxBytes      int    `json:"max_bytes"`
	MaxAgeSeconds int64  `json:"max_age_seconds"`
}

type PromptHistoryInput struct {
	OwnerID     string
	Prompt      string
	Language    string
	Source      string
	SessionID   string
	ScopeKind   PromptScopeKind
	ScopeID     string
	RecordedAt  time.Time
	ContentKind string
}

// PromptHistoryEntry is local private data. It is never embedded in a
// lifecycle receipt, ledger entry, managed bundle, federation payload or
// release artifact.
type PromptHistoryEntry struct {
	SchemaVersion int             `json:"schema_version"`
	ID            string          `json:"id"`
	OwnerID       string          `json:"owner_id"`
	RecordedAt    time.Time       `json:"recorded_at"`
	Language      string          `json:"language"`
	Source        string          `json:"source"`
	SessionID     string          `json:"session_id"`
	ScopeKind     PromptScopeKind `json:"scope_kind"`
	ScopeID       string          `json:"scope_id"`
	SHA256        string          `json:"sha256"`
	ContentKind   string          `json:"content_kind"`
	Prompt        string          `json:"prompt"`
}

type PromptHistoryReceipt struct {
	ID         string          `json:"id"`
	OwnerID    string          `json:"owner_id"`
	RecordedAt time.Time       `json:"recorded_at"`
	Language   string          `json:"language"`
	Source     string          `json:"source"`
	SessionID  string          `json:"session_id"`
	ScopeKind  PromptScopeKind `json:"scope_kind"`
	ScopeID    string          `json:"scope_id"`
	SHA256     string          `json:"sha256"`
	Bytes      int             `json:"bytes"`
}

type PromptHistorySelectionLimits struct {
	OwnerID         string
	MaxCount        int
	MaxBytes        int
	MaxAge          time.Duration
	ScopeKind       PromptScopeKind
	ScopeID         string
	CurrentPrompt   string
	RelevanceKeys   []string
	CurrentLanguage string
}

type PromptHistorySelection struct {
	Entry   PromptHistoryEntry
	Score   int
	Reasons []string
}

var promptLanguagePattern = regexp.MustCompile(`^[A-Za-z]{2,8}(?:-[A-Za-z0-9]{2,8})?$`)

func DefaultPromptHistoryConfig() PromptHistoryConfig {
	return PromptHistoryConfig{SchemaVersion: PromptHistorySchemaVersion, MaxEntries: 100, MaxBytes: 256 << 10, MaxAgeSeconds: int64((90 * 24 * time.Hour).Seconds())}
}

func ConfigurePromptHistory(root string, config PromptHistoryConfig) error {
	if err := validatePromptHistoryConfig(config); err != nil {
		return err
	}
	return withPromptHistoryLock(root, func(entriesPath, configPath string) error {
		existingConfig, err := loadPromptHistoryConfig(configPath)
		if err != nil {
			return err
		}
		if existingConfig.OwnerID != "" {
			if config.OwnerID != "" && config.OwnerID != existingConfig.OwnerID {
				return errors.New("prompt history owner binding cannot be changed")
			}
			config.OwnerID = existingConfig.OwnerID
		}
		entries, err := readPromptHistoryEntries(entriesPath)
		if err != nil {
			return err
		}
		if config.OwnerID == "" && len(entries) > 0 {
			config.OwnerID = entries[0].OwnerID
		}
		if config.OwnerID != "" {
			for _, entry := range entries {
				if entry.OwnerID != config.OwnerID {
					return errors.New("prompt history owner binding is inconsistent")
				}
			}
		}
		entries = retainPromptHistory(entries, config, time.Now().UTC())
		if err := writePromptHistoryEntries(entriesPath, entries); err != nil {
			return err
		}
		return writePrivateJSON(configPath, config)
	})
}

func LoadPromptHistoryConfig(root string) (PromptHistoryConfig, error) {
	_, configPath, err := ensurePromptHistoryStore(root)
	if err != nil {
		return PromptHistoryConfig{}, err
	}
	return loadPromptHistoryConfig(configPath)
}

func loadPromptHistoryConfig(configPath string) (PromptHistoryConfig, error) {
	body, err := os.ReadFile(configPath)
	if err != nil {
		return PromptHistoryConfig{}, err
	}
	var config PromptHistoryConfig
	if err := json.Unmarshal(body, &config); err != nil {
		return PromptHistoryConfig{}, err
	}
	if err := validatePromptHistoryConfig(config); err != nil {
		return PromptHistoryConfig{}, err
	}
	return config, nil
}

func PromptHistoryEntriesPath(root string) string {
	return filepath.Join(root, filepath.FromSlash(PromptHistoryRelativePath))
}

func validatePromptHistoryConfig(config PromptHistoryConfig) error {
	if config.SchemaVersion != PromptHistorySchemaVersion || (config.OwnerID != "" && !observationIdentifier.MatchString(config.OwnerID)) || config.MaxEntries < 1 || config.MaxEntries > maximumPromptEntries || config.MaxBytes < 1 || config.MaxBytes > maximumPromptStoreBytes || config.MaxAgeSeconds < int64(time.Hour/time.Second) || config.MaxAgeSeconds > int64((10*365*24*time.Hour)/time.Second) {
		return errors.New("prompt history configuration is invalid")
	}
	return nil
}

func validatePromptHistoryInput(input PromptHistoryInput) error {
	if !observationIdentifier.MatchString(input.OwnerID) || !observationIdentifier.MatchString(input.SessionID) || !observationIdentifier.MatchString(input.ScopeID) {
		return errors.New("prompt history owner, session or scope ID is invalid")
	}
	if strings.TrimSpace(input.Prompt) == "" || len([]byte(input.Prompt)) > maximumPromptBytes {
		return errors.New("user prompt is empty or exceeds the per-entry bound")
	}
	if !promptLanguagePattern.MatchString(input.Language) {
		return errors.New("prompt history language is invalid")
	}
	if input.Source != "claude" && input.Source != "codex" && input.Source != "cli" && input.Source != "owner" {
		return errors.New("prompt history source is invalid")
	}
	if input.ContentKind != "" && input.ContentKind != "user_prompt" {
		return errors.New("prompt history accepts user prompts only")
	}
	if !validPromptScope(input.ScopeKind, input.ScopeID) {
		return errors.New("prompt history scope is invalid")
	}
	return nil
}

func validPromptScope(kind PromptScopeKind, id string) bool {
	if !observationIdentifier.MatchString(id) {
		return false
	}
	switch kind {
	case PromptScopeGlobal, PromptScopeWorkspace, PromptScopeAccount, PromptScopeCase:
		return true
	default:
		return false
	}
}

func ensurePromptHistoryStore(root string) (string, string, error) {
	if strings.TrimSpace(root) == "" {
		return "", "", errors.New("prompt history root is required")
	}
	rootPath, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	if err := ensurePromptDirectory(rootPath, false); err != nil {
		return "", "", err
	}
	ownerPath := filepath.Join(rootPath, "owner")
	if err := ensurePromptDirectory(ownerPath, true); err != nil {
		return "", "", err
	}
	historyPath := filepath.Join(ownerPath, "prompt-history")
	if err := ensurePromptDirectory(historyPath, true); err != nil {
		return "", "", err
	}
	entriesPath := filepath.Join(historyPath, "entries.jsonl")
	configPath := filepath.Join(historyPath, "config.json")
	if err := ensurePromptFile(entriesPath); err != nil {
		return "", "", err
	}
	if err := ensurePromptFile(configPath); err != nil {
		return "", "", err
	}
	if info, err := os.Stat(configPath); err == nil && info.Size() == 0 {
		if err := writePrivateJSON(configPath, DefaultPromptHistoryConfig()); err != nil {
			return "", "", err
		}
	}
	return entriesPath, configPath, nil
}

func ensurePromptDirectory(path string, private bool) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return err
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("prompt history path is not a private directory: %s", path)
	}
	if private {
		return os.Chmod(path, 0o700)
	}
	return nil
}

func ensurePromptFile(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		file, createErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if createErr != nil {
			return createErr
		}
		return file.Close()
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("prompt history path is not a private regular file: %s", path)
	}
	return os.Chmod(path, 0o600)
}

func RecordUserPrompt(root string, input PromptHistoryInput) (PromptHistoryReceipt, error) {
	if err := validatePromptHistoryInput(input); err != nil {
		return PromptHistoryReceipt{}, err
	}
	var receipt PromptHistoryReceipt
	err := withPromptHistoryLock(root, func(entriesPath, configPath string) error {
		config, err := loadPromptHistoryConfig(configPath)
		if err != nil {
			return err
		}
		entries, err := readPromptHistoryEntries(entriesPath)
		if err != nil {
			return err
		}
		if config.OwnerID == "" && len(entries) > 0 {
			config.OwnerID = entries[0].OwnerID
		}
		if config.OwnerID != "" && config.OwnerID != input.OwnerID {
			return errors.New("prompt history owner does not match this store")
		}
		if config.OwnerID == "" {
			config.OwnerID = input.OwnerID
			if err := writePrivateJSON(configPath, config); err != nil {
				return err
			}
		}
		now := time.Now().UTC()
		if input.RecordedAt.IsZero() {
			input.RecordedAt = now
		}
		if input.RecordedAt.After(now.Add(5 * time.Minute)) {
			return errors.New("prompt history timestamp is too far in the future")
		}
		if now.Sub(input.RecordedAt) > time.Duration(config.MaxAgeSeconds)*time.Second {
			return errors.New("prompt is outside configured history retention")
		}
		entry := PromptHistoryEntry{
			SchemaVersion: PromptHistorySchemaVersion,
			OwnerID:       input.OwnerID, RecordedAt: input.RecordedAt.UTC(), Language: input.Language,
			Source: input.Source, SessionID: input.SessionID, ScopeKind: input.ScopeKind,
			ScopeID: input.ScopeID, SHA256: digest(input.Prompt), ContentKind: "user_prompt", Prompt: input.Prompt,
		}
		entry.ID = "prompt-" + digest(entry.OwnerID + "\x00" + entry.SessionID + "\x00" + entry.RecordedAt.Format(time.RFC3339Nano) + "\x00" + entry.SHA256)[:24]
		for _, existing := range entries {
			if existing.ID == entry.ID {
				receipt = promptHistoryReceipt(existing)
				return nil
			}
		}
		entries = append(entries, entry)
		entries = retainPromptHistory(entries, config, now)
		found := false
		for _, retained := range entries {
			if retained.ID == entry.ID {
				found = true
				break
			}
		}
		if !found {
			return errors.New("prompt was excluded by configured history retention")
		}
		if err := writePromptHistoryEntries(entriesPath, entries); err != nil {
			return err
		}
		receipt = promptHistoryReceipt(entry)
		return nil
	})
	return receipt, err
}

func InspectPromptHistory(root string) ([]PromptHistoryReceipt, error) {
	entriesPath, _, err := ensurePromptHistoryStore(root)
	if err != nil {
		return nil, err
	}
	entries, err := readPromptHistoryEntries(entriesPath)
	if err != nil {
		return nil, err
	}
	result := make([]PromptHistoryReceipt, 0, len(entries))
	for _, entry := range entries {
		result = append(result, promptHistoryReceipt(entry))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].RecordedAt.Before(result[j].RecordedAt) })
	return result, nil
}

func ExportPromptHistory(root string) ([]PromptHistoryEntry, error) {
	entriesPath, _, err := ensurePromptHistoryStore(root)
	if err != nil {
		return nil, err
	}
	entries, err := readPromptHistoryEntries(entriesPath)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].RecordedAt.Before(entries[j].RecordedAt) })
	return entries, nil
}

func DeletePromptHistory(root, id string, confirmed bool) error {
	if !confirmed {
		return ErrConfirmationRequired
	}
	return withPromptHistoryLock(root, func(entriesPath, _ string) error {
		entries, err := readPromptHistoryEntries(entriesPath)
		if err != nil {
			return err
		}
		filtered := entries[:0]
		found := false
		for _, entry := range entries {
			if entry.ID == id {
				found = true
				continue
			}
			filtered = append(filtered, entry)
		}
		if !found {
			return os.ErrNotExist
		}
		return writePromptHistoryEntries(entriesPath, filtered)
	})
}

func ResetPromptHistory(root string, confirmed bool) error {
	if !confirmed {
		return ErrConfirmationRequired
	}
	return withPromptHistoryLock(root, func(entriesPath, _ string) error {
		return writePromptHistoryEntries(entriesPath, nil)
	})
}

func SelectPromptHistory(root string, limits PromptHistorySelectionLimits, now time.Time) ([]PromptHistoryEntry, error) {
	selected, err := SelectRelevantPromptHistory(root, limits, now)
	if err != nil {
		return nil, err
	}
	entries := make([]PromptHistoryEntry, 0, len(selected))
	for _, item := range selected {
		entries = append(entries, item.Entry)
	}
	return entries, nil
}

func SelectRelevantPromptHistory(root string, limits PromptHistorySelectionLimits, now time.Time) ([]PromptHistorySelection, error) {
	entriesPath, _, err := ensurePromptHistoryStore(root)
	if err != nil {
		return nil, err
	}
	config, err := LoadPromptHistoryConfig(root)
	if err != nil {
		return nil, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if limits.MaxCount <= 0 || limits.MaxCount > config.MaxEntries || limits.MaxCount > maximumPromptPacketCount {
		limits.MaxCount = minInt(config.MaxEntries, maximumPromptPacketCount)
	}
	if limits.MaxBytes <= 0 || limits.MaxBytes > config.MaxBytes || limits.MaxBytes > maximumPromptPacketBytes {
		limits.MaxBytes = minInt(config.MaxBytes, maximumPromptPacketBytes)
	}
	if limits.MaxAge <= 0 || limits.MaxAge > time.Duration(config.MaxAgeSeconds)*time.Second {
		limits.MaxAge = time.Duration(config.MaxAgeSeconds) * time.Second
	}
	if !validPromptScope(limits.ScopeKind, limits.ScopeID) {
		return nil, errors.New("prompt history selection scope is invalid")
	}
	if !observationIdentifier.MatchString(limits.OwnerID) {
		return nil, errors.New("prompt history selection owner is invalid")
	}
	entries, err := readPromptHistoryEntries(entriesPath)
	if err != nil {
		return nil, err
	}
	if config.OwnerID != "" && config.OwnerID != limits.OwnerID {
		return nil, errors.New("prompt history selection owner does not match this store")
	}
	candidates := make([]PromptHistorySelection, 0, len(entries))
	for _, entry := range entries {
		if entry.OwnerID != limits.OwnerID || now.Sub(entry.RecordedAt) > limits.MaxAge || !promptScopeMatches(entry, limits.ScopeKind, limits.ScopeID) {
			continue
		}
		score, reasons := promptRelevance(entry.Prompt, limits.CurrentPrompt, limits.RelevanceKeys)
		candidates = append(candidates, PromptHistorySelection{Entry: entry, Score: score, Reasons: reasons})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		if candidates[i].Entry.RecordedAt.Equal(candidates[j].Entry.RecordedAt) {
			return candidates[i].Entry.ID > candidates[j].Entry.ID
		}
		return candidates[i].Entry.RecordedAt.After(candidates[j].Entry.RecordedAt)
	})
	selected := make([]PromptHistorySelection, 0, limits.MaxCount)
	bytes := 0
	for _, candidate := range candidates {
		if len(selected) >= limits.MaxCount || bytes+len(candidate.Entry.Prompt) > limits.MaxBytes {
			continue
		}
		selected = append(selected, candidate)
		bytes += len(candidate.Entry.Prompt)
	}
	return selected, nil
}

func promptRelevance(prompt, current string, keys []string) (int, []string) {
	promptTokens := tokenSet(prompt)
	currentTokens := tokenSet(current)
	score := 0
	reasons := []string{}
	for token := range currentTokens {
		if promptTokens[token] {
			score += 10
		}
	}
	if score > 0 {
		reasons = append(reasons, "current_token_overlap")
	}
	for _, key := range keys {
		keyTokens := tokenSet(key)
		matched := false
		for token := range keyTokens {
			if promptTokens[token] {
				score += 100
				matched = true
			}
		}
		if matched {
			reasons = append(reasons, "explicit_key_overlap:"+key)
		}
	}
	if len(reasons) == 0 {
		reasons = []string{"recent_fallback"}
	}
	sort.Strings(reasons)
	return score, reasons
}

func tokenSet(value string) map[string]bool {
	set := map[string]bool{}
	for _, token := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}) {
		if len(token) >= 3 {
			set[token] = true
		}
	}
	return set
}

func promptScopeMatches(entry PromptHistoryEntry, kind PromptScopeKind, id string) bool {
	return (entry.ScopeKind == PromptScopeGlobal && kind != PromptScopeGlobal) || (entry.ScopeKind == kind && entry.ScopeID == id)
}

func retainPromptHistory(entries []PromptHistoryEntry, config PromptHistoryConfig, now time.Time) []PromptHistoryEntry {
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].RecordedAt.After(entries[j].RecordedAt) })
	retained := make([]PromptHistoryEntry, 0, minInt(len(entries), config.MaxEntries))
	bytes := 0
	for _, entry := range entries {
		if now.Sub(entry.RecordedAt) > time.Duration(config.MaxAgeSeconds)*time.Second || len(retained) >= config.MaxEntries || bytes+len(entry.Prompt) > config.MaxBytes {
			continue
		}
		retained = append(retained, entry)
		bytes += len(entry.Prompt)
	}
	sort.SliceStable(retained, func(i, j int) bool { return retained[i].RecordedAt.Before(retained[j].RecordedAt) })
	return retained
}

func readPromptHistoryEntries(path string) ([]PromptHistoryEntry, error) {
	if err := ensurePromptFile(path); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), maximumPromptBytes+4096)
	entries := []PromptHistoryEntry{}
	ownerID := ""
	for scanner.Scan() {
		var entry PromptHistoryEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, err
		}
		if err := validatePromptHistoryEntry(entry); err != nil {
			return nil, err
		}
		if ownerID == "" {
			ownerID = entry.OwnerID
		} else if ownerID != entry.OwnerID {
			return nil, errors.New("prompt history contains multiple owners")
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func writePromptHistoryEntries(path string, entries []PromptHistoryEntry) error {
	if err := ensurePromptFile(path); err != nil {
		return err
	}
	var body strings.Builder
	for _, entry := range entries {
		encoded, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		body.Write(encoded)
		body.WriteByte('\n')
	}
	return atomicPrivateWrite(path, []byte(body.String()))
}

func validatePromptHistoryEntry(entry PromptHistoryEntry) error {
	if entry.SchemaVersion != PromptHistorySchemaVersion || !observationIdentifier.MatchString(entry.ID) || entry.ContentKind != "user_prompt" {
		return errors.New("prompt history entry is not a user prompt")
	}
	if err := validatePromptHistoryInput(PromptHistoryInput{OwnerID: entry.OwnerID, Prompt: entry.Prompt, Language: entry.Language, Source: entry.Source, SessionID: entry.SessionID, ScopeKind: entry.ScopeKind, ScopeID: entry.ScopeID, RecordedAt: entry.RecordedAt, ContentKind: entry.ContentKind}); err != nil {
		return err
	}
	if len(entry.SHA256) != 64 || entry.SHA256 != digest(entry.Prompt) {
		return errors.New("prompt history hash does not match the body")
	}
	return nil
}

func promptHistoryReceipt(entry PromptHistoryEntry) PromptHistoryReceipt {
	return PromptHistoryReceipt{ID: entry.ID, OwnerID: entry.OwnerID, RecordedAt: entry.RecordedAt, Language: entry.Language, Source: entry.Source, SessionID: entry.SessionID, ScopeKind: entry.ScopeKind, ScopeID: entry.ScopeID, SHA256: entry.SHA256, Bytes: len([]byte(entry.Prompt))}
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
