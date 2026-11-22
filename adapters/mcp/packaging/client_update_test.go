package packaging

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCurrentDesktopMethodsVerifierRejectsAlteredAndForeignPayload(t *testing.T) {
	root := filepath.Join(t.TempDir(), "frontend")
	built, err := BuildDesktopMethodFrontend(DesktopFrontendOptions{Destination: root, Version: "0.1.51"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, built.Archive)
	verified, err := VerifyDesktopMethodFrontend(path, "0.1.51")
	if err != nil || verified.ArchiveSHA256 != built.ArchiveSHA256 || verified.FileCount != built.FileCount {
		t.Fatal("current canonical frontend rejected", err)
	}
	if _, err := VerifyDesktopMethodFrontend(path, "0.1.52"); err == nil {
		t.Fatal("old method library accepted as new version")
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source, err := zip.NewReader(bytes.NewReader(original), int64(len(original)))
	if err != nil {
		t.Fatal(err)
	}
	var altered bytes.Buffer
	writer := zip.NewWriter(&altered)
	for index, entry := range source.File {
		header := entry.FileHeader
		out, err := writer.CreateHeader(&header)
		if err != nil {
			t.Fatal(err)
		}
		body := []byte("modified")
		if index > 0 {
			file, err := entry.Open()
			if err != nil {
				t.Fatal(err)
			}
			var b bytes.Buffer
			_, err = b.ReadFrom(file)
			_ = file.Close()
			if err != nil {
				t.Fatal(err)
			}
			body = b.Bytes()
		}
		if _, err := out.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, altered.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyDesktopMethodFrontend(path, "0.1.51"); err == nil {
		t.Fatal("altered method library admitted")
	}
}

func TestNativeQualificationFingerprintBindsStableCarrierWithoutChangingLegacy(t *testing.T) {
	in := nativeFixture(t)
	o, _, err := prepare(in.Packet)
	if err != nil {
		t.Fatal(err)
	}
	got, err := profileFingerprint(o)
	if err != nil {
		t.Fatal(err)
	}
	files, err := nativeProfileFiles(o)
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := targetConfigurationSnapshot(o.Binding.WorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := json.Marshal(map[string]any{"generated": files, "target_configuration": configuration})
	if err != nil || got != digest(legacy) {
		t.Fatal("legacy qualification fingerprint changed", err)
	}
	o.Carrier = &StableCarrier{ID: strings.Repeat("a", 32)}
	first, err := profileFingerprint(o)
	if err != nil {
		t.Fatal(err)
	}
	o.Carrier.ID = strings.Repeat("b", 32)
	second, err := profileFingerprint(o)
	if err != nil {
		t.Fatal(err)
	}
	if first == got || first == second {
		t.Fatal("carrier identity missing from genuine qualifier profile")
	}
}

func TestDesktopLiveBootstrapUsesCurrentCatalogAndCompleteMethodPages(t *testing.T) {
	files, err := desktopLiveFrontendPayload("0.1.51")
	if err != nil {
		t.Fatal(err)
	}
	for path, body := range files {
		if !strings.HasPrefix(path, "skills/") || !strings.HasSuffix(path, "/SKILL.md") {
			continue
		}
		for _, required := range []string{"methods.read", "methods.plan", "methods.list", "next_offset", "sha256", "total_bytes", "live method version", "reimport"} {
			if !bytes.Contains(body, []byte(required)) {
				t.Fatalf("%s lacks %s", path, required)
			}
		}
	}
	var projection struct {
		Projection string            `json:"projection"`
		Files      map[string]string `json:"files"`
	}
	if err := json.Unmarshal(files["methods-projection.json"], &projection); err != nil {
		t.Fatal(err)
	}
	if projection.Projection != "live_method_bootstrap" {
		t.Fatal("bootstrap provenance unavailable")
	}
	for path, hash := range projection.Files {
		if digest(files[path]) != hash {
			t.Fatal("projected bootstrap hash stale", path)
		}
	}
	root := filepath.Join(t.TempDir(), "live-bootstrap")
	built, err := BuildDesktopMethodFrontend(DesktopFrontendOptions{Destination: root, Version: "0.1.51", LiveMethods: true})
	if err != nil {
		t.Fatal(err)
	}
	verified, err := VerifyDesktopLiveMethodFrontend(filepath.Join(root, built.Archive), "0.1.51")
	if err != nil || verified.BootstrapProtocol != DesktopLiveMethodProtocol || verified.ArchiveSHA256 != built.ArchiveSHA256 {
		t.Fatal("canonical live bootstrap verification failed", err)
	}
	if _, err := VerifyDesktopMethodFrontend(filepath.Join(root, built.Archive), "0.1.51"); err == nil {
		t.Fatal("live bootstrap was confused with older snapshot library")
	}
}

func TestDesktopNamedBootstrapUsesExactPlanResources(t *testing.T) {
	files, err := desktopLiveFrontendPayload("0.1.51")
	if err != nil {
		t.Fatal(err)
	}
	for path, body := range files {
		if !strings.HasPrefix(path, "skills/") || !strings.HasSuffix(path, "/SKILL.md") {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(path, "skills/"), "/SKILL.md")
		if id == "operator" {
			if !bytes.Contains(body, []byte("Discover the complete current catalog using methods.list")) || !bytes.Contains(body, []byte("following next_offset while has_more")) {
				t.Fatal("open-ended operator discovery lost complete catalog pagination")
			}
			continue
		}
		if id == "resume" {
			continue
		}
		if id == "review-explain-change" {
			t.Logf("STATIC_NAMED_BOOTSTRAP method_id=%s wrapper_bytes=%d", id, len(body))
		}
		if !bytes.Contains(body, []byte("Call methods.plan with method_id `"+id+"`")) {
			t.Fatalf("named wrapper must select its exact current plan without enumerating unrelated methods: %s", id)
		}
		for _, stale := range []string{"after it appears in the live catalog", "Discover the current catalog with methods.list, including every page"} {
			if bytes.Contains(body, []byte(stale)) {
				t.Fatalf("named wrapper still requires a full catalog scan: %s", id)
			}
		}
		for _, required := range []string{"resources", "instruction_uri", "user_invoked_only", "plan's method_id differs, stop", "descriptors are absent", "Resolve referenced canonical methods with the same exact methods.plan lookup", "Match each page's resource_uri, sha256 and total_bytes against its plan descriptor", "next_offset", "has_more", "normal host review"} {
			if !bytes.Contains(body, []byte(required)) {
				t.Fatalf("named lookup omitted a current identity, dependency or compatibility requirement: %s: %s", id, required)
			}
		}
	}
	for _, required := range []string{"execution-continuity", "explicit resume request", "grants no resume authority"} {
		if !bytes.Contains(files["skills/resume/SKILL.md"], []byte(required)) {
			t.Fatal("resume alias lost explicit selection or its existing authority boundary", required)
		}
	}
}
