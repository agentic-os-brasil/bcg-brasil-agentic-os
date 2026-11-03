package memory

import "time"

// ActiveArtifact reads an artifact through the canonical visibility-aware
// manifest. It does not initialize state or expose a caller-supplied path.
func (engine *Engine) ActiveArtifact(workspaceID, key string) (Artifact, string, error) {
	if err := engine.validateRuntime(workspaceID, false); err != nil {
		return Artifact{}, "", err
	}
	artifact, path, err := engine.readArtifactByKey(workspaceID, key)
	if err != nil {
		return Artifact{}, "", err
	}
	if err := engine.validateArtifact(artifact); err != nil {
		return Artifact{}, "", err
	}
	return artifact, path, nil
}

// DailyInputIdentity uses the same eligible bytes as the actual daily handler.
// It creates no directories or signing authority.
func (engine *Engine) DailyInputIdentity(workspaceID string, day time.Time) (string, error) {
	if err := engine.validateRuntime(workspaceID, false); err != nil {
		return "", err
	}
	before, err := engine.readVisibility(workspaceID)
	if err != nil {
		return "", err
	}
	sources, err := engine.dailySources(workspaceID, day.UTC().Format("2006-01-02"), "attested-captures")
	if err != nil {
		return "", err
	}
	after, err := engine.readVisibility(workspaceID)
	if err != nil {
		return "", err
	}
	if before.Revision != after.Revision {
		return "", ErrCaptureConflict
	}
	return engine.sourceFingerprint("daily", sources), nil
}
