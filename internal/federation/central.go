package federation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const minimumCohortEvidence = 2

// CentralInbox is the bridge-owned durable inbox. Pilot devices can submit a
// Batch only when their opaque installation has been provisioned centrally.
// The allowlist is bridge configuration; it never returns to a device.
type CentralInbox struct {
	Root                 string
	AllowedInstallations map[string]bool
}

type CentralDigest struct {
	SchemaVersion int              `json:"schema_version"`
	Period        string           `json:"period"`
	BatchCount    int              `json:"batch_count"`
	Signals       []SignalTally    `json:"signals"`
	Candidates    []CandidateTally `json:"candidates"`
}

type SignalTally struct {
	Signal Signal `json:"signal"`
	Count  int    `json:"count"`
}

type CandidateTally struct {
	Candidate SkillCandidate `json:"candidate"`
	Count     int            `json:"count"`
}

// CentralBridge exposes the one endpoint used by the pilot-device HTTP
// adapter. It receives no GitHub credential and returns no stored payload.
type CentralBridge struct {
	Inbox CentralInbox
}

func (bridge CentralBridge) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost || request.URL.Path != "/federation/v1/batches" {
		http.NotFound(writer, request)
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 32<<10)
	batch, err := Parse(request.Body)
	if err != nil {
		http.Error(writer, "invalid federation batch", http.StatusBadRequest)
		return
	}
	accepted, err := bridge.Inbox.Accept(batch)
	if err != nil {
		if errors.Is(err, ErrUntrustedInstallation) {
			http.Error(writer, "untrusted installation", http.StatusForbidden)
			return
		}
		http.Error(writer, "federation bridge unavailable", http.StatusServiceUnavailable)
		return
	}
	if accepted {
		writer.WriteHeader(http.StatusAccepted)
		return
	}
	writer.WriteHeader(http.StatusOK)
}

var ErrUntrustedInstallation = errors.New("federation installation is not trusted by the central bridge")

func (inbox CentralInbox) Accept(batch Batch) (bool, error) {
	if strings.TrimSpace(inbox.Root) == "" {
		return false, errors.New("central federation inbox root is required")
	}
	if err := batch.Validate(); err != nil {
		return false, err
	}
	if !inbox.AllowedInstallations[batch.InstallationID] {
		return false, ErrUntrustedInstallation
	}
	if err := os.MkdirAll(inbox.batchRoot(), 0o700); err != nil {
		return false, err
	}
	encoded, err := json.Marshal(batch)
	if err != nil {
		return false, err
	}
	digest := sha256.Sum256(encoded)
	path := filepath.Join(inbox.batchRoot(), hex.EncodeToString(digest[:])+".json")
	if err := writeNewJSON(path, batch); err != nil {
		if errors.Is(err, os.ErrExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (inbox CentralInbox) Digest(period string) (CentralDigest, error) {
	if strings.TrimSpace(inbox.Root) == "" || !periodPattern.MatchString(period) {
		return CentralDigest{}, errors.New("invalid central federation digest request")
	}
	entries, err := os.ReadDir(inbox.batchRoot())
	if errors.Is(err, os.ErrNotExist) {
		return CentralDigest{SchemaVersion: SchemaVersion, Period: period}, nil
	}
	if err != nil {
		return CentralDigest{}, err
	}
	signalCounts := map[string]SignalTally{}
	candidateCounts := map[string]CandidateTally{}
	batchCount := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			return CentralDigest{}, fmt.Errorf("invalid central federation inbox entry %q", entry.Name())
		}
		var batch Batch
		if err := readStrictJSON(filepath.Join(inbox.batchRoot(), entry.Name()), &batch); err != nil {
			return CentralDigest{}, err
		}
		if err := batch.Validate(); err != nil {
			return CentralDigest{}, err
		}
		if batch.Period != period {
			continue
		}
		batchCount++
		for _, signal := range batch.Signals {
			key, err := tallyKey(signal)
			if err != nil {
				return CentralDigest{}, err
			}
			tally := signalCounts[key]
			tally.Signal = signal
			tally.Count++
			signalCounts[key] = tally
		}
		for _, candidate := range batch.Candidates {
			key, err := tallyKey(candidate)
			if err != nil {
				return CentralDigest{}, err
			}
			tally := candidateCounts[key]
			tally.Candidate = candidate
			tally.Count++
			candidateCounts[key] = tally
		}
	}
	digest := CentralDigest{SchemaVersion: SchemaVersion, Period: period, BatchCount: batchCount}
	for _, tally := range signalCounts {
		digest.Signals = append(digest.Signals, tally)
	}
	for _, tally := range candidateCounts {
		digest.Candidates = append(digest.Candidates, tally)
	}
	sort.Slice(digest.Signals, func(left, right int) bool {
		return signalDigestKey(digest.Signals[left].Signal) < signalDigestKey(digest.Signals[right].Signal)
	})
	sort.Slice(digest.Candidates, func(left, right int) bool {
		return digest.Candidates[left].Candidate.Fingerprint < digest.Candidates[right].Candidate.Fingerprint
	})
	return digest, nil
}

func (inbox CentralInbox) batchRoot() string { return filepath.Join(inbox.Root, "batches") }

func tallyKey(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func signalDigestKey(signal Signal) string {
	key, _ := tallyKey(signal)
	return key
}
