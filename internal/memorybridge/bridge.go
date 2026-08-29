package memorybridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const (
	PreviewBytes        = 4 << 10
	MaxDirectoryEntries = 256
	MaxCandidates       = 128
	MaxCandidateBytes   = 32 << 10
	MaxDiscoveredBytes  = 512 << 10
	MaxSelectedBytes    = 64 << 10
)

var (
	workspacePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
	layers           = []string{"recent", "weekly", "medium-term", "lifetime"}
)

type Service struct {
	DataRoot string
	Engine   *memory.Engine
}

type Candidate struct {
	ID          string `json:"candidate_id"`
	LegacyLayer string `json:"legacy_layer"`
	Bytes       int    `json:"bytes"`
	SHA256      string `json:"sha256"`
}

type InspectReport struct {
	SchemaVersion int         `json:"schema_version"`
	Candidates    []Candidate `json:"candidates"`
}

type Preview struct {
	SchemaVersion int    `json:"schema_version"`
	CandidateID   string `json:"candidate_id"`
	LegacyLayer   string `json:"legacy_layer"`
	Content       string `json:"content"`
	Truncated     bool   `json:"truncated"`
}

type ApplyRequest struct {
	WorkspaceID         string
	CandidateIDs        []string
	AttestedTargetScope bool
	Confirmed           bool
}

type candidateRecord struct {
	Candidate
	content []byte
}

func (service *Service) Inspect(ctx context.Context, workspaceID string) (InspectReport, error) {
	if err := service.validate(workspaceID); err != nil {
		return InspectReport{}, err
	}
	records, err := service.discover(ctx)
	if err != nil {
		return InspectReport{}, err
	}
	report := InspectReport{SchemaVersion: 1, Candidates: make([]Candidate, len(records))}
	for index, record := range records {
		report.Candidates[index] = record.Candidate
	}
	return report, nil
}

func (service *Service) Preview(ctx context.Context, workspaceID, candidateID string) (Preview, error) {
	if err := service.validate(workspaceID); err != nil {
		return Preview{}, err
	}
	if !validCandidateID(candidateID) {
		return Preview{}, errors.New("legacy Hub candidate is invalid or no longer available")
	}
	records, err := service.discover(ctx)
	if err != nil {
		return Preview{}, err
	}
	for _, record := range records {
		if record.ID != candidateID {
			continue
		}
		body := record.content
		truncated := len(body) > PreviewBytes
		if truncated {
			body = append([]byte(nil), body[:PreviewBytes]...)
			for len(body) > 0 && !utf8.Valid(body) {
				body = body[:len(body)-1]
			}
		}
		return Preview{SchemaVersion: 1, CandidateID: record.ID, LegacyLayer: record.LegacyLayer, Content: string(body), Truncated: truncated}, nil
	}
	return Preview{}, errors.New("legacy Hub candidate is invalid or no longer available")
}

func (service *Service) Apply(ctx context.Context, request ApplyRequest) (memory.LegacyHubImportResult, error) {
	if err := service.validate(request.WorkspaceID); err != nil {
		return memory.LegacyHubImportResult{}, err
	}
	if !request.Confirmed || !request.AttestedTargetScope {
		return memory.LegacyHubImportResult{}, errors.New("legacy Hub import requires explicit target-scope attestation and confirmation")
	}
	if len(request.CandidateIDs) == 0 || len(request.CandidateIDs) > 16 {
		return memory.LegacyHubImportResult{}, errors.New("legacy Hub import candidate selection is outside its limit")
	}
	records, err := service.discover(ctx)
	if err != nil {
		return memory.LegacyHubImportResult{}, err
	}
	available := make(map[string]candidateRecord, len(records))
	for _, record := range records {
		available[record.ID] = record
	}
	seen := make(map[string]bool, len(request.CandidateIDs))
	sources := make([]memory.LegacyHubSource, 0, len(request.CandidateIDs))
	selectedBytes := 0
	for _, id := range request.CandidateIDs {
		if !validCandidateID(id) || seen[id] {
			return memory.LegacyHubImportResult{}, errors.New("legacy Hub candidate selection is invalid")
		}
		seen[id] = true
		record, ok := available[id]
		if !ok {
			return memory.LegacyHubImportResult{}, errors.New("legacy Hub candidate is invalid or no longer available")
		}
		selectedBytes += len(record.content)
		if selectedBytes > MaxSelectedBytes {
			return memory.LegacyHubImportResult{}, errors.New("legacy Hub selected content exceeds its byte limit")
		}
		sources = append(sources, memory.LegacyHubSource{CandidateID: record.ID, LegacyLayer: record.LegacyLayer, SHA256: record.SHA256, Content: append([]byte(nil), record.content...)})
	}
	result, err := service.Engine.ImportLegacyHub(ctx, memory.LegacyHubImportRequest{WorkspaceID: request.WorkspaceID, Sources: sources})
	if err != nil {
		return memory.LegacyHubImportResult{}, errors.New("canonical legacy Hub memory import failed safely before a partial result became visible")
	}
	return result, nil
}

func (service *Service) validate(workspaceID string) error {
	if service == nil || service.Engine == nil || strings.TrimSpace(service.DataRoot) == "" {
		return errors.New("legacy Hub bridge is unavailable")
	}
	dataRoot, dataErr := filepath.Abs(filepath.Clean(service.DataRoot))
	engineRoot, engineErr := filepath.Abs(filepath.Clean(service.Engine.Root))
	if dataErr != nil || engineErr != nil || dataRoot != engineRoot {
		return errors.New("legacy Hub bridge source and target authorities do not agree")
	}
	if !workspacePattern.MatchString(workspaceID) {
		return errors.New("legacy Hub target workspace is invalid")
	}
	return nil
}

func (service *Service) discover(ctx context.Context) ([]candidateRecord, error) {
	var records []candidateRecord
	identities := map[string]bool{}
	entryCount, totalBytes := 0, 0
	for _, layer := range layers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		root, canonicalErr := scheduler.CanonicalPrivatePath(filepath.Join(service.DataRoot, "memory", layer))
		if canonicalErr != nil {
			return nil, errors.New("legacy Hub memory source boundary is invalid")
		}
		info, err := os.Lstat(root)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("legacy Hub memory source boundary is invalid")
		}
		if err := scheduler.ValidatePrivateDirectory(root); err != nil {
			return nil, errors.New("legacy Hub memory source boundary is not private and intact")
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, errors.New("legacy Hub memory source could not be inspected")
		}
		entryCount += len(entries)
		if entryCount > MaxDirectoryEntries {
			return nil, errors.New("legacy Hub memory source exceeds its entry limit")
		}
		for _, entry := range entries {
			entryInfo, err := entry.Info()
			if err != nil {
				return nil, errors.New("legacy Hub memory entry could not be inspected")
			}
			if entryInfo.Mode()&os.ModeSymlink != 0 {
				return nil, errors.New("legacy Hub memory source contains a symlink")
			}
			if !entryInfo.Mode().IsRegular() || filepath.Ext(entry.Name()) != ".md" {
				continue
			}
			if entryInfo.Size() <= 0 || entryInfo.Size() > MaxCandidateBytes {
				return nil, errors.New("legacy Hub memory candidate exceeds its file limit")
			}
			body, err := scheduler.ReadPrivateFile(filepath.Join(root, entry.Name()), MaxCandidateBytes)
			if err != nil {
				return nil, errors.New("legacy Hub memory candidate could not be read")
			}
			if !utf8.Valid(body) {
				return nil, errors.New("legacy Hub memory candidate is not valid UTF-8")
			}
			digest := sha256.Sum256(body)
			digestText := hex.EncodeToString(digest[:])
			identity := sha256.Sum256([]byte(layer + "\x00" + entry.Name() + "\x00" + digestText))
			candidateID := hex.EncodeToString(identity[:16])
			if identities[candidateID] {
				return nil, errors.New("legacy Hub candidate identity collision")
			}
			identities[candidateID] = true
			records = append(records, candidateRecord{Candidate: Candidate{ID: candidateID, LegacyLayer: layer, Bytes: len(body), SHA256: digestText}, content: body})
			totalBytes += len(body)
			if len(records) > MaxCandidates || totalBytes > MaxDiscoveredBytes {
				return nil, errors.New("legacy Hub memory candidates exceed discovery limits")
			}
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	return records, nil
}

func validCandidateID(value string) bool {
	if len(value) != 32 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
