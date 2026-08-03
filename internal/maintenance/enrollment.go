package maintenance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

const EnrollmentSchemaVersion = 2

var uidValuePattern = regexp.MustCompile(`^[0-9]+$`)

type Activation struct {
	JobID               string `json:"job_id"`
	QualificationDigest string `json:"qualification_digest"`
}

type CanaryEnrollment struct {
	SchemaVersion    int          `json:"schema_version"`
	WorkspaceID      string       `json:"workspace_id"`
	AgentID          string       `json:"agent_id"`
	Home             string       `json:"home"`
	Executable       string       `json:"executable"`
	UID              string       `json:"uid"`
	Timezone         string       `json:"timezone"`
	LaunchAgentLabel string       `json:"launch_agent_label"`
	Mode             string       `json:"mode"`
	EnrolledAt       time.Time    `json:"enrolled_at"`
	Activated        []Activation `json:"activated"`
}

func QualificationDigest(jobID string) string {
	digest := sha256.Sum256([]byte("darwin-canary-handler-v1\x00" + jobID))
	return hex.EncodeToString(digest[:])
}

func (enrollment CanaryEnrollment) Validate() error {
	if enrollment.SchemaVersion != EnrollmentSchemaVersion || !commandIDPattern.MatchString(enrollment.WorkspaceID) || enrollment.AgentID != "darwin" || !filepath.IsAbs(enrollment.Home) || !filepath.IsAbs(enrollment.Executable) || !uidValuePattern.MatchString(enrollment.UID) || enrollment.Timezone == "" || enrollment.LaunchAgentLabel != "com.bcg.maestro.maintenance" || enrollment.EnrolledAt.IsZero() {
		return errors.New("invalid Darwin Canary enrollment header")
	}
	if _, err := time.LoadLocation(enrollment.Timezone); err != nil {
		return errors.New("invalid Darwin Canary IANA timezone")
	}
	if enrollment.Mode != "native" && enrollment.Mode != "filesystem_only" {
		return errors.New("invalid Darwin Canary enrollment mode")
	}
	seen := map[string]bool{}
	for _, activation := range enrollment.Activated {
		if !validID(activation.JobID) || (activation.JobID != "darwin-housekeeping-daily" && activation.JobID != "darwin-deep-weekly" && activation.JobID != MemoryCheckpointJobID && activation.JobID != MemoryLightDreamJobID) || activation.QualificationDigest != QualificationDigest(activation.JobID) || seen[activation.JobID] {
			return errors.New("invalid or duplicate Darwin Canary activation")
		}
		seen[activation.JobID] = true
	}
	return nil
}

func SaveCanaryEnrollment(root string, enrollment CanaryEnrollment) error {
	if err := enrollment.Validate(); err != nil {
		return err
	}
	directory, err := ensurePrivateTree(root, "maintenance")
	if err != nil {
		return err
	}
	return writeAtomicJSON(filepath.Join(directory, "canary-enrollment.json"), enrollment)
}

func LoadCanaryEnrollment(root string) (CanaryEnrollment, error) {
	directory, err := ensurePrivateTree(root, "maintenance")
	if err != nil {
		return CanaryEnrollment{}, err
	}
	file, err := os.Open(filepath.Join(directory, "canary-enrollment.json"))
	if err != nil {
		return CanaryEnrollment{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var enrollment CanaryEnrollment
	if err := decoder.Decode(&enrollment); err != nil {
		return CanaryEnrollment{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return CanaryEnrollment{}, errors.New("Canary enrollment contains multiple JSON values")
		}
		return CanaryEnrollment{}, err
	}
	return enrollment, enrollment.Validate()
}

func DeleteCanaryEnrollment(root string) error {
	directory, err := ensurePrivateTree(root, "maintenance")
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(directory, "canary-enrollment.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func writeAtomicJSON(path string, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	body = append(body, '\n')
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".enrollment-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err == nil {
		_, err = temporary.Write(body)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("persist Canary enrollment: %w", err)
	}
	return nil
}

func ActivationMaps(enrollment CanaryEnrollment) (map[string]string, []string) {
	qualified := make(map[string]string, len(enrollment.Activated))
	activated := make([]string, 0, len(enrollment.Activated))
	for _, activation := range enrollment.Activated {
		qualified[activation.JobID] = activation.QualificationDigest
		activated = append(activated, activation.JobID)
	}
	return qualified, activated
}
