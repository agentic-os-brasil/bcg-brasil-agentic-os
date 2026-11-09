package installtx

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

// ReleaseBinding retains authenticated release material for launch and managed
// rollback. Source history deliberately does not belong to this snapshot.
type ReleaseBinding struct {
	ManifestSHA256     string `json:"manifest_sha256"`
	ClientInputsSHA256 string `json:"client_inputs_sha256"`
	ManifestJSON       string `json:"manifest_json"`
	ManifestSignature  string `json:"manifest_signature"`
	BundleSHA256       string `json:"bundle_sha256"`
	BundleSize         int64  `json:"bundle_size"`
	BundleArchive      string `json:"bundle_archive"`
}

type SourceBinding struct {
	Tuple            releasecontract.SourceTuple `json:"tuple"`
	Sequence         uint64                      `json:"sequence"`
	GenerationSHA256 string                      `json:"generation_sha256"`
}

// SourceControl is current installation custody, never restorable Previous
// state. The exact signed record is reauthenticated whenever it grants launch.
type SourceControl struct {
	Locator string `json:"locator"`
	SourceBinding
	Body                    []byte                          `json:"body"`
	Signature               []byte                          `json:"signature"`
	Recalls                 []releasecontract.ReleaseRecall `json:"recalls"`
	RecallCapacityExhausted bool                            `json:"recall_capacity_exhausted,omitempty"`
	MinimumReaderContract   string                          `json:"minimum_reader_contract"`
	ObservedAt              time.Time                       `json:"observed_at"`
}

func validReleaseBinding(b ReleaseBinding) bool {
	return sha256Pattern.MatchString(b.ManifestSHA256) && sha256Pattern.MatchString(b.ClientInputsSHA256) &&
		sha256Pattern.MatchString(b.BundleSHA256) && b.BundleSize > 0 && len(b.ManifestJSON) > 0 && len(b.ManifestJSON) <= 1<<20 &&
		len(b.ManifestSignature) == 88 && filepath.IsAbs(b.BundleArchive) && filepath.Clean(b.BundleArchive) == b.BundleArchive
}

func bindingForPrepared(v releaseverify.VerifiedRelease, plan ActivationPlan) (ReleaseBinding, error) {
	if !v.Manifest.Modern() || v.Manifest.Server.ExecutionContract == "" {
		return ReleaseBinding{}, nil
	}
	body, err := os.ReadFile(filepath.Join(v.Directory, releaseverify.ManifestName))
	if err != nil {
		return ReleaseBinding{}, err
	}
	sig, err := os.ReadFile(filepath.Join(v.Directory, releaseverify.ManifestSignatureName))
	if err != nil {
		return ReleaseBinding{}, err
	}
	sum := sha256.Sum256(body)
	if len(body) > 1<<20 || fmt.Sprintf("%x", sum) != v.ManifestSHA256 || len(sig) != 64 {
		return ReleaseBinding{}, errors.New("source-aware release binding changed")
	}
	return ReleaseBinding{ManifestSHA256: v.ManifestSHA256, ClientInputsSHA256: v.Manifest.Delivery.ClientInputsSHA256, ManifestJSON: string(body), ManifestSignature: base64.StdEncoding.EncodeToString(sig), BundleSHA256: plan.BundleSHA256, BundleSize: plan.BundleSize, BundleArchive: plan.StagedBundleArchive}, nil
}

func StateDigest(state State) string {
	body, _ := json.Marshal(state)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// ReadSourceState never performs the historical schema1 migration. Source
// review, status and launch must remain read-only even on unsupported state.
func ReadSourceState(dataRoot, managedRoot string) (State, error) {
	managed, data, err := normalizedRoots(managedRoot, dataRoot)
	if err != nil {
		return State{}, err
	}
	state, err := ReadState(data)
	if err != nil {
		return State{}, err
	}
	if state.ManagedRoot != managed {
		return State{}, errors.New("source state belongs to another managed installation")
	}
	return state, nil
}
func stateSchema(profile string, b ReleaseBinding) int {
	if b.ManifestSHA256 != "" {
		return 4
	}
	return schemaForProfile(profile, 2)
}
func receiptSchema(plan ActivationPlan) int {
	if plan.SchemaVersion == 4 {
		return 3
	}
	if plan.Profile != "" {
		return 2
	}
	return 1
}
func guardStateWrite(dataRoot string, next State) error {
	if next.SchemaVersion == 4 && (!validReleaseBinding(next.Binding) || next.Server.ExecutionContract != releasecontract.SyncedSourceContract || validateSourceShape(next.Source) != nil) {
		return errors.New("invalid protected source state")
	}
	if next.SchemaVersion != 4 && (next.Binding != (ReleaseBinding{}) || next.Source != nil) {
		return errors.New("historical state cannot carry source-aware fields")
	}
	body, err := os.ReadFile(statePath(dataRoot))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var header struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(body, &header); err != nil {
		return err
	}
	if header.SchemaVersion != 4 {
		return nil
	}
	if next.SchemaVersion != 4 {
		return errors.New("protected source state cannot be downgraded")
	}
	old, err := ReadState(dataRoot)
	if err != nil {
		return err
	}
	return preserveSourceCustody(old, next)
}
func sourceBinding(control *SourceControl) SourceBinding {
	if control == nil {
		return SourceBinding{}
	}
	return control.SourceBinding
}

func validateSourceShape(s *SourceControl) error {
	if s == nil {
		return nil
	}
	if !filepath.IsAbs(s.Locator) || filepath.Clean(s.Locator) != s.Locator || s.Tuple.Validate() != nil || s.Sequence == 0 || !sha256Pattern.MatchString(s.GenerationSHA256) || s.MinimumReaderContract != releasecontract.SyncedSourceContract || s.ObservedAt.IsZero() || s.ObservedAt.Location() != time.UTC || s.Recalls == nil || len(s.Recalls) > releasecontract.MaximumSourceRecalls {
		return errors.New("invalid source custody")
	}
	g, err := releasecontract.ParseGeneration(s.Body)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(s.Body)
	if g.Source != s.Tuple || g.Sequence != s.Sequence || fmt.Sprintf("%x", sum) != s.GenerationSHA256 || len(s.Signature) != 64 {
		return errors.New("source custody does not match signed metadata")
	}
	seen := map[releasecontract.ReleaseRecall]bool{}
	for _, r := range s.Recalls {
		if r.Validate() != nil || seen[r] {
			return errors.New("invalid cumulative recalls")
		}
		seen[r] = true
	}
	for _, r := range g.Recalls {
		if !seen[r] && !s.RecallCapacityExhausted {
			return errors.New("source custody omitted authenticated recall")
		}
	}
	return nil
}

func preserveSourceCustody(old, next State) error {
	if old.SchemaVersion != 4 {
		return nil
	}
	if next.SchemaVersion != 4 {
		return errors.New("protected source state cannot be downgraded")
	}
	if old.Source == nil {
		return nil
	}
	if next.Source == nil {
		return errors.New("source custody cannot be removed")
	}
	a, b := old.Source, next.Source
	if a.RecallCapacityExhausted && !b.RecallCapacityExhausted {
		return errors.New("source recall capacity exhaustion cannot be cleared")
	}
	if a.Tuple != b.Tuple || a.Locator != b.Locator || b.Sequence < a.Sequence || b.ObservedAt.Before(a.ObservedAt) || a.MinimumReaderContract != b.MinimumReaderContract || (a.Sequence == b.Sequence && a.GenerationSHA256 != b.GenerationSHA256) {
		return errors.New("source custody cannot regress or rebind")
	}
	for _, r := range a.Recalls {
		found := false
		for _, n := range b.Recalls {
			found = found || r == n
		}
		if !found {
			return errors.New("known recall cannot be forgotten")
		}
	}
	return nil
}

// ObserveSource is the sole attended source-custody mutation. Callers first
// present the exact StateDigest and authenticated record to a trusted reviewer.
// Temporal eligibility is intentionally checked by the caller AFTER this
// durable monotonic observation, so expired higher denial survives payload loss.
func ObserveSource(dataRoot, managedRoot, locator string, g releaseverify.VerifiedGeneration, expectedStateSHA256 string, now time.Time) (State, error) {
	return observeSource(dataRoot, managedRoot, locator, g, expectedStateSHA256, now, nil)
}

// ObserveSourceWithRegistry is trusted installer/maintenance composition. The
// caller supplies its independently admitted fixed registry, never a feed/model
// key. This keeps authenticated denial/source refresh available when the current
// MCP cannot execute. It does not describe that unavailable executable as healthy.
func ObserveSourceWithRegistry(dataRoot, managedRoot, locator string, g releaseverify.VerifiedGeneration, expectedStateSHA256 string, now time.Time, registry releaseverify.KeyRegistry) (State, error) {
	if registry == nil {
		return State{}, errors.New("independently admitted source registry required")
	}
	return observeSource(dataRoot, managedRoot, locator, g, expectedStateSHA256, now, registry)
}

func observeSource(dataRoot, managedRoot, locator string, g releaseverify.VerifiedGeneration, expectedStateSHA256 string, now time.Time, registry releaseverify.KeyRegistry) (State, error) {
	unlock, err := acquireLock(managedRoot)
	if err != nil {
		return State{}, err
	}
	defer unlock()
	current, err := ReadSourceState(dataRoot, managedRoot)
	if err != nil {
		return State{}, err
	}
	if StateDigest(current) != expectedStateSHA256 {
		return State{}, errors.New("source review is stale")
	}
	if current.SchemaVersion != 4 || current.Server.ExecutionContract != releasecontract.SyncedSourceContract || !validReleaseBinding(current.Binding) {
		return State{}, errors.New("source enrollment upgrade/migration unavailable: genuine signed source-aware current release is required")
	}
	if !filepath.IsAbs(locator) || filepath.Clean(locator) != locator || g.Generation.Source.OS != current.TargetOS || g.Generation.Source.Arch != current.TargetArch || g.Generation.Source.Channel != current.Channel {
		return State{}, errors.New("source tuple/locator does not match installed target")
	}
	if registry == nil {
		registry, err = installedRegistry(managedRoot, current)
		if err != nil {
			return State{}, err
		}
	}
	auth, err := releaseverify.VerifyGeneration(g.Body, g.Signature, registry)
	if err != nil {
		return State{}, err
	}
	if auth.SHA256 != g.SHA256 || !reflect.DeepEqual(auth.Generation, g.Generation) {
		return State{}, errors.New("source metadata changed after trusted review")
	}
	if err := verifyReleaseWithRegistry(registry, current, current.Binding, current.Server); err != nil {
		return State{}, err
	}
	if now.IsZero() || current.Source != nil && now.Before(current.Source.ObservedAt) {
		return State{}, errors.New("trusted source clock moved backwards")
	}
	next := &SourceControl{Locator: locator, SourceBinding: SourceBinding{Tuple: auth.Generation.Source, Sequence: auth.Generation.Sequence, GenerationSHA256: auth.SHA256}, Body: auth.Body, Signature: auth.Signature, Recalls: []releasecontract.ReleaseRecall{}, MinimumReaderContract: auth.Generation.MinimumReaderContract, ObservedAt: now.UTC()}
	if current.Source != nil {
		if current.Source.Tuple != next.Tuple || current.Source.Locator != locator {
			return State{}, errors.New("enrolled source cannot be silently rebound")
		}
		next.Recalls = append(next.Recalls, current.Source.Recalls...)
		next.RecallCapacityExhausted = current.Source.RecallCapacityExhausted
	}
	priorRecalls := append([]releasecontract.ReleaseRecall{}, next.Recalls...)
	for _, r := range auth.Generation.Recalls {
		found := false
		for _, known := range next.Recalls {
			found = found || r == known
		}
		if !found {
			next.Recalls = append(next.Recalls, r)
		}
	}
	if len(next.Recalls) > releasecontract.MaximumSourceRecalls {
		// Preserve the bounded known set and the complete newly authenticated
		// record, then deny every release through a sticky capacity fence. A
		// finite list must never discard higher authenticated denial knowledge.
		next.Recalls = priorRecalls
		next.RecallCapacityExhausted = true
	}
	updated := current
	updated.Source = next
	if err := preserveSourceCustody(current, updated); err != nil {
		return State{}, err
	}
	if err := WriteState(dataRoot, updated); err != nil {
		return State{}, err
	}
	return updated, nil
}

func installedRegistry(managedRoot string, state State) (releaseverify.AuthorityRegistry, error) {
	path := filepath.Join(managedRoot, "bin", executableForProfile(state.TargetOS, state.Profile))
	if err := verifyRegularFile(path, state.Server.Size, state.Server.SHA256); err != nil {
		return releaseverify.AuthorityRegistry{}, err
	}
	i, err := releasecontract.ReadServerIdentity(path)
	if err != nil {
		return releaseverify.AuthorityRegistry{}, err
	}
	if i.ExecutionContract != releasecontract.SyncedSourceContract || i.ExecutionContract != state.Server.ExecutionContract {
		return releaseverify.AuthorityRegistry{}, errors.New("actual executable lacks admitted source execution contract")
	}
	r, err := releaseverify.LoadPinnedAuthorityRegistry(filepath.Join(managedRoot, "trust", "release-authority-registry.json"), i.AuthorityRegistrySHA256, time.Now)
	if err != nil {
		return releaseverify.AuthorityRegistry{}, err
	}
	return r, nil
}

func verifyReleaseBinding(managedRoot, dataRoot string, state State, b ReleaseBinding, server ExecutableIdentity, verifyBundle bool) error {
	if !validReleaseBinding(b) || server.ExecutionContract != releasecontract.SyncedSourceContract {
		return errors.New("authenticated source-aware release binding required")
	}
	r, err := installedRegistry(managedRoot, state)
	if err != nil {
		return err
	}
	if err := verifyReleaseWithRegistry(r, state, b, server); err != nil {
		return err
	}
	if verifyBundle {
		return verifyBoundBundle(managedRoot, dataRoot, state.BundleVersion, b)
	}
	return nil
}

func verifyReleaseWithRegistry(r releaseverify.KeyRegistry, state State, b ReleaseBinding, server ExecutableIdentity) error {
	sig, err := base64.StdEncoding.DecodeString(b.ManifestSignature)
	if err != nil {
		return err
	}
	m, _, err := releaseverify.VerifyManifest([]byte(b.ManifestJSON), sig, r)
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(b.ManifestJSON))
	if fmt.Sprintf("%x", sum) != b.ManifestSHA256 || m.Release != state.Release || m.Channel != state.Channel || m.Bundle.Version != state.BundleVersion || m.Delivery.ClientInputsSHA256 != b.ClientInputsSHA256 || m.Server.ExecutionContract != server.ExecutionContract {
		return errors.New("installed signed release identity differs from source-aware custody")
	}
	var matched, bundleMatched bool
	for _, a := range m.Artifacts {
		if a.Kind == "mcp_server" && a.OS == state.TargetOS && a.Arch == state.TargetArch {
			matched = serverForManifest(m, a) == server
		}
		if a.Kind == "bundle" {
			bundleMatched = a.SHA256 == b.BundleSHA256 && a.Size == b.BundleSize
		}
	}
	if !matched || !bundleMatched {
		return errors.New("installed server/bundle differs from authenticated component")
	}
	return nil
}

func verifyBoundBundle(managedRoot, dataRoot, version string, b ReleaseBinding) error {
	if err := ensureSafePath(dataRoot, b.BundleArchive); err != nil {
		return err
	}
	if err := verifyRegularFile(b.BundleArchive, b.BundleSize, b.BundleSHA256); err != nil {
		return err
	}
	file, err := openRegular(b.BundleArchive, b.BundleSize)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	root := filepath.Join(managedRoot, "bundles", version)
	if err := ensureSafePath(managedRoot, root); err != nil {
		return err
	}
	seen := map[string]bool{}
	files := 0
	var total int64
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		files++
		total += h.Size
		if files > maximumBundleFiles || total > maximumBundleBytes || h.Size < 0 || !safeArchivePath(h.Name) || seen[h.Name] {
			return errors.New("authenticated bundle inventory invalid")
		}
		seen[h.Name] = true
		path := filepath.Join(root, filepath.FromSlash(h.Name))
		if err := ensureSafePath(root, path); err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if !info.IsDir() {
				return errors.New("installed bundle directory differs")
			}
		case tar.TypeReg, tar.TypeRegA:
			if !info.Mode().IsRegular() || info.Size() != h.Size {
				return errors.New("installed bundle file differs")
			}
			hash := sha256.New()
			if _, err := io.Copy(hash, tr); err != nil {
				return err
			}
			if err := verifyRegularFile(path, h.Size, fmt.Sprintf("%x", hash.Sum(nil))); err != nil {
				return err
			}
			if (info.Mode().Perm()&0111 != 0) != (h.Mode&0111 != 0) {
				return errors.New("installed bundle executable mode differs")
			}
		default:
			return errors.New("unsupported authenticated bundle entry")
		}
	}
	if files == 0 {
		return errors.New("empty authenticated bundle")
	}
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("installed bundle contains alias")
		}
		if !entry.IsDir() && !seen[filepath.ToSlash(rel)] {
			return errors.New("installed bundle contains unauthenticated file")
		}
		return nil
	})
}

// ValidateQualifiedLaunch is a read-only admission check. It never refreshes
// the sync folder, enrolls source custody, repairs state or activates payloads.
func ValidateQualifiedLaunch(managedRoot, dataRoot string, expected ReleaseBinding, now time.Time) (State, error) {
	if (expected.ManifestSHA256 == "") != (expected.ClientInputsSHA256 == "") {
		return State{}, errors.New("incomplete carrier release binding: manifest and client-input pins are required together")
	}
	state, err := ReadSourceState(dataRoot, managedRoot)
	if err != nil {
		return State{}, err
	}
	if state.SchemaVersion != 4 || state.Source == nil {
		return State{}, errors.New("qualified source launch unavailable: use the independent installer source review/check")
	}
	if state.Source.RecallCapacityExhausted {
		return State{}, errors.New("source recall capacity exhausted; qualified launch unavailable")
	}
	if expected.ManifestSHA256 != "" && (state.Binding.ManifestSHA256 != expected.ManifestSHA256 || state.Binding.ClientInputsSHA256 != expected.ClientInputsSHA256) {
		return State{}, errors.New("carrier release changed: regenerate the client packet")
	}
	if err := validateSourceShape(state.Source); err != nil {
		return State{}, err
	}
	r, err := installedRegistry(managedRoot, state)
	if err != nil {
		return State{}, err
	}
	g, err := releaseverify.VerifyGeneration(state.Source.Body, state.Source.Signature, r)
	if err != nil {
		return State{}, err
	}
	if g.SHA256 != state.Source.GenerationSHA256 {
		return State{}, errors.New("cached source metadata differs from custody")
	}
	if err := g.Generation.CheckTime(now, state.Source.ObservedAt); err != nil {
		return State{}, err
	}
	if err := releaseEligible(state.Source, state.Release, state.Binding); err != nil {
		return State{}, err
	}
	if err := CheckActivatedServer(managedRoot, state); err != nil {
		return State{}, err
	}
	if err := verifyReleaseBinding(managedRoot, dataRoot, state, state.Binding, state.Server, true); err != nil {
		return State{}, err
	}
	return state, nil
}

// GuardQualifiedCurrent couples a domain publication to the same serialization
// boundary as accepted recalls and activation. It does not acquire domain or
// maintenance locks, preserving domain -> control -> installation lock order.
func GuardQualifiedCurrent(managedRoot, dataRoot string, expected ReleaseBinding, now time.Time, publish func() error) error {
	if publish == nil {
		return errors.New("publication callback required")
	}
	unlock, err := acquireLock(managedRoot)
	if err != nil {
		return err
	}
	defer unlock()
	if _, err := ValidateQualifiedLaunch(managedRoot, dataRoot, expected, now); err != nil {
		return err
	}
	return publish()
}

// GuardQualifiedReconciliationCurrent is used only by an explicitly reviewed
// client reconciliation. Reuse the canonical dead-owner/live-owner checks;
// ordinary startup, launch, publication and status never reclaim a lock.
func GuardQualifiedReconciliationCurrent(managedRoot, dataRoot string, expected ReleaseBinding, now time.Time, publish func() error) error {
	if publish == nil {
		return errors.New("reconciliation callback required")
	}
	unlock, err := acquireReconciliationLock(managedRoot)
	if err != nil {
		return err
	}
	defer unlock()
	if _, err = ValidateQualifiedLaunch(managedRoot, dataRoot, expected, now); err != nil {
		return err
	}
	return publish()
}

func releaseEligible(source *SourceControl, release string, b ReleaseBinding) error {
	if source == nil || source.MinimumReaderContract != releasecontract.SyncedSourceContract {
		return errors.New("source-aware execution floor unavailable")
	}
	if source.RecallCapacityExhausted {
		return errors.New("source recall capacity exhausted; release admission unavailable")
	}
	for _, r := range source.Recalls {
		if r.Release == release && r.ManifestSHA256 == b.ManifestSHA256 {
			return errors.New("release recalled by authenticated source")
		}
	}
	return nil
}

func validateSourceAdmission(managedRoot string, state State, now time.Time) error {
	if state.SchemaVersion != 4 || state.Source == nil {
		return errors.New("enrolled source-aware state required")
	}
	if state.Source.RecallCapacityExhausted {
		return errors.New("source recall capacity exhausted; source admission unavailable")
	}
	if err := validateSourceShape(state.Source); err != nil {
		return err
	}
	r, err := installedRegistry(managedRoot, state)
	if err != nil {
		return err
	}
	g, err := releaseverify.VerifyGeneration(state.Source.Body, state.Source.Signature, r)
	if err != nil {
		return err
	}
	if g.SHA256 != state.Source.GenerationSHA256 {
		return errors.New("source custody digest differs")
	}
	return g.Generation.CheckTime(now, state.Source.ObservedAt)
}

type RollbackReview struct {
	Digest         string        `json:"digest"`
	FromRelease    string        `json:"from_release"`
	ToRelease      string        `json:"to_release"`
	ManifestSHA256 string        `json:"manifest_sha256"`
	Source         SourceBinding `json:"source_binding"`
}

func ReviewRollback(managedRoot, dataRoot string) (RollbackReview, error) {
	s, err := ReadSourceState(dataRoot, managedRoot)
	if err != nil {
		return RollbackReview{}, err
	}
	if err := validateQualifiedRollback(managedRoot, dataRoot, s, time.Now()); err != nil {
		return RollbackReview{}, err
	}
	return RollbackReview{Digest: StateDigest(s), FromRelease: s.Release, ToRelease: s.Previous.Release, ManifestSHA256: s.Previous.Binding.ManifestSHA256, Source: s.Source.SourceBinding}, nil
}
func RollbackQualified(managedRoot, dataRoot, expectedDigest string) error {
	if !sha256Pattern.MatchString(expectedDigest) {
		return errors.New("exact rollback review digest required")
	}
	return rollback(managedRoot, dataRoot, nil, expectedDigest)
}
func validateQualifiedRollback(managedRoot, dataRoot string, s State, now time.Time) error {
	if s.Previous == nil {
		return errors.New("no previous authenticated source-aware release available")
	}
	if err := validateSourceAdmission(managedRoot, s, now); err != nil {
		return err
	}
	p := s.Previous
	if p.Profile != s.Profile || p.Server.ExecutionContract != releasecontract.SyncedSourceContract || !validReleaseBinding(p.Binding) {
		return errors.New("previous release is below source-aware execution floor")
	}
	if err := releaseEligible(s.Source, p.Release, p.Binding); err != nil {
		return err
	}
	r, err := installedRegistry(managedRoot, s)
	if err != nil {
		return err
	}
	target := State{Profile: p.Profile, Server: p.Server, Release: p.Release, Channel: p.Channel, BundleVersion: p.BundleVersion, TargetOS: p.TargetOS, TargetArch: p.TargetArch}
	if err := verifyReleaseWithRegistry(r, target, p.Binding, p.Server); err != nil {
		return err
	}
	if err := checkModernServer(p.CLIBackup, managedRoot, p.Server); err != nil {
		return err
	}
	return verifyBoundBundle(managedRoot, dataRoot, p.BundleVersion, p.Binding)
}

func validateReconcileSource(plan ActivationPlan, current State) error {
	if current.SchemaVersion != 4 || current.Source == nil || current.Source.Tuple != plan.Source.Tuple || current.Source.Sequence < plan.Source.Sequence {
		return errors.New("reconciliation source custody unavailable or regressed")
	}
	if err := validateSourceShape(current.Source); err != nil {
		return err
	}
	if err := checkModernServer(plan.StagedCLI, plan.ManagedRoot, plan.Server); err != nil {
		return err
	}
	identity, err := releasecontract.ReadServerIdentity(plan.StagedCLI)
	if err != nil {
		return err
	}
	registry, err := releaseverify.LoadPinnedAuthorityRegistry(filepath.Join(plan.ManagedRoot, "trust", "release-authority-registry.json"), identity.AuthorityRegistrySHA256, time.Now)
	if err != nil {
		return err
	}
	target := State{Release: plan.Release, Channel: plan.Channel, BundleVersion: plan.BundleVersion, TargetOS: plan.TargetOS, TargetArch: plan.TargetArch}
	if err := verifyReleaseWithRegistry(registry, target, plan.Binding, plan.Server); err != nil {
		return err
	}
	g, err := releaseverify.VerifyGeneration(current.Source.Body, current.Source.Signature, registry)
	if err != nil {
		return err
	}
	if g.SHA256 != current.Source.GenerationSHA256 {
		return errors.New("reconciliation source metadata differs from custody")
	}
	return nil
}

func sourceMatchesPlan(state State, plan ActivationPlan) bool {
	return state.SchemaVersion == 4 && state.Source != nil && sourceBinding(state.Source) == plan.Source
}
