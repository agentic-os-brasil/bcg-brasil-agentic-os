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
	if request.Method != http.MethodPost || (request.URL.Path != "/federation/v1/batches" && request.URL.Path != "/federation/v1/portable-skills") {
		http.NotFound(writer, request)
		return
	}
	maximumBody := int64(32 << 10)
	if request.URL.Path == "/federation/v1/portable-skills" {
		maximumBody = MaximumPortableSkillBytes + (16 << 10)
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maximumBody)
	if request.URL.Path == "/federation/v1/portable-skills" {
		installationID := request.Header.Get("X-Maestro-Installation-ID")
		packageValue, err := ParsePortableSkill(request.Body)
		if err != nil {
			http.Error(writer, "invalid portable skill package", http.StatusBadRequest)
			return
		}
		accepted, err := bridge.Inbox.AcceptPortable(installationID, packageValue)
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
		} else {
			writer.WriteHeader(http.StatusOK)
		}
		return
	}
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

// AcceptPortable stores an explicitly born-portable package without retaining
// the submitting installation identity. Identity is used only at the ingress
// gate; central curation sees the package and manifest, never a workspace.
func (inbox CentralInbox) AcceptPortable(installationID string, packageValue PortableSkillPackage) (bool, error) {
	if strings.TrimSpace(inbox.Root) == "" {
		return false, errors.New("central federation inbox root is required")
	}
	if !installationIDPattern.MatchString(installationID) || !inbox.AllowedInstallations[installationID] {
		return false, ErrUntrustedInstallation
	}
	if err := packageValue.Validate(); err != nil {
		return false, err
	}
	if err := os.MkdirAll(inbox.portableRoot(), 0o700); err != nil {
		return false, err
	}
	encoded, err := json.Marshal(packageValue)
	if err != nil {
		return false, err
	}
	digest := sha256.Sum256(encoded)
	path := filepath.Join(inbox.portableRoot(), hex.EncodeToString(digest[:])+".json")
	if err := writeNewJSON(path, packageValue); err != nil {
		if errors.Is(err, os.ErrExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (inbox CentralInbox) PortableSkills() ([]PortableSkillPackage, error) {
	if strings.TrimSpace(inbox.Root) == "" {
		return nil, errors.New("central federation inbox root is required")
	}
	entries, err := os.ReadDir(inbox.portableRoot())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	packages := make([]PortableSkillPackage, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			return nil, fmt.Errorf("invalid central portable skill entry %q", entry.Name())
		}
		var packageValue PortableSkillPackage
		if err := readStrictJSON(filepath.Join(inbox.portableRoot(), entry.Name()), &packageValue); err != nil {
			return nil, err
		}
		if err := packageValue.Validate(); err != nil {
			return nil, err
		}
		packages = append(packages, packageValue)
	}
	sort.Slice(packages, func(left, right int) bool { return packages[left].Manifest.SkillID < packages[right].Manifest.SkillID })
	return packages, nil
}

func (inbox CentralInbox) batchRoot() string    { return filepath.Join(inbox.Root, "batches") }
func (inbox CentralInbox) portableRoot() string { return filepath.Join(inbox.Root, "portable-skills") }

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
