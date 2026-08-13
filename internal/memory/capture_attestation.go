package memory

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return nil, err
		}
	}
	if info, err := os.Lstat(directory); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("capture attestation directory is not a private directory")
	}
	path := filepath.Join(directory, "capture-v2.key")
	read := func() ([]byte, error) {
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		// Unix mode bits are not an authority on Windows: Go synthesises FileMode
		// from the read-only attribute, so a key written 0600 reports 0666 there.
		// See internal/actionconfirmation/store.go (loadOrCreateKey) for the same
		// guard and the fuller rationale.
		permissive := runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || permissive {
			return nil, errors.New("capture attestation key is not a private regular file")
		}
		key, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if len(key) != captureAttestationKeyBytes {
			return nil, errors.New("capture attestation key has an invalid length")
		}
		return key, nil
	}
	if key, err := read(); err == nil {
		return key, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if !create {
		return nil, errors.New("capture attestation key is unavailable")
	}
	key := make([]byte, captureAttestationKeyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return read()
	}
	if err != nil {
		return nil, err
	}
	if _, err = file.Write(key); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("persist capture attestation key: %w", err)
	}
	if syncErr := syncDirectory(directory); syncErr != nil {
		return nil, fmt.Errorf("sync capture attestation directory: %w", syncErr)
	}
	return key, nil
}
