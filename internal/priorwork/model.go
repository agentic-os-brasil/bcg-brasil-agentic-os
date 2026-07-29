package priorwork

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	maximumSnapshotBytes = 64 << 20
	maximumItems         = 100000
	maximumRoots         = 128
)

var (
	opaqueRefPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:!,-]{0,255}$`)
	watermarkPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._~:/+=-]{0,511}$`)
	digestPattern    = regexp.MustCompile(`^[a-f0-9]{64}$`)
	timezonePattern  = regexp.MustCompile(`^[A-Za-z_]+(?:/[A-Za-z0-9_+.-]+)+$`)
)

type RootRef struct {
	SiteRef   string `json:"site_ref"`
	DriveRef  string `json:"drive_ref"`
	FolderRef string `json:"folder_ref"`
}

func (root RootRef) key() string {
	return root.SiteRef + "\x00" + root.DriveRef + "\x00" + root.FolderRef
}

type RootResult struct {
	Root  RootRef `json:"root"`
	State string  `json:"state"`
}

type Facets struct {
	Clients    []string `json:"clients"`
	Projects   []string `json:"projects"`
	Themes     []string `json:"themes"`
	Years      []int    `json:"years"`
	Audiences  []string `json:"audiences"`
	People     []string `json:"people"`
	Presenters []string `json:"presenters"`
}

type Item struct {
	ItemRef      string    `json:"item_ref"`
	ParentRef    string    `json:"parent_ref,omitempty"`
	Root         RootRef   `json:"root"`
	Kind         string    `json:"kind"`
	Name         string    `json:"name"`
	PathSegments []string  `json:"path_segments"`
	SourceURL    string    `json:"source_url"`
	CreatedAt    time.Time `json:"created_at"`
	ModifiedAt   time.Time `json:"modified_at"`
	SizeBytes    int64     `json:"size_bytes"`
	MediaType    string    `json:"media_type"`
	ETag         string    `json:"etag"`
	Facets       Facets    `json:"facets"`
	SearchTerms  []string  `json:"search_terms"`
	Sensitivity  string    `json:"sensitivity"`
	Status       string    `json:"status"`
}

func (item Item) key() string {
	return item.Root.key() + "\x00" + item.ItemRef
}

type Tombstone struct {
	ItemRef    string    `json:"item_ref"`
	Root       RootRef   `json:"root"`
	Reason     string    `json:"reason"`
	ObservedAt time.Time `json:"observed_at"`
}

func (tombstone Tombstone) key() string {
	return tombstone.Root.key() + "\x00" + tombstone.ItemRef
}

type Snapshot struct {
	SchemaVersion      int          `json:"schema_version"`
	Source             string       `json:"source"`
	AdapterRuntime     string       `json:"adapter_runtime"`
	TenantRef          string       `json:"tenant_ref"`
	Mode               string       `json:"mode"`
	CollectionSequence uint64       `json:"collection_sequence"`
	GeneratedAt        time.Time    `json:"generated_at"`
	PreviousWatermark  string       `json:"previous_watermark,omitempty"`
	Watermark          string       `json:"watermark"`
	Roots              []RootRef    `json:"roots"`
	RootResults        []RootResult `json:"root_results"`
	Items              []Item       `json:"items"`
	Tombstones         []Tombstone  `json:"tombstones"`
}

type ImportReceipt struct {
	SchemaVersion         int       `json:"schema_version"`
	ReceiptID             string    `json:"receipt_id"`
	EvidenceClass         string    `json:"evidence_class"`
	Capability            string    `json:"capability"`
	ProducerRuntime       string    `json:"producer_runtime"`
	Outcome               string    `json:"outcome"`
	EmittedAt             time.Time `json:"emitted_at"`
	TenantRef             string    `json:"tenant_ref"`
	Roots                 []RootRef `json:"roots"`
	PolicyVersion         string    `json:"policy_version"`
	EnrollmentFingerprint string    `json:"enrollment_fingerprint"`
	CollectionSequence    uint64    `json:"collection_sequence"`
	Watermark             string    `json:"watermark"`
	SnapshotDigest        string    `json:"snapshot_digest"`
	KeyID                 string    `json:"key_id"`
	TriggerRef            string    `json:"trigger_ref"`
	Signature             string    `json:"signature"`
}

type Enrollment struct {
	SchemaVersion              int       `json:"schema_version"`
	TenantRef                  string    `json:"tenant_ref"`
	Purpose                    string    `json:"purpose"`
	PolicyVersion              string    `json:"policy_version"`
	AuthorizedBy               string    `json:"authorized_by"`
	CollectorKeyID             string    `json:"collector_key_id"`
	CollectorPublicKey         string    `json:"collector_public_key"`
	EnrolledAt                 time.Time `json:"enrolled_at"`
	AuthorizationExpiresAt     time.Time `json:"authorization_expires_at"`
	ScopeExpansionConfirmAfter time.Time `json:"scope_expansion_confirm_after"`
	RefreshHours               int       `json:"refresh_hours"`
	StaleHours                 int       `json:"stale_hours"`
	ScheduleTimezone           string    `json:"schedule_timezone"`
	MaxItemBytes               int64     `json:"max_item_bytes"`
	MaxSnapshotItems           int       `json:"max_snapshot_items"`
	AllowedItemTypes           []string  `json:"allowed_item_types"`
	AllowedOrigins             []string  `json:"allowed_origins"`
	Roots                      []RootRef `json:"roots"`
}

type Catalog struct {
	SchemaVersion      int       `json:"schema_version"`
	TenantRef          string    `json:"tenant_ref"`
	PolicyVersion      string    `json:"policy_version"`
	CollectionSequence uint64    `json:"collection_sequence"`
	Watermark          string    `json:"watermark"`
	SnapshotDigest     string    `json:"snapshot_digest"`
	GeneratedAt        time.Time `json:"generated_at"`
	Roots              []RootRef `json:"roots"`
	Items              []Item    `json:"items"`
}

type Manifest struct {
	SchemaVersion         int       `json:"schema_version"`
	Version               string    `json:"version"`
	TenantRef             string    `json:"tenant_ref"`
	CollectionSequence    uint64    `json:"collection_sequence"`
	Watermark             string    `json:"watermark"`
	Fingerprint           string    `json:"fingerprint"`
	PolicyVersion         string    `json:"policy_version"`
	EnrollmentFingerprint string    `json:"enrollment_fingerprint"`
	SnapshotDigest        string    `json:"snapshot_digest"`
	CompilerVersion       string    `json:"compiler_version"`
	PublishedAt           time.Time `json:"published_at"`
	ItemCount             int       `json:"item_count"`
}

type Barrier struct {
	SchemaVersion         int       `json:"schema_version"`
	TenantRef             string    `json:"tenant_ref"`
	Root                  RootRef   `json:"root"`
	ItemRef               string    `json:"item_ref"`
	Reason                string    `json:"reason"`
	ObservedAt            time.Time `json:"observed_at"`
	CollectionSequence    uint64    `json:"collection_sequence"`
	Watermark             string    `json:"watermark"`
	PolicyVersion         string    `json:"policy_version"`
	EnrollmentFingerprint string    `json:"enrollment_fingerprint"`
	SnapshotDigest        string    `json:"snapshot_digest"`
}

type RevocationFence struct {
	SchemaVersion      int    `json:"schema_version"`
	TenantRef          string `json:"tenant_ref"`
	CollectionSequence uint64 `json:"collection_sequence"`
	Watermark          string `json:"watermark"`
	SnapshotDigest     string `json:"snapshot_digest"`
}

type ApplyReport struct {
	State              string `json:"state"`
	Version            string `json:"version"`
	Fingerprint        string `json:"fingerprint"`
	Watermark          string `json:"watermark"`
	CollectionSequence uint64 `json:"collection_sequence"`
	Items              int    `json:"items"`
	Removed            int    `json:"removed"`
}

type Status struct {
	State              string    `json:"state"`
	Due                bool      `json:"due"`
	Stale              bool      `json:"stale"`
	LastSyncAt         time.Time `json:"last_sync_at,omitempty"`
	Watermark          string    `json:"watermark,omitempty"`
	CollectionSequence uint64    `json:"collection_sequence,omitempty"`
	Fingerprint        string    `json:"fingerprint,omitempty"`
	Items              int       `json:"items"`
	RefreshHours       int       `json:"refresh_hours,omitempty"`
	StaleHours         int       `json:"stale_hours,omitempty"`
}

func ParseSnapshot(reader io.Reader) (Snapshot, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maximumSnapshotBytes+1))
	if err != nil {
		return Snapshot{}, err
	}
	if len(body) > maximumSnapshotBytes {
		return Snapshot{}, fmt.Errorf("SharePoint catalog snapshot exceeds %d bytes", maximumSnapshotBytes)
	}
	if err := rejectDuplicateJSONKeys(body); err != nil {
		return Snapshot{}, fmt.Errorf("decode SharePoint catalog snapshot: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var snapshot Snapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return Snapshot{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Snapshot{}, errors.New("SharePoint catalog snapshot contains multiple JSON values")
		}
		return Snapshot{}, err
	}
	if err := ValidateSnapshot(snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func ParseImportReceipt(reader io.Reader, snapshot Snapshot) (ImportReceipt, error) {
	body, err := io.ReadAll(io.LimitReader(reader, 1<<20))
	if err != nil {
		return ImportReceipt{}, err
	}
	if len(body) >= 1<<20 {
		return ImportReceipt{}, errors.New("SharePoint import receipt exceeds one MiB")
	}
	if err := rejectDuplicateJSONKeys(body); err != nil {
		return ImportReceipt{}, fmt.Errorf("decode SharePoint import receipt: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var receipt ImportReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return ImportReceipt{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return ImportReceipt{}, errors.New("SharePoint import receipt contains multiple JSON values")
		}
		return ImportReceipt{}, err
	}
	if err := ValidateImportReceipt(receipt, snapshot); err != nil {
		return ImportReceipt{}, err
	}
	return receipt, nil
}

func ValidateImportReceipt(receipt ImportReceipt, snapshot Snapshot) error {
	digest, err := fingerprintSnapshot(snapshot)
	if err != nil {
		return err
	}
	if receipt.SchemaVersion != 1 || !opaqueRefPattern.MatchString(receipt.ReceiptID) ||
		receipt.EvidenceClass != "adapter_command" ||
		receipt.Capability != "sharepoint_work_collection" || receipt.ProducerRuntime != "claude" ||
		receipt.Outcome != "succeeded" || receipt.EmittedAt.Before(snapshot.GeneratedAt) ||
		receipt.EmittedAt.After(snapshot.GeneratedAt.Add(15*time.Minute)) ||
		receipt.TenantRef != snapshot.TenantRef || !rootsEqual(receipt.Roots, snapshot.Roots) ||
		receipt.CollectionSequence != snapshot.CollectionSequence ||
		receipt.Watermark != snapshot.Watermark || receipt.SnapshotDigest != digest ||
		validateLabel(receipt.PolicyVersion, 128) != nil || receipt.EnrollmentFingerprint == "" ||
		!opaqueRefPattern.MatchString(receipt.KeyID) || !opaqueRefPattern.MatchString(receipt.TriggerRef) {
		return errors.New("SharePoint import receipt does not bind the snapshot")
	}
	signature, err := base64.StdEncoding.Strict().DecodeString(receipt.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return errors.New("SharePoint import receipt signature is malformed")
	}
	return nil
}

func ValidateSnapshot(snapshot Snapshot) error {
	if snapshot.SchemaVersion != 1 || snapshot.Source != "sharepoint" || snapshot.AdapterRuntime != "claude" {
		return errors.New("snapshot requires schema 1, SharePoint source and declared Claude adapter runtime")
	}
	if !opaqueRefPattern.MatchString(snapshot.TenantRef) || !watermarkPattern.MatchString(snapshot.Watermark) ||
		snapshot.CollectionSequence == 0 || snapshot.GeneratedAt.IsZero() {
		return errors.New("snapshot requires valid tenant, sequence, watermark and generation time")
	}
	if snapshot.Mode != "full" && snapshot.Mode != "delta" {
		return errors.New("snapshot mode must be full or delta")
	}
	if snapshot.CollectionSequence == 1 {
		if snapshot.Mode != "full" || snapshot.PreviousWatermark != "" {
			return errors.New("initial snapshot must be a full snapshot without previous watermark")
		}
	} else if !watermarkPattern.MatchString(snapshot.PreviousWatermark) {
		return errors.New("non-initial snapshot requires a valid previous watermark")
	}
	if snapshot.Roots == nil || snapshot.RootResults == nil || snapshot.Items == nil || snapshot.Tombstones == nil ||
		len(snapshot.Roots) == 0 || len(snapshot.Roots) > maximumRoots ||
		len(snapshot.RootResults) != len(snapshot.Roots) ||
		len(snapshot.Items)+len(snapshot.Tombstones) > maximumItems {
		return errors.New("snapshot exceeds root or item limits")
	}
	rootSet := map[string]bool{}
	for _, root := range snapshot.Roots {
		if err := validateRoot(root); err != nil {
			return err
		}
		if rootSet[root.key()] {
			return errors.New("snapshot contains duplicate roots")
		}
		rootSet[root.key()] = true
	}
	rootResultSet := map[string]bool{}
	for _, result := range snapshot.RootResults {
		if !rootSet[result.Root.key()] || result.State != "complete" || rootResultSet[result.Root.key()] {
			return errors.New("snapshot root results must exactly and completely cover enrolled roots")
		}
		rootResultSet[result.Root.key()] = true
	}
	itemSet := map[string]bool{}
	for index := range snapshot.Items {
		item := &snapshot.Items[index]
		if err := validateItem(*item, rootSet); err != nil {
			return fmt.Errorf("item %q: %w", item.ItemRef, err)
		}
		if itemSet[item.key()] {
			return fmt.Errorf("snapshot contains duplicate composite item %q", item.ItemRef)
		}
		itemSet[item.key()] = true
	}
	tombstoneSet := map[string]bool{}
	for _, tombstone := range snapshot.Tombstones {
		if !opaqueRefPattern.MatchString(tombstone.ItemRef) || !rootSet[tombstone.Root.key()] || tombstone.ObservedAt.IsZero() ||
			(tombstone.Reason != "deleted" && tombstone.Reason != "access_revoked") {
			return errors.New("snapshot contains an invalid tombstone")
		}
		if tombstoneSet[tombstone.key()] || itemSet[tombstone.key()] {
			return fmt.Errorf("snapshot contains conflicting item/tombstone %q", tombstone.ItemRef)
		}
		tombstoneSet[tombstone.key()] = true
	}
	return nil
}

func ValidateEnrollment(enrollment Enrollment) error {
	if enrollment.SchemaVersion != 1 || !opaqueRefPattern.MatchString(enrollment.TenantRef) ||
		enrollment.Purpose != "prior_work_retrieval" || validateLabel(enrollment.PolicyVersion, 128) != nil ||
		!opaqueRefPattern.MatchString(enrollment.AuthorizedBy) || enrollment.EnrolledAt.IsZero() ||
		!opaqueRefPattern.MatchString(enrollment.CollectorKeyID) ||
		!enrollment.AuthorizationExpiresAt.After(enrollment.EnrolledAt) ||
		enrollment.ScopeExpansionConfirmAfter.Before(enrollment.EnrolledAt) ||
		enrollment.ScopeExpansionConfirmAfter.After(enrollment.AuthorizationExpiresAt) ||
		enrollment.RefreshHours <= 0 || enrollment.RefreshHours > 8760 ||
		enrollment.StaleHours <= 0 || enrollment.StaleHours > 8760 ||
		enrollment.MaxItemBytes <= 0 || enrollment.MaxItemBytes > 1<<40 ||
		enrollment.MaxSnapshotItems <= 0 || enrollment.MaxSnapshotItems > maximumItems ||
		enrollment.StaleHours < enrollment.RefreshHours || len(enrollment.Roots) == 0 ||
		len(enrollment.Roots) > maximumRoots || len(enrollment.AllowedOrigins) == 0 ||
		len(enrollment.AllowedItemTypes) == 0 {
		return errors.New("invalid prior-work enrollment")
	}
	if len(enrollment.ScheduleTimezone) > 64 || !timezonePattern.MatchString(enrollment.ScheduleTimezone) {
		return errors.New("invalid prior-work scheduler timezone")
	}
	if _, err := time.LoadLocation(enrollment.ScheduleTimezone); err != nil {
		return errors.New("invalid prior-work scheduler timezone")
	}
	publicKey, err := base64.StdEncoding.Strict().DecodeString(enrollment.CollectorPublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return errors.New("invalid prior-work collector public key")
	}
	seen := map[string]bool{}
	for _, root := range enrollment.Roots {
		if err := validateRoot(root); err != nil {
			return err
		}
		if seen[root.key()] {
			return errors.New("enrollment contains duplicate roots")
		}
		seen[root.key()] = true
	}
	types := map[string]bool{}
	for _, itemType := range enrollment.AllowedItemTypes {
		if (itemType != "file" && itemType != "folder") || types[itemType] {
			return errors.New("invalid allowed item types")
		}
		types[itemType] = true
	}
	origins := map[string]bool{}
	for _, origin := range enrollment.AllowedOrigins {
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Hostname() == "" ||
			parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Port() != "" ||
			(parsed.Path != "" && parsed.Path != "/") ||
			!(strings.EqualFold(parsed.Hostname(), "sharepoint.com") ||
				strings.HasSuffix(strings.ToLower(parsed.Hostname()), ".sharepoint.com")) ||
			origins[strings.ToLower(strings.TrimSuffix(origin, "/"))] {
			return errors.New("invalid or duplicate allowed SharePoint origin")
		}
		origins[strings.ToLower(strings.TrimSuffix(origin, "/"))] = true
	}
	return nil
}

func validateRoot(root RootRef) error {
	if !opaqueRefPattern.MatchString(root.SiteRef) || !opaqueRefPattern.MatchString(root.DriveRef) ||
		!opaqueRefPattern.MatchString(root.FolderRef) {
		return errors.New("invalid SharePoint root reference")
	}
	return nil
}

func validateItem(item Item, roots map[string]bool) error {
	if !opaqueRefPattern.MatchString(item.ItemRef) || (item.ParentRef != "" && !opaqueRefPattern.MatchString(item.ParentRef)) {
		return errors.New("invalid item reference")
	}
	if !roots[item.Root.key()] {
		return errors.New("item is outside the enrolled snapshot roots")
	}
	if item.Kind != "file" && item.Kind != "folder" {
		return errors.New("item kind must be file or folder")
	}
	if err := validateLabel(item.Name, 256); err != nil {
		return fmt.Errorf("invalid name: %w", err)
	}
	if item.PathSegments == nil || item.SearchTerms == nil || len(item.PathSegments) > 64 {
		return errors.New("too many path segments")
	}
	for _, segment := range item.PathSegments {
		if err := validateLabel(segment, 256); err != nil || segment == "." || segment == ".." || strings.ContainsAny(segment, `/\`) {
			return errors.New("invalid path segment")
		}
	}
	parsed, err := url.Parse(item.SourceURL)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Hostname() == "" ||
		len([]rune(item.SourceURL)) > 2048 || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Port() != "" ||
		!(parsed.Hostname() == "sharepoint.com" || strings.HasSuffix(strings.ToLower(parsed.Hostname()), ".sharepoint.com")) {
		return errors.New("source URL must be a canonical HTTPS SharePoint URL without credentials, query or fragment")
	}
	if item.CreatedAt.IsZero() || item.ModifiedAt.IsZero() || item.SizeBytes < 0 ||
		item.SizeBytes > 1<<40 || validateLabel(item.MediaType, 255) != nil || validateLabel(item.ETag, 512) != nil {
		return errors.New("invalid item metadata")
	}
	if item.Sensitivity != "internal" && item.Sensitivity != "client_restricted" {
		return errors.New("invalid item sensitivity")
	}
	if item.Status != "active" {
		return errors.New("item status must be active")
	}
	if err := validateFacets(item.Facets); err != nil {
		return err
	}
	if err := validateLabels(item.SearchTerms, 128); err != nil {
		return fmt.Errorf("invalid search terms: %w", err)
	}
	return nil
}

func validateFacets(facets Facets) error {
	if facets.Clients == nil || facets.Projects == nil || facets.Themes == nil || facets.Years == nil ||
		facets.Audiences == nil || facets.People == nil || facets.Presenters == nil {
		return errors.New("facet arrays are required")
	}
	for _, values := range [][]string{
		facets.Clients, facets.Projects, facets.Themes, facets.Audiences, facets.People, facets.Presenters,
	} {
		if err := validateLabels(values, 32); err != nil {
			return fmt.Errorf("invalid facets: %w", err)
		}
	}
	seenYears := map[int]bool{}
	if len(facets.Years) > 16 {
		return errors.New("too many year facets")
	}
	for _, year := range facets.Years {
		if year < 1990 || year > 2200 || seenYears[year] {
			return errors.New("invalid year facets")
		}
		seenYears[year] = true
	}
	return nil
}

func validateLabels(values []string, maximum int) error {
	if len(values) > maximum {
		return errors.New("too many values")
	}
	seen := map[string]bool{}
	for _, value := range values {
		if err := validateLabel(value, 256); err != nil {
			return err
		}
		normalized := normalize(value)
		if seen[normalized] {
			return errors.New("duplicate normalized value")
		}
		seen[normalized] = true
	}
	return nil
}

func validateLabel(value string, maximum int) error {
	if strings.TrimSpace(value) != value || value == "" || len([]rune(value)) > maximum {
		return errors.New("value is empty, untrimmed or oversized")
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return errors.New("control characters are forbidden")
		}
	}
	return nil
}

func rootsEqual(left, right []RootRef) bool {
	if len(left) != len(right) {
		return false
	}
	leftKeys := make([]string, 0, len(left))
	rightKeys := make([]string, 0, len(right))
	for _, root := range left {
		leftKeys = append(leftKeys, root.key())
	}
	for _, root := range right {
		rightKeys = append(rightKeys, root.key())
	}
	sort.Strings(leftKeys)
	sort.Strings(rightKeys)
	return strings.Join(leftKeys, "\n") == strings.Join(rightKeys, "\n")
}

func rejectDuplicateJSONKeys(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if err := walkJSONValue(decoder, token); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func walkJSONValue(decoder *json.Decoder, token json.Token) error {
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("JSON object key is not a string")
			}
			if seen[key] {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			seen[key] = true
			valueToken, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := walkJSONValue(decoder, valueToken); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return errors.New("unterminated JSON object")
		}
	case '[':
		for decoder.More() {
			valueToken, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := walkJSONValue(decoder, valueToken); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return errors.New("unterminated JSON array")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	return nil
}
