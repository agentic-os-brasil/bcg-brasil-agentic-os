package memory

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	AttestedCaptureSchemaVersion = 2
	SkillRouteSanitizerID        = "skill-selection-metadata-v1"
	captureAttestationKeyBytes   = 32
)

var trustedCaptureProducers = map[string]string{
	"claude.context-injection": SkillRouteSanitizerID,
	"codex.context-injection":  SkillRouteSanitizerID,
}

type CaptureAttestor struct {
	Root string
}

func (attestor CaptureAttestor) Seal(capture Capture) (Capture, error) {
	if capture.SchemaVersion != 0 && capture.SchemaVersion != AttestedCaptureSchemaVersion {
		return Capture{}, errors.New("unsupported capture attestation schema")
	}
	capture.SchemaVersion = AttestedCaptureSchemaVersion
	if err := validateAttestedCaptureEnvelope(capture); err != nil {
		return Capture{}, err
	}
	key, err := attestor.key(capture.WorkspaceID, true)
	if err != nil {
		return Capture{}, err
	}
	capture.Attestation = captureMAC(key, capture)
	return capture, nil
}

func (attestor CaptureAttestor) Verify(capture Capture) error {
	if capture.SchemaVersion == OwnedCaptureSchemaVersion {
		return attestor.verifyOwned(capture)
	}
	if err := validateAttestedCaptureEnvelope(capture); err != nil {
		return err
	}
	if len(capture.Attestation) != 64 {
		return errors.New("capture attestation is missing")
	}
	provided, err := hex.DecodeString(capture.Attestation)
	if err != nil {
		return errors.New("capture attestation is invalid")
	}
	key, err := attestor.key(capture.WorkspaceID, false)
	if err != nil {
		return err
	}
	expected, _ := hex.DecodeString(captureMAC(key, capture))
	if !hmac.Equal(provided, expected) {
		return errors.New("capture attestation does not match its trusted producer envelope")
	}
	return nil
}

func validateAttestedCaptureEnvelope(capture Capture) error {
	if capture.SchemaVersion != AttestedCaptureSchemaVersion || !capture.Sanitized || capture.WorkspaceID == "" || capture.RecordedAt.IsZero() || strings.TrimSpace(capture.Kind) != "skill_route" || strings.TrimSpace(capture.Text) == "" || len(capture.SourceDigest) != 64 {
		return errors.New("capture-v2 requires a sanitized, digest-bound skill route")
	}
	if _, err := hex.DecodeString(capture.SourceDigest); err != nil {
		return errors.New("capture-v2 source digest is invalid")
	}
	wanted, trusted := trustedCaptureProducers[capture.ProducerID]
	if !trusted || capture.SanitizerID != wanted {
		return errors.New("capture-v2 producer or sanitizer is not trusted")
	}
	return nil
}

func captureMAC(key []byte, capture Capture) string {
	copy := capture
	copy.Attestation = ""
	body, _ := json.Marshal(copy)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("bcgos-memory-capture-v2\x00"))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func (attestor CaptureAttestor) key(workspaceID string, create bool) ([]byte, error) {
	if strings.TrimSpace(attestor.Root) == "" || !filepath.IsAbs(attestor.Root) {
		return nil, errors.New("absolute memory root is required for capture attestation")
	}
	if err := validateWorkspaceID(workspaceID); err != nil {
		return nil, err
	}
	directory := filepath.Join(attestor.Root, "workspaces", workspaceID, ".attestation")
	if create {
		if err := memoryMkdirAll(directory, 0700); err != nil {
			return nil, err
		}
	}
	path := filepath.Join(directory, "capture-v2.key")
	key, err := memoryReadFile(path, captureAttestationKeyBytes)
	if err == nil {
		if len(key) != captureAttestationKeyBytes {
			return nil, errors.New("capture attestation key has an invalid length")
		}
		return key, nil
	}
	if !create || !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key = make([]byte, captureAttestationKeyBytes)
	if _, err = rand.Read(key); err != nil {
		return nil, err
	}
	if err = memoryPublish(path, key, true); errors.Is(err, os.ErrExist) {
		return attestor.key(workspaceID, false)
	}
	return key, err
}
